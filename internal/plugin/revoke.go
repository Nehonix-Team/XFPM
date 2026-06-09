package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
)

// RevokeTrust completely revokes a plugin's privileges, removes it from the configuration,
// and optionally adds it back to the pending inspection list.
func RevokeTrust(projectRoot string, pkgName string, noPending bool) error {
	configPath := paths.ConfigPath(projectRoot)

	// We only need the path, no need to read the whole file to memory manually anymore.

	// Pre-revocation check: Ensure it's an actual plugin in node_modules
	pkgDir := filepath.Join(projectRoot, "node_modules", pkgName)
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return fmt.Errorf("package %s is not installed. Revocation requires an active installation for verification", pkgName)
	}

	sigPath := filepath.Join(pkgDir, "xypriss.plugin.xsig")
	if _, err := os.Stat(sigPath); os.IsNotExist(err) {
		return fmt.Errorf("package %s is not a verified plugin (missing .xsig)", pkgName)
	}

	pj, err := core.LoadPackageJson(filepath.Join(pkgDir, "package.json"))
	if err != nil {
		return fmt.Errorf("failed to load package.json for %s: %w", pkgName, err)
	}
	pkgVer := pj.Version

	safeName := strings.ReplaceAll(pkgName, ".", "\\.")
	if err := utils.RemoveFromJsonFile(configPath, []string{"$internal." + safeName}); err == nil {
		utils.Success("\nTrust revoked and permissions retired for %s.\n", pkgName)
	}

	// Always add back to pending list so it remains manageable in the UI
	pendingPath := paths.PendingPluginsPath(projectRoot)
	var pending []map[string]string
	if data, err := os.ReadFile(pendingPath); err == nil {
		json.Unmarshal(data, &pending)
	}

	found := false
	for _, p := range pending {
		if p["name"] == pkgName && p["version"] == pkgVer {
			found = true
			break
		}
	}

	if !found {
		pending = append(pending, map[string]string{
			"name":    pkgName,
			"version": pkgVer,
		})
		if out, err := json.MarshalIndent(pending, "", "  "); err == nil {
			os.MkdirAll(filepath.Dir(pendingPath), 0755)
			os.WriteFile(pendingPath, out, 0644)
			utils.Info("Plugin %s@%s moved back to pending list.", pkgName, pkgVer)
		}
	}

	if noPending {
		err := os.RemoveAll(pkgDir)
		if err == nil {
			utils.Info("Plugin symlink %s has been physically uninstalled. It remains in pending list for future re-installation.", pkgName)
		}
	}

	return nil
}
