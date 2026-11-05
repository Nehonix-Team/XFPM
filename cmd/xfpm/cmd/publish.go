package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var publishCmd = &cobra.Command{
	Use:                "publish",
	Short:              "Publish a package to the registry",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintBanner()

		// Handle help flag manually since DisableFlagParsing is true
		for _, arg := range args {
			if arg == "--help" || arg == "-h" {
				cmd.Help()
				return nil
			}
		}

		utils.Matrix("Preparing to publish package...")

		npmArgs := append([]string{"publish"}, args...)
		c := exec.Command("npm", npmArgs...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin

		err := c.Run()
		if err != nil {
			utils.Error("\nFailed to publish: %v", err)
			return err
		}

		// Invalidate local metadata cache for the published package so subsequent installs fetch the new version immediately
		if pkgData, readErr := os.ReadFile("package.json"); readErr == nil {
			var pkgMeta struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(pkgData, &pkgMeta) == nil && pkgMeta.Name != "" {
				cacheFile := filepath.Join(paths.GlobalCacheDir(), "metadata", strings.ReplaceAll(pkgMeta.Name, "/", "+")+".json")
				_ = os.Remove(cacheFile)
			}
		}

		utils.Success("Package published successfully!")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(publishCmd)
}
