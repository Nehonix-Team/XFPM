/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:     "uninstall [packages...]",
	Short:   "Uninstall packages and remove them from package.json",
	Aliases: []string{"remove", "rm", "un"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no packages specified for removal")
		}

		global, _ := cmd.Flags().GetBool("global")
		projectRoot, _ := os.Getwd()

		var nmRoot string
		var pkgJsonPath string

		if global {
			nmRoot = filepath.Join(paths.GlobalsDir(), "node_modules")
		} else {
			nmRoot = filepath.Join(projectRoot, "node_modules")
			pkgJsonPath = filepath.Join(projectRoot, "package.json")
		}

		fmt.Printf("\n  %s Removing packages...\n", pterm.FgRed.Sprint("🗑"))

		removedCount := 0

		for _, pkgName := range args {
			// Resolve exact version from package.json if available (for vstore cleanup)
			resolvedVersion := ""
			if !global && pkgJsonPath != "" {
				if pkg, err := core.LoadPackageJson(pkgJsonPath); err == nil {
					all := pkg.AllDependencies()
					if req, ok := all[pkgName]; ok {
						resolvedVersion = req
					}
				}
			}

			pkgPath := filepath.Join(nmRoot, pkgName)
			if _, err := os.Lstat(pkgPath); err == nil {
				info, _ := os.Lstat(pkgPath)
				if info.Mode()&os.ModeSymlink != 0 {
					os.Remove(pkgPath)
				} else {
					os.RemoveAll(pkgPath)
				}

				// Clean up empty scope directory (for @scoped packages)
				if parent := filepath.Dir(pkgPath); parent != nmRoot {
					if entries, err := os.ReadDir(parent); err == nil && len(entries) == 0 {
						os.Remove(parent)
					}
				}

				// Clean up binary links from .bin
				if global {
					cleanupGlobalBinaries(paths.XpmHome(), pkgName)
				} else {
					cleanupBinaries(nmRoot, pkgName)
				}

				// Clean up from virtual store (deep clean matching version)
				vstoreRoot := paths.LocalVStoreDir(projectRoot)
				if global {
					vstoreRoot = paths.GlobalVStoreDir()
				}
				cleanupVirtualStore(vstoreRoot, pkgName, resolvedVersion)

				pterm.Printf("   %s Removed %s\n", pterm.FgRed.Sprint("-"), pterm.Bold.Sprint(pkgName))
				removedCount++
			} else {
				pterm.Printf("   %s Package %s not found in %s\n",
					pterm.FgYellow.Sprint("⚠"),
					pterm.FgDarkGray.Sprint(pkgName),
					nmRoot)
			}
		}

		// Update package.json
		if !global && pkgJsonPath != "" {
			updatePackageJsonRemove(pkgJsonPath, args)
			// Remove lockfile so it's regenerated cleanly on next install
			lockfile := filepath.Join(projectRoot, "xfpm.resolve.lock")
			os.Remove(lockfile)
			pterm.Printf("   %s Removed xfpm.resolve.lock (will regenerate on next install)\n", pterm.FgYellow.Sprint("🗑"))
		}

		fmt.Println()
		if removedCount > 0 {
			pterm.Printf("%s Uninstall complete (%d removed)\n", pterm.FgGreen.Sprint("✓"), removedCount)
		} else {
			pterm.Printf("%s Nothing to remove\n", pterm.FgGreen.Sprint("✓"))
		}
		utils.PrintFooter(0)
		return nil
	},
}

// cleanupVirtualStore removes entries from the virtual store that match the given package name
// and optionally a version constraint/prefix.
func cleanupVirtualStore(vstoreRoot, pkgName, versionHint string) {
	entries, err := os.ReadDir(vstoreRoot)
	if err != nil {
		return
	}
	// Normalize scoped packages: @scope/pkg -> +scope+pkg prefix in vstore
	prefix := strings.ReplaceAll(pkgName, "/", "+") + "@"
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		// If we have a version hint, only remove that version
		if versionHint != "" {
			// Strip leading ^ ~ >= etc.
			cleanVer := strings.TrimLeft(versionHint, "^~>=<")
			if !strings.HasSuffix(name, "@"+cleanVer) && !strings.Contains(name, "@"+cleanVer) {
				continue
			}
		}

		path := filepath.Join(vstoreRoot, name)
		pterm.Printf("   %s Pruning vstore: %s\n", pterm.FgMagenta.Sprint("⦿"), pterm.FgDarkGray.Sprint(name))
		os.RemoveAll(path)
	}
}

func cleanupBinaries(nmRoot, pkgName string) {
	binDir := filepath.Join(nmRoot, ".bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return
	}
	pattern := string(os.PathSeparator) + pkgName + string(os.PathSeparator)
	patternEnd := string(os.PathSeparator) + pkgName
	for _, entry := range entries {
		path := filepath.Join(binDir, entry.Name())
		target, err := os.Readlink(path)
		if err != nil {
			continue
		}
		if strings.Contains(target, pattern) || strings.Contains(target, ".."+string(os.PathSeparator)+pkgName+string(os.PathSeparator)) || strings.HasSuffix(target, patternEnd) {
			os.Remove(path)
		}
	}
}

func cleanupGlobalBinaries(globalRoot, pkgName string) {
	binDir := filepath.Join(globalRoot, "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return
	}
	pattern := fmt.Sprintf("%cnode_modules%c%s%c", os.PathSeparator, os.PathSeparator, pkgName, os.PathSeparator)
	patternEnd := fmt.Sprintf("%cnode_modules%c%s", os.PathSeparator, os.PathSeparator, pkgName)
	for _, entry := range entries {
		path := filepath.Join(binDir, entry.Name())
		target, err := os.Readlink(path)
		if err != nil {
			continue
		}
		if strings.Contains(target, pattern) || strings.HasSuffix(target, patternEnd) {
			os.Remove(path)
			pterm.Printf("   %s Removed binary link: %s\n", pterm.FgMagenta.Sprint("🔗"), entry.Name())
		}
	}
}

func updatePackageJsonRemove(path string, packages []string) {
	var removes []string
	for _, pkg := range packages {
		safeName := strings.ReplaceAll(pkg, ".", "\\.")
		removes = append(removes, 
			"dependencies."+safeName, 
			"devDependencies."+safeName, 
			"optionalDependencies."+safeName, 
			"peerDependencies."+safeName,
		)
	}

	if len(removes) > 0 {
		utils.RemoveFromJsonFile(path, removes)
		pterm.Printf("   %s Updated package.json\n", pterm.FgBlue.Sprint("✎"))
	}
}

func init() {
	RootCmd.AddCommand(uninstallCmd)
	uninstallCmd.Flags().BoolP("global", "g", false, "Uninstall from global store")
}
