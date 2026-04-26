package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
)

// RevokeTrust completely revokes a plugin's privileges, removes it from the configuration,
// and optionally adds it back to the pending inspection list.
func RevokeTrust(projectRoot string, pkgName string, noPending bool) error {
	configPath := paths.ConfigPath(projectRoot)

	b, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

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

	var config map[string]interface{}
	json.Unmarshal(b, &config)

	internal, ok := config["$internal"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no trusted plugins found in project configuration")
	}

	if _, exists := internal[pkgName]; !exists {
		return fmt.Errorf("plugin %s is not currently trusted in this project", pkgName)
	}

	delete(internal, pkgName)
	config["$internal"] = internal
	if out, err := json.MarshalIndent(config, "", "    "); err == nil {
		os.WriteFile(configPath, out, 0644)
		utils.Success("Trust revoked and permissions retired for %s.", pkgName)
	}

	if !noPending {
		pendingPath := paths.PendingPluginsPath(projectRoot)
		var pending []map[string]string
		if data, err := os.ReadFile(pendingPath); err == nil {
			json.Unmarshal(data, &pending)
		}

		// Check if already pending
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
	} else {
		err := os.RemoveAll(pkgDir)
		if err == nil {
			utils.Info("Plugin symlink %s has been physically uninstalled.", pkgName)
		}
	}

	return nil
}
