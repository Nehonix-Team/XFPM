package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// HasLegacyStorage checks if a project has the old storage format.
func HasLegacyStorage(projectRoot string) bool {
	legacyDir := filepath.Join(projectRoot, "node_modules", ".xpm", "storage")
	fi, err := os.Stat(legacyDir)
	return err == nil && fi.IsDir()
}

// ProgressCallback is called during migration to report progress.
type ProgressCallback func(current, total int, message string)

// MigrateLegacyStorage moves files from local storage to the global CAS.
func MigrateLegacyStorage(projectRoot string, cas *Cas, callback ProgressCallback) error {
	legacyDir := filepath.Join(projectRoot, "node_modules", ".xpm", "storage")
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

// FindLegacyStorages recursively finds legacy XFPM storage directories.
// optimized for speed.
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
		"node_modules": false, // we want to enter node_modules but not its children
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
				found = append(found, filepath.Dir(path)) // return project root
			}
			return filepath.SkipDir // don't go deeper into node_modules
		}

		return nil
	})

	return found
}
