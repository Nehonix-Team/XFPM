package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nehonix-Team/XFMP/internal/paths"
)

// HasLegacyStorage checks if a project has the old storage format.
func HasLegacyStorage(projectRoot string) bool {
	legacyDir := filepath.Join(paths.LocalXpmDir(projectRoot), "storage")
	fi, err := os.Stat(legacyDir)
	return err == nil && fi.IsDir()
}

// ProgressCallback is called during migration to report progress.
type ProgressCallback func(current, total int, message string)

// MigrateLegacyStorage moves files from local storage to the global CAS.
func MigrateLegacyStorage(projectRoot string, cas *Cas, callback ProgressCallback) error {
	legacyDir := filepath.Join(paths.LocalXpmDir(projectRoot), "storage")
	return MigratePathToCas(legacyDir, cas, callback)
}

// MigratePathToCas moves files from ANY legacy storage directory to the unified global CAS.
func MigratePathToCas(sourceDir string, cas *Cas, callback ProgressCallback) error {
	legacyFilesDir := filepath.Join(sourceDir, "files")
	// If it doesn't have a 'files' subfolder, maybe it's the root itself?
	// Old versions might have different structures.
	if _, err := os.Stat(legacyFilesDir); err != nil {
		legacyFilesDir = sourceDir
	}

	// Count files first for progress reporting
	var total int
	filepath.Walk(legacyFilesDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total++
		}
		return nil
	})

	var current int
	// 1. Move files to global CAS
	err := filepath.Walk(legacyFilesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}

		current++
		if callback != nil {
			callback(current, total, filepath.Base(path))
		}

		// Re-store in global CAS
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		isExecutable := (info.Mode() & 0111) != 0
		_, err = cas.StoreStream(file, isExecutable)
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to migrate files: %w", err)
	}

	// 2. Cleanup legacy storage
	return os.RemoveAll(sourceDir)
}

// MigrateLegacyHome moves binaries, globals and storage from .xpm to .xfpm
func MigrateLegacyHome(oldHome, newHome string, cas *Cas, callback ProgressCallback) error {
	// 1. Migrate bin
	oldBin := filepath.Join(oldHome, "bin")
	newBin := filepath.Join(newHome, "bin")
	if err := migrateDirContents(oldBin, newBin); err != nil {
		return fmt.Errorf("failed to migrate binaries: %w", err)
	}

	// 2. Migrate globals
	oldGlobals := filepath.Join(oldHome, "globals")
	newGlobals := filepath.Join(newHome, "globals")
	if err := migrateDirContents(oldGlobals, newGlobals); err != nil {
		return fmt.Errorf("failed to migrate globals: %w", err)
	}

	// 3. Migrate storage
	oldStorage := filepath.Join(oldHome, "storage")
	if fi, err := os.Stat(oldStorage); err == nil && fi.IsDir() {
		if err := MigratePathToCas(oldStorage, cas, callback); err != nil {
			return err
		}
	}

	// 4. Final Cleanup (remove what's left in old home)
	return os.RemoveAll(oldHome)
}

// migrateDirContents moves all entries from source to destination, merging if needed.
func migrateDirContents(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// If destination already exists, remove it first (overwrite)
		if _, err := os.Stat(dstPath); err == nil {
			os.RemoveAll(dstPath)
		}

		if err := os.Rename(srcPath, dstPath); err != nil {
			// Fallback: simple copy if rename fails
			return err
		}
	}
	return nil
}

// FindLegacyStorages recursively finds legacy XFPM storage directories.
func FindLegacyStorages(root string) []string {
	var found []string
	
	// Skip common large non-project directories
	skipDirs := map[string]bool{
		".git":         true,
		".vscode":      true,
		".idea":        true,
		"dist":         true,
		"build":        true,
		"out":          true,
		"temp":         true,
		"tmp":          true,
		"cache":        true,
		"node_modules": false, 
	}

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			return nil
		}

		name := info.Name()
		if skipDirs[name] {
			return filepath.SkipDir
		}

		// If we are in a node_modules, check for .xpm/storage
		if name == "node_modules" {
			legacyPath := filepath.Join(path, ".xpm", "storage")
			if fi, err := os.Stat(legacyPath); err == nil && fi.IsDir() {
				found = append(found, filepath.Dir(path)) 
			}
			return filepath.SkipDir 
		}

		return nil
	})

	return found
}
