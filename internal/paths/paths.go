package paths

import (
	"os"
	"path/filepath"
)

// XpmHome returns the base XFPM directory, usually ~/.xpm
func XpmHome() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".xpm")
}

// BinDir returns the directory for global binaries
func BinDir() string {
	return filepath.Join(XpmHome(), "bin")
}

// StorageDir returns the content-addressable storage directory.
// It respects the XFPM_STORAGE environment variable if set.
func StorageDir() string {
	if envStore := os.Getenv("XFPM_STORAGE"); envStore != "" {
		return envStore
	}
	return filepath.Join(XpmHome(), "storage")
}

// GlobalsDir returns the directory for global package installations
func GlobalsDir() string {
	return filepath.Join(XpmHome(), "globals")
}

// GlobalCacheDir returns the global metadata/registry cache directory
func GlobalCacheDir() string {
	return filepath.Join(XpmHome(), "cache")
}

// LocalXpmDir returns the project-local .xpm directory
func LocalXpmDir(projectRoot string) string {
	return filepath.Join(projectRoot, "node_modules", ".xpm")
}

// LocalVStoreDir returns the project-local virtual store directory
func LocalVStoreDir(projectRoot string) string {
	return filepath.Join(LocalXpmDir(projectRoot), "vstore")
}

// LocalTmpDir returns the project-local temporary directory
func LocalTmpDir(projectRoot string) string {
	return filepath.Join(LocalXpmDir(projectRoot), "tmp")
}

// RegistryCacheDir returns the cache directory for the registry client
func RegistryCacheDir(localXpmDir string) string {
	return filepath.Join(localXpmDir, "cache")
}

// ConfigPath returns the path to the xypriss configuration file.
// It checks for .jsonc first, then .json. If neither exists, returns .jsonc as default.
func ConfigPath(projectRoot string) string {
	jsonc := filepath.Join(projectRoot, "xypriss.config.jsonc")
	if _, err := os.Stat(jsonc); err == nil {
		return jsonc
	}
	json := filepath.Join(projectRoot, "xypriss.config.json")
	if _, err := os.Stat(json); err == nil {
		return json
	}
	return jsonc
}

// GlobalVStoreDir returns the global virtual store directory
func GlobalVStoreDir() string {
	return filepath.Join(GlobalsDir(), "node_modules", ".xpm", "vstore")
}

// EphemeralCacheDir returns a temporary directory for ephemeral operations
func EphemeralCacheDir() string {
	return filepath.Join(GlobalCacheDir(), "ephemeral")
}

// LegacyStoragePaths returns a list of old storage directories for migration purposes
func LegacyStoragePaths() []string {
	home := XpmHome()
	return []string{
		filepath.Join(home, "xpm_global_storage"),
		filepath.Join(home, "xpm_store"),
		filepath.Join(home, "xpm_store_legacy"),
		filepath.Join(home, "registry_cache"),
	}
}
