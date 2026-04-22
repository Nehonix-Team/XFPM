package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/utils"
)

var GlobalBytesDownloaded atomic.Uint64

type RegistryPackage struct {
	Name     string                     `json:"name"`
	DistTags map[string]string          `json:"dist-tags"`
	Versions map[string]VersionMetadata `json:"versions"`
}

type StringOrStringArray []string

func (s *StringOrStringArray) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '[' {
		var a []string
		if err := json.Unmarshal(data, &a); err != nil {
			return err
		}
		*s = a
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = []string{str}
	return nil
}

type VersionMetadata struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dist                 Dist              `json:"dist"`
	Dependencies         map[string]string `json:"dependencies,omitempty"`
	PeerDependencies     map[string]string `json:"peerDependencies,omitempty"`
	PeerDependenciesMeta map[string]struct {
		Optional bool `json:"optional"`
	} `json:"peerDependenciesMeta,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	Bin                  json.RawMessage   `json:"bin,omitempty"`
	OS                   StringOrStringArray `json:"os,omitempty"`
	CPU                  StringOrStringArray `json:"cpu,omitempty"`
	Libc                 StringOrStringArray `json:"libc,omitempty"`
	Xfpm                 *XfpmConfig         `json:"xfpm,omitempty"`
	Scripts              map[string]string   `json:"scripts,omitempty"`
	Files                []string            `json:"files,omitempty"`
}

type Dist struct {
	Tarball      string `json:"tarball"`
	Shasum       string `json:"shasum"`
	Integrity    string `json:"integrity,omitempty"`
	UnpackedSize uint64 `json:"unpackedSize,omitempty"`
	FileCount    uint64 `json:"fileCount,omitempty"`
}

type RegistryClient struct {
	httpClient       *http.Client
	downloadClient   *http.Client
	baseURL          string
	retries          uint32
	packageCache    sync.Map
	inflight         sync.Map
	metaSemaphore    *utils.AdaptiveSemaphore
	dataSemaphore    *utils.AdaptiveSemaphore
	cacheDir        string
	config          *DynamicConfig
}

func NewRegistryClient(baseURL string, retries uint32) *RegistryClient {
	if baseURL == "" {
		baseURL = "https://registry.npmjs.org"
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	downloadClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 32,
		},
	}

	return &RegistryClient{
		httpClient:     httpClient,
		downloadClient: downloadClient,
		baseURL:        strings.TrimSuffix(baseURL, "/"),
		retries:        retries,
		metaSemaphore:  utils.NewAdaptiveSemaphore(128),
		dataSemaphore:  utils.NewAdaptiveSemaphore(8),
		config:         NewDynamicConfig(),
	}
}

func (c *RegistryClient) SetCacheDir(path string) {
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".xpm", "cache")
	}
	c.cacheDir = filepath.Join(path, "metadata")
	os.MkdirAll(c.cacheDir, 0755)
}

func (c *RegistryClient) FetchPackage(ctx context.Context, name string, ignoreCache bool) (*RegistryPackage, error) {
	if !ignoreCache {
		if val, ok := c.packageCache.Load(name); ok {
			return val.(*RegistryPackage), nil
		}
		if c.cacheDir != "" {
			path := filepath.Join(c.cacheDir, strings.ReplaceAll(name, "/", "+")+".json")
			if data, err := os.ReadFile(path); err == nil {
				var pkg RegistryPackage
				if err := json.Unmarshal(data, &pkg); err == nil {
					c.packageCache.Store(name, &pkg)
					return &pkg, nil
				}
			}
		}
	}

	// Simple inflight suppression
	var wg sync.WaitGroup
	var pkg *RegistryPackage
	var err error

	actual, loaded := c.inflight.LoadOrStore(name, &wg)
	if loaded {
		actual.(*sync.WaitGroup).Wait()
		return c.FetchPackage(ctx, name, false)
	}

	wg.Add(1)
	defer func() {
		wg.Done()
		c.inflight.Delete(name)
	}()

	pkg, err = c.fetchPackageNetwork(ctx, name)
	return pkg, err
}

func (c *RegistryClient) FetchVersionMetadata(ctx context.Context, name, version string) (*VersionMetadata, error) {
	// Check persistent metadata cache first (Local-First)
	home, _ := os.UserHomeDir()
	metaPath := filepath.Join(home, ".xpm", "storage", "metadata", strings.ReplaceAll(name, "/", "+")+"@"+version+".json")
	
	if data, err := os.ReadFile(metaPath); err == nil {
		var meta VersionMetadata
		if err := json.Unmarshal(data, &meta); err == nil {
			return &meta, nil
		}
	}

	url := fmt.Sprintf("%s/%s/%s", c.baseURL, name, version)
	
	bytes, err := c.requestWithRetry(ctx, url, true, false) // Not abbreviated!
	if err != nil {
		return nil, err
	}

	var meta VersionMetadata
	if err := json.Unmarshal(bytes, &meta); err != nil {
		return nil, fmt.Errorf("parsing version metadata for %s@%s: %w", name, version, err)
	}

	// Persist for offline use
	os.MkdirAll(filepath.Dir(metaPath), 0755)
	os.WriteFile(metaPath, bytes, 0644)

	return &meta, nil
}

func (c *RegistryClient) fetchPackageNetwork(ctx context.Context, name string) (*RegistryPackage, error) {
	url := fmt.Sprintf("%s/%s", c.baseURL, name)
	
	bytes, err := c.requestWithRetry(ctx, url, true, true)
	if err != nil {
		return nil, err
	}

	var pkg RegistryPackage
	if err := json.Unmarshal(bytes, &pkg); err != nil {
		return nil, fmt.Errorf("parsing metadata for %s: %w", name, err)
	}

	c.packageCache.Store(name, &pkg)

	if c.cacheDir != "" {
		path := filepath.Join(c.cacheDir, strings.ReplaceAll(name, "/", "+")+".json")
		go os.WriteFile(path, bytes, 0644)
	}

	return &pkg, nil
}

func (c *RegistryClient) requestWithRetry(ctx context.Context, url string, isMetadata bool, isAbbreviated bool) ([]byte, error) {
	var lastErr error
	for attempt := uint32(0); attempt <= c.retries; attempt++ {
		// Adaptive Concurrency Check
		limit := c.config.GetConcurrency()
		semaphore := c.metaSemaphore
		if !isMetadata {
			limit = 8 // Keep data limited to avoid bandwidth saturated
			semaphore = c.dataSemaphore
		}

		if err := semaphore.Acquire(ctx, limit); err != nil {
			return nil, err
		}

		timeout := c.config.GetTimeout(attempt)
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		
		start := time.Now()
		client := c.httpClient
		if !isMetadata {
			client = c.downloadClient
		}

		req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
		if err != nil {
			cancel()
			semaphore.Release()
			return nil, err
		}

		if isMetadata {
			if isAbbreviated {
				req.Header.Set("Accept", "application/vnd.npm.install-v1+json; q=1.0, application/json; q=0.8, */*")
			} else {
				req.Header.Set("Accept", "application/json")
			}
		}
		req.Header.Set("User-Agent", "XyPriss/1.0 (Go; +https://github.com/Nehonix-Team/XyPriss)")

		resp, err := client.Do(req)
		elapsed := time.Since(start)
		
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				body, berr := io.ReadAll(resp.Body)
				resp.Body.Close()
				cancel()
				semaphore.Release()
				if berr == nil {
					c.config.RecordRequest(elapsed, false)
					return body, nil
				}
				lastErr = berr
			} else {
				resp.Body.Close()
				cancel()
				semaphore.Release()
				if resp.StatusCode == http.StatusNotFound {
					return nil, fmt.Errorf("package not found: %s", url)
				}
				lastErr = fmt.Errorf("HTTP %d for URL: %s", resp.StatusCode, url)
			}
		} else {
			cancel()
			semaphore.Release()
			lastErr = err
		}

		c.config.RecordRequest(elapsed, true)

		if attempt < c.retries {
			time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
		}
	}

	return nil, fmt.Errorf("failed to fetch %s after %d retries: %w", url, c.retries, lastErr)
}

func (c *RegistryClient) DownloadTarballStream(ctx context.Context, url string) (io.ReadCloser, error) {
	var lastErr error
	for attempt := uint32(0); attempt <= c.retries; attempt++ {
		if err := c.dataSemaphore.Acquire(ctx, 8); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			c.dataSemaphore.Release()
			return nil, err
		}
		req.Header.Set("User-Agent", "XyPriss/1.0 (Go)")
		start := time.Now()

		resp, err := c.downloadClient.Do(req)
		elapsed := time.Since(start)

		if err == nil {
			if resp.StatusCode == http.StatusOK {
				c.config.RecordRequest(elapsed, false)
				wrapped := &trackingReadCloser{ReadCloser: resp.Body}
				return &semaphoreReleasingReader{ReadCloser: wrapped, sem: c.dataSemaphore}, nil
			}
			resp.Body.Close()
			c.dataSemaphore.Release()
			lastErr = fmt.Errorf("HTTP %d for URL: %s", resp.StatusCode, url)
		} else {
			lastErr = err
			c.config.RecordRequest(elapsed, true)
			c.dataSemaphore.Release()
		}

		if attempt < c.retries {
			time.Sleep(time.Duration(500*(attempt+1)) * time.Millisecond)
		}
	}

	return nil, fmt.Errorf("failed to download tarball %s: %w", url, lastErr)
}

type semaphoreReleasingReader struct {
	io.ReadCloser
	sem  *utils.AdaptiveSemaphore
	once sync.Once
}

func (r *semaphoreReleasingReader) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(func() {
		r.sem.Release()
	})
	return err
}

type trackingReadCloser struct {
	io.ReadCloser
}

func (t *trackingReadCloser) Read(p []byte) (n int, err error) {
	n, err = t.ReadCloser.Read(p)
	if n > 0 {
		GlobalBytesDownloaded.Add(uint64(n))
	}
	return
}
