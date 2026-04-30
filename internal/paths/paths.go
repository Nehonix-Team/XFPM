package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var xfpmPath string = ".xfpm"

// XpmHome returns the base XFPM directory, usually ~/.xfpm
func XpmHome() string {
	return filepath.Join(getEffectiveHome(), xfpmPath)
}

// LegacyHome returns the old base XPM directory, usually ~/.xpm
func LegacyHome() string {
	return filepath.Join(getEffectiveHome(), ".xpm")
}

// getEffectiveHome returns the home directory, prioritizing the original user's home
// when running under sudo to ensure consistent tool discovery.
func getEffectiveHome() string {
	if runtime.GOOS == "windows" {
		home, _ := os.UserHomeDir()
		return home
	}

	if os.Geteuid() == 0 {
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			// Try to recover original home from environment
			if home := os.Getenv("HOME"); home != "" && !strings.HasPrefix(home, "/root") {
				return home
			}
			// Fallback: common Linux home path
			userHome := filepath.Join("/home", sudoUser)
			if _, err := os.Stat(userHome); err == nil {
				return userHome
			}
		}
	}
	home, _ := os.UserHomeDir()
	return home
}

// IdentityPath returns the path to the user's Ed25519 private key
func IdentityPath() string {
	return filepath.Join(XpmHome(), "id_ed25519")
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
	return filepath.Join(projectRoot, "node_modules", xfpmPath)
}

// LocalBinDir returns the project-local node_modules/.bin directory
func LocalBinDir(projectRoot string) string {
	return filepath.Join(projectRoot, "node_modules", ".bin")
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
	return filepath.Join(GlobalsDir(), "node_modules", xfpmPath, "vstore")
}

// EphemeralCacheDir returns a temporary directory for ephemeral operations
func EphemeralCacheDir() string {
	return filepath.Join(GlobalCacheDir(), "ephemeral")
}

// LegacyStoragePaths returns a list of old storage directories for migration purposes
func LegacyStoragePaths() []string {
	home := XpmHome()
	return []string{
		filepath.Join(home, "xpm_global"),
		filepath.Join(home, "xpm_store"),
		filepath.Join(home, "virtual_store"),
		filepath.Join(home, "xpm_store_legacy"),
		filepath.Join(home, "registry_cache"),
		filepath.Join(home, "xpm_cache"),
		filepath.Join(os.Getenv("HOME"), ".xpm"),
	}
}

// PendingPluginsPath returns the path to the pending plugins verification file
func PendingPluginsPath(projectRoot string) string {
	return filepath.Join(LocalXpmDir(projectRoot), "pending_plugins.json")
}

// PackageVStoreDir returns the absolute directory for a specific package strictly inside the virtual store
func PackageVStoreDir(projectRoot, pkgName, pkgVersion string) string {
	importName := strings.ReplaceAll(pkgName, "/", "+") + "@" + pkgVersion
	return filepath.Join(LocalVStoreDir(projectRoot), importName, "node_modules", pkgName)
}

// PackageSigPath returns the designated signature file path within the package root
func PackageSigPath(pkgDir string) string {
	return filepath.Join(pkgDir, SigFileName)
}

// SigFileName is the official name for XyPriss cryptographic signatures
const SigFileName = "xypriss.plugin.xsig"

// NodeModulesPkgDir returns the path to a package within the project's node_modules
func NodeModulesPkgDir(projectRoot, pkgName string) string {
	return filepath.Join(projectRoot, "node_modules", pkgName)
}

// PackageJsonPath returns the path to the project's package.json
func PackageJsonPath(projectRoot string) string {
	return filepath.Join(projectRoot, "package.json")
}
