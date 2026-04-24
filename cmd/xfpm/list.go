/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list [package...]",
	Short:   "List installed packages, or show dependents of several given packages",
	Aliases: []string{"ls", "ll"},
	RunE: func(cmd *cobra.Command, args []string) error {
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

		// If specific package names are given, show where they're used
		if len(args) > 0 {
			for i, target := range args {
				if i > 0 {
					fmt.Println()
					pterm.Println(pterm.FgDarkGray.Sprint("  " + strings.Repeat("─", 60)))
				}
				if err := listDependents(nmRoot, pkgJsonPath, target); err != nil {
					return err
				}
			}
			utils.PrintFooter(0)
			return nil
		}

		// Otherwise, list all direct dependencies from package.json
		if !global && pkgJsonPath != "" {
			return listAll(pkgJsonPath, nmRoot)
		}

		// Global mode: list everything in the global node_modules
		return listGlobal(nmRoot)
	},
}

// listAll lists all dependencies from package.json and whether they are installed.
func listAll(pkgJsonPath, nmRoot string) error {
	data, err := os.ReadFile(pkgJsonPath)
	if err != nil {
		return fmt.Errorf("could not read package.json: %w", err)
	}

	var raw struct {
		Name                 string            `json:"name"`
		Version              string            `json:"version"`
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	fmt.Printf("\n  %s %s %s\n\n",
		pterm.FgCyan.Sprint("[PROJECT]"),
		pterm.Bold.Sprint(raw.Name),
		pterm.FgDarkGray.Sprintf("v%s", raw.Version))

	printListSection("dependencies", raw.Dependencies, nmRoot)
	printListSection("devDependencies", raw.DevDependencies, nmRoot)
	printListSection("optionalDependencies", raw.OptionalDependencies, nmRoot)
	printListSection("peerDependencies", raw.PeerDependencies, nmRoot)

	fmt.Println()
	utils.PrintFooter(0)
	return nil
}

func printListSection(title string, deps map[string]string, nmRoot string) {
	if len(deps) == 0 {
		return
	}
	pterm.Printf("  %s\n", pterm.FgMagenta.Sprint(title))
	for name, req := range deps {
		installedVer := getInstalledVersion(nmRoot, name)
		statusIcon := pterm.FgGreen.Sprint("✔")
		verStr := pterm.FgGreen.Sprint(installedVer)
		if installedVer == "" {
			statusIcon = pterm.FgRed.Sprint("✘")
			verStr = pterm.FgDarkGray.Sprint("not installed")
		}
		pterm.Printf("    %s  %-40s %s  %s\n",
			statusIcon,
			pterm.Bold.Sprint(name),
			pterm.FgDarkGray.Sprint(req),
			verStr,
		)
	}
	fmt.Println()
}

// getInstalledVersion reads the package.json of a locally installed package to get its real version.
func getInstalledVersion(nmRoot, pkgName string) string {
	pkgJsonPath := filepath.Join(nmRoot, pkgName, "package.json")
	data, err := os.ReadFile(pkgJsonPath)
	if err != nil {
		return ""
	}
	var p struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}
	return p.Version
}

