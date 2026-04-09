package core

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/Nehonix-Team/xfpm-go/internal/utils"
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
	smallFileLimit := 64 * 1024
	buffer := make([]byte, 0, smallFileLimit)

	// Try to read up to smallFileLimit directly from reader (no bufio!)
	tmp := make([]byte, 16384)
	var err error
	for len(buffer) < smallFileLimit {
		var n int
		n, err = reader.Read(tmp)
		if n > 0 {
			buffer = append(buffer, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}

	if err != nil && err != io.EOF {
		return "", err
	}

	// Case 1: Small file (fit in memory)
	if len(buffer) < smallFileLimit {
		h := blake3.New()
		h.Write(buffer)
		hashHex := hex.EncodeToString(h.Sum(nil))
		destPath := c.GetFilePath(hashHex)

		// Validate cached file is non-empty (guard against broken prior extractions)
		if fi, err := os.Stat(destPath); err == nil && fi.Size() == int64(len(buffer)) {
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
			// Ensure destination doesn't exist to avoid truncation if it was a link
			os.Remove(destPath)
			if err := c.copyAndCleanup(tempPath, destPath); err != nil {
				return "", err
			}
		}

		return hashHex, nil
	}

	// Case 2: Large file (streaming)
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

	streamingBuffer := make([]byte, 128*1024)
	for {
		n, err := reader.Read(streamingBuffer)
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

	// Validate cached file integrity before reusing
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		c.ensurePermissions(destPath, isExecutable)
		return hashHex, nil
	}

	if err := c.ensureParentDirs(hashHex); err != nil {
		return "", err
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		// If rename fails (e.g., cross-device link), try copying
		// Ensure destination doesn't exist to avoid truncation if it was a link
		os.Remove(destPath)
		if err := c.copyAndCleanup(tempPath, destPath); err != nil {
			return "", fmt.Errorf("failed to rename or copy %s to %s: %w", tempPath, destPath, err)
		}
		// Verify copied file size
		fi_src, _ := os.Stat(tempPath)
		fi_dst, _ := os.Stat(destPath)
		if fi_src != nil && fi_dst != nil {
			if fi_dst.Size() != fi_src.Size() {
				// This is a critical error, log it prominently
				utils.Error("[CAS] CRITICAL: Size mismatch after copy! Source: %d, Dest: %d. File: %s", fi_src.Size(), fi_dst.Size(), destPath)
			}
		}
	}

	// Double check final file size
	if fi, err := os.Stat(destPath); err == nil && fi.Size() == 0 {
		utils.Error("[CAS] CRITICAL: Final file %s is 0 bytes after rename/copy!", destPath)
	}

	c.ensurePermissions(destPath, isExecutable)
	return hashHex, nil
}

func (c *Cas) ensurePermissions(path string, isExecutable bool) {
	if runtime.GOOS == "windows" {
		return
	}
	mode := os.FileMode(0444)
	if isExecutable {
		mode = 0555
	}
	os.Chmod(path, mode)
}

func (c *Cas) copyAndCleanup(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Ensure destination doesn't exist to avoid truncation if it was a link to source
	os.Remove(dst)
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
