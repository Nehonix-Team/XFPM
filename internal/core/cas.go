package core

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/google/uuid"
	"github.com/zeebo/blake3"
)

type Cas struct {
	BasePath string
	dirCache sync.Map
}

func NewCas(basePath string) (*Cas, error) {
	if !filepath.IsAbs(basePath) {
		cwd, _ := os.Getwd()
		basePath = filepath.Join(cwd, basePath)
	}

	cas := &Cas{BasePath: basePath}

	dirs := []string{
		"files",
		"indices",
		"metadata",
		"temp",
		"virtual_store",
	}

	for _, d := range dirs {
		if err := CreateDirAllSecure(filepath.Join(basePath, d)); err != nil {
			return nil, fmt.Errorf("failed to create XCAS %s directory: %w", d, err)
		}
	}

	return cas, nil
}

func CreateDirAllSecure(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(path, 0755)
	}
	return nil
}

func (c *Cas) GetFilePath(hash string) string {
	if len(hash) < 4 {
		return filepath.Join(c.BasePath, "files", hash)
	}
	prefix := hash[0:2]
	subPrefix := hash[2:4]
	rest := hash[4:]
	return filepath.Join(c.BasePath, "files", prefix, subPrefix, rest)
}

func (c *Cas) StoreStream(reader io.Reader, isExecutable bool) (string, error) {
	bufReader := bufio.NewReaderSize(reader, 128*1024)
	smallFileLimit := 64 * 1024
	buffer := make([]byte, 0, smallFileLimit)

	// Try to read up to smallFileLimit
	tmp := make([]byte, 16384)
	for len(buffer) < smallFileLimit {
		n, err := bufReader.Read(tmp)
		if n > 0 {
			buffer = append(buffer, tmp[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}

	if len(buffer) < smallFileLimit {
		// Memory path
		h := blake3.New()
		h.Write(buffer)
		hashHex := hex.EncodeToString(h.Sum(nil))
		destPath := c.GetFilePath(hashHex)

		if _, err := os.Stat(destPath); err == nil {
			c.ensurePermissions(destPath, isExecutable)
			return hashHex, nil
		}

		if err := c.ensureParentDirs(hashHex); err != nil {
			return "", err
		}

		tempPath := filepath.Join(c.BasePath, "temp", uuid.New().String())
		if err := os.WriteFile(tempPath, buffer, 0644); err != nil {
			return "", fmt.Errorf("writing small file to temp CAS: %w", err)
		}

		c.ensurePermissions(tempPath, isExecutable)

		if err := os.Rename(tempPath, destPath); err != nil {
			// Fallback to copy if rename fails (e.g. cross-device)
			if err := c.copyAndCleanup(tempPath, destPath); err != nil {
				return "", err
			}
		}

		return hashHex, nil
	}

	// Streaming path for large files
	tempPath := filepath.Join(c.BasePath, "temp", uuid.New().String())
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return "", fmt.Errorf("creating temp file in CAS: %w", err)
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempPath)
	}()

	hasher := blake3.New()
	
	// Write already buffered data
	if _, err := tempFile.Write(buffer); err != nil {
		return "", err
	}
	hasher.Write(buffer)

	streamingBuffer := make([]byte, 131072)
	for {
		n, err := bufReader.Read(streamingBuffer)
		if n > 0 {
			if _, werr := tempFile.Write(streamingBuffer[:n]); werr != nil {
				return "", werr
			}
			hasher.Write(streamingBuffer[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}

	if err := tempFile.Sync(); err != nil {
		return "", err
	}
	tempFile.Close()

	hashHex := hex.EncodeToString(hasher.Sum(nil))
	destPath := c.GetFilePath(hashHex)

	if _, err := os.Stat(destPath); err == nil {
		c.ensurePermissions(destPath, isExecutable)
		return hashHex, nil
	}

	if err := c.ensureParentDirs(hashHex); err != nil {
		return "", err
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		if err := c.copyAndCleanup(tempPath, destPath); err != nil {
			return "", err
		}
	}

	c.ensurePermissions(destPath, isExecutable)
	return hashHex, nil
}

func (c *Cas) ensurePermissions(path string, isExecutable bool) {
	if runtime.GOOS == "windows" {
		return
	}
	mode := os.FileMode(0644)
	if isExecutable {
		mode = 0755
	}
	os.Chmod(path, mode)
}

func (c *Cas) copyAndCleanup(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	os.Remove(src)
	return nil
}

func (c *Cas) ensureParentDirs(hash string) error {
	if len(hash) < 4 {
		return nil
	}
	prefix := hash[0:2]
	subPrefix := hash[2:4]
	key := prefix + "/" + subPrefix
	
	if _, ok := c.dirCache.Load(key); !ok {
		dir := filepath.Join(c.BasePath, "files", prefix, subPrefix)
		if err := CreateDirAllSecure(dir); err != nil {
			return fmt.Errorf("failed to create CAS directory %s: %w", dir, err)
		}
		c.dirCache.Store(key, struct{}{})
	}
	return nil
}

func (c *Cas) StoreIndex(name, version string, index map[string]string) error {
	safeName := filepath.Clean(name)
	filename := fmt.Sprintf("%s@%s.json", safeName, version)
	// Replace / with + for safe filename
	filename = filepath.Join(c.BasePath, "indices", fmt.Sprintf("%s@%s.json", filepath.Base(filepath.Join("/", name)), version))
	// Actually better rewrite the logic to match Rust's name.replace("/", "+")
	filename = filepath.Join(c.BasePath, "indices", fmt.Sprintf("%s@%s.json", c.escapePackageName(name), version))

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func (c *Cas) GetIndex(name, version string) (map[string]string, error) {
	path := filepath.Join(c.BasePath, "indices", fmt.Sprintf("%s@%s.json", c.escapePackageName(name), version))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var index map[string]string
	err = json.Unmarshal(data, &index)
	return index, err
}

func (c *Cas) escapePackageName(name string) string {
	// Equivalent to Rust's name.replace("/", "+")
	res := ""
	for _, r := range name {
		if r == '/' {
			res += "+"
		} else {
			res += string(r)
		}
	}
	return res
}
