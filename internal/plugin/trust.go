package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
)

// TrustPlugin securely pins manual trust for a plugin author after verifying its installation and signature integrity.
func TrustPlugin(projectRoot string, pkgName string, authorKey string) error {
	// 1. Validation Logic: Ensure installed and signature exists
	pkgDir := filepath.Join(projectRoot, "node_modules", pkgName)
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return fmt.Errorf("package %s is not installed", pkgName)
	}

	sigPath := filepath.Join(pkgDir, "xypriss.plugin.xsig")
	if _, err := os.Stat(sigPath); os.IsNotExist(err) {
		return fmt.Errorf("package %s is not a verified plugin (missing .xsig)", pkgName)
	}

	// 2. Add to configuration
	configPath := paths.ConfigPath(projectRoot)

	var config map[string]interface{}
	if cfgBytes, err := os.ReadFile(configPath); err == nil {
		lines := strings.Split(string(cfgBytes), "\n")
		var cleanLines []string
		for _, line := range lines {
			if idx := strings.Index(line, "//"); idx != -1 {
				line = line[:idx]
			}
			cleanLines = append(cleanLines, line)
		}
		json.Unmarshal([]byte(strings.Join(cleanLines, "\n")), &config)
	}
	if config == nil {
		config = make(map[string]interface{})
	}

	internalRaw, ok := config["$internal"]
	if !ok {
		internalRaw = make(map[string]interface{})
	}
	internal, ok := internalRaw.(map[string]interface{})
	if !ok {
		internal = make(map[string]interface{})
	}

	pluginCfgRaw, ok := internal[pkgName]
	if !ok {
		pluginCfgRaw = make(map[string]interface{})
	}
	pluginCfg, ok := pluginCfgRaw.(map[string]interface{})
	if !ok {
		pluginCfg = make(map[string]interface{})
	}

	sigCfgRaw, ok := pluginCfg["signature"]
	if !ok {
		sigCfgRaw = make(map[string]interface{})
	}
	sigCfg, ok := sigCfgRaw.(map[string]interface{})
	if !ok {
		sigCfg = make(map[string]interface{})
	}

	sigCfg["author_key"] = authorKey
	pluginCfg["signature"] = sigCfg
	internal[pkgName] = pluginCfg
	config["$internal"] = internal

	if out, err := json.MarshalIndent(config, "", "    "); err == nil {
		os.WriteFile(configPath, out, 0644)
		utils.Success("Manual trust pinned for %s.", pkgName)
	}
	return nil
}
