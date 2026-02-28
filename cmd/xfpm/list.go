package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nehonix-Team/xfpm-go/internal/core"
	"github.com/Nehonix-Team/xfpm-go/internal/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installed packages",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, _ := os.Getwd()
		pkgPath := filepath.Join(projectRoot, "package.json")
		pkg, err := core.LoadPackageJson(pkgPath)
		if err != nil {
			return err
		}

		utils.Info("Dependencies for: %s", pkg.Name)
		printDeps("Dependencies", pkg.Dependencies)
		printDeps("Dev Dependencies", pkg.DevDependencies)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func printDeps(title string, deps map[string]string) {
	if len(deps) == 0 {
		return
	}
	fmt.Printf("\n%s:\n", color.MagentaString(title))
	for name, version := range deps {
		fmt.Printf("  %s@%s\n", name, color.CyanString(version))
	}
}
