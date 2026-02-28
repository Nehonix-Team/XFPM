package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nehonix-Team/xfpm-go/internal/core"
	"github.com/Nehonix-Team/xfpm-go/internal/utils"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [packages...]",
	Short: "Uninstall packages and remove them from package.json",
	Aliases: []string{"remove", "rm", "un"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no packages specified for removal")
		}

		projectRoot, _ := os.Getwd()
		pkgPath := filepath.Join(projectRoot, "package.json")
		pkg, err := core.LoadPackageJson(pkgPath)
		if err != nil {
			return err
		}

		for _, pkgName := range args {
			delete(pkg.Dependencies, pkgName)
			delete(pkg.DevDependencies, pkgName)
			delete(pkg.OptionalDependencies, pkgName)
			
			// Remove from node_modules
			nmPath := filepath.Join(projectRoot, "node_modules", pkgName)
			if err := os.RemoveAll(nmPath); err != nil {
				utils.Warn("Failed to remove node_modules/%s: %v", pkgName, err)
			}
		}

		if err := pkg.Save(pkgPath); err != nil {
			return err
		}

		utils.Success("Packages removed from package.json.")
		utils.Info("Note: full dependency tree pruning is not yet implemented.")
		
		// Optional: triggered install to re-resolve the tree
		// return installCmd.RunE(cmd, []string{})
		return nil
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