// listDependents finds all installed packages that depend on the target package.
func listDependents(nmRoot, localPkgJsonPath, target string) error {
	// Get installed version of target
	targetVersion := getInstalledVersion(nmRoot, target)

	if targetVersion == "" {
		pterm.Printf("\n  %s Package %s is %s in %s\n\n",
			pterm.FgYellow.Sprint("⚠"),
			pterm.Bold.Sprint(target),
			pterm.FgRed.Sprint("NOT installed"),
			nmRoot)
		return nil
	}

	pterm.Printf("\n  %s %s %s\n",
		pterm.FgCyan.Sprint("[PACKAGE]"),
		pterm.Bold.Sprint(target),
		pterm.FgGreen.Sprintf("v%s", targetVersion))
	pterm.Printf("  %s %s\n\n", pterm.FgDarkGray.Sprint("Location:"), filepath.Join(nmRoot, target))

	// Search all packages that declare target as a dependency
	dependents := []string{}

	// Check root package.json
	if localPkgJsonPath != "" {
		if data, err := os.ReadFile(localPkgJsonPath); err == nil {
			var raw map[string]json.RawMessage
			if json.Unmarshal(data, &raw) == nil {
				for _, section := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
					if raw[section] != nil {
						var deps map[string]string
						if json.Unmarshal(raw[section], &deps) == nil {
							if _, ok := deps[target]; ok {
								// Try to get name
								name := "(root package)"
								if raw["name"] != nil {
									_ = json.Unmarshal(raw["name"], &name)
								}
								dependents = append(dependents, fmt.Sprintf("%s %s", pterm.FgCyan.Sprint(name), pterm.FgDarkGray.Sprint("[root]")))
								break
							}
						}
					}
				}
			}
		}
	}

	// Scan all installed packages in node_modules
	entries, err := os.ReadDir(nmRoot)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(nmRoot, entry.Name()))
		if err != nil {
			continue
		}
		if (!info.IsDir() && info.Mode()&os.ModeSymlink == 0) || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		entryName := entry.Name()

		// Handle scoped packages (@scope/pkg)
		if strings.HasPrefix(entryName, "@") {
			sub, _ := os.ReadDir(filepath.Join(nmRoot, entryName))
			for _, s := range sub {
				fullName := entryName + "/" + s.Name()
				checkDependent(nmRoot, fullName, target, &dependents)
			}
			continue
		}

		checkDependent(nmRoot, entryName, target, &dependents)
	}

	if len(dependents) == 0 {
		pterm.Printf("  %s No packages depend on %s\n\n", pterm.FgDarkGray.Sprint("─"), target)
	} else {
		pterm.Printf("  %s %s %s:\n\n", pterm.FgCyan.Sprint("[DEPENDENTS]"), pterm.Bold.Sprint(target), pterm.FgDarkGray.Sprintf("(%d)", len(dependents)))
		for _, d := range dependents {
			pterm.Printf("    %s %s\n", pterm.FgGreen.Sprint("├─"), d)
		}
		fmt.Println()
	}


	return nil
}

func checkDependent(nmRoot, pkgName, target string, dependents *[]string) {
	pkgJsonPath := filepath.Join(nmRoot, pkgName, "package.json")
	data, err := os.ReadFile(pkgJsonPath)
	if err != nil {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	for _, section := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
		if raw[section] == nil {
			continue
		}
		var deps map[string]string
		if json.Unmarshal(raw[section], &deps) != nil {
			continue
		}
		if req, ok := deps[target]; ok {
			installedVer := getInstalledVersion(nmRoot, pkgName)
			entry := fmt.Sprintf("%s%s  %s",
				pterm.Bold.Sprint(pkgName),
				pterm.FgDarkGray.Sprintf(" v%s", installedVer),
				pterm.FgDarkGray.Sprintf("(requires: %s)", req))
			*dependents = append(*dependents, entry)
			return
		}
	}
}

func listGlobal(nmRoot string) error {
	entries, err := os.ReadDir(nmRoot)
	if err != nil {
		return fmt.Errorf("global node_modules not found: %s", nmRoot)
	}

	pterm.Printf("\n  %s %s\n\n", pterm.FgCyan.Sprint("[GLOBAL]"), pterm.FgDarkGray.Sprint(nmRoot))

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "@") {
			sub, _ := os.ReadDir(filepath.Join(nmRoot, name))
			for _, s := range sub {
				fullName := name + "/" + s.Name()
				ver := getInstalledVersion(nmRoot, fullName)
				pterm.Printf("    %s  %-40s %s\n", pterm.FgGreen.Sprint("✔"), pterm.Bold.Sprint(fullName), pterm.FgGreen.Sprint(ver))
				count++
			}
			continue
		}
		ver := getInstalledVersion(nmRoot, name)
		pterm.Printf("    %s  %-40s %s\n", pterm.FgGreen.Sprint("✔"), pterm.Bold.Sprint(name), pterm.FgGreen.Sprint(ver))
		count++
	}

	fmt.Printf("\n  %s %d packages installed globally\n\n", pterm.FgCyan.Sprint("→"), count)
	utils.PrintFooter(0)
	return nil
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolP("global", "g", false, "List globally installed packages")
}
