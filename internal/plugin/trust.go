package plugin

import (
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
)

func TrustPlugin(projectRoot string, pkgName string, authorKey string) error {
	// The actual signature cryptography and filesystem integrity is evaluated
	// deeply during the execution lifecycle and installation phase by the XHSC core
	// and Installer logic. This function merely pins the security policy in config.
	// 2. Add to configuration
	configPath := paths.ConfigPath(projectRoot)

	safeName := strings.ReplaceAll(pkgName, ".", "\\.")
	updates := map[string]interface{}{
		"$internal." + safeName + ".signature.author_key": authorKey,
	}

	if err := utils.UpdateJsonFile(configPath, updates); err == nil {
		utils.Success("Manual trust pinned for %s.", pkgName)
	} else {
		return err
	}
	return nil
}
