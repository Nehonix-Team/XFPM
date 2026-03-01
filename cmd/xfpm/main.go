/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 *
 * @author Nehonix
 * @license Nehonix OSL (NOSL)
 *
 * Copyright (c) 2025 Nehonix. All rights reserved.
 *
 * This License governs the use, modification, and distribution of software
 * provided by NEHONIX under its open source projects.
 * NEHONIX is committed to fostering collaborative innovation while strictly
 * protecting its intellectual property rights.
 * Violation of any term of this License will result in immediate termination of all granted rights
 * and may subject the violator to legal action.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED,
 * INCLUDING BUT NOT LIMITED TO WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE,
 * AND NON-INFRINGEMENT.
 * IN NO EVENT SHALL NEHONIX BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY,
 * OR CONSEQUENTIAL DAMAGES ARISING FROM THE USE OR INABILITY TO USE THE SOFTWARE,
 * EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGES.
 *
 ***************************************************************************** */

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nehonix-Team/xfpm-go/internal/core"
	"github.com/Nehonix-Team/xfpm-go/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	version = "0.1.0-go"
	rootCmd = &cobra.Command{
		Use:   "xfpm",
		Short: "Official XyPriss Fast Package Manager & CLI (Go version)",
		Long:  `Official XyPriss Fast Package Manager & CLI rewritten in Go for cross-platform support.`,
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			utils.SetupUI()
			utils.CheckForUpdates()
		},
		Run: func(cmd *cobra.Command, args []string) {
			utils.PrintBanner()
			cmd.Help()
		},
	}
)

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.PersistentFlags().StringP("cwd", "C", "", "Change work directory")
	installCmd.Flags().BoolP("save-dev", "D", false, "Save to devDependencies")
	installCmd.Flags().BoolP("save-optional", "O", false, "Save to optionalDependencies")
	installCmd.Flags().BoolP("force", "f", false, "Force re-install even if already extracted")
	installCmd.Flags().BoolP("global", "g", false, "Install packages globally")
}

var installCmd = &cobra.Command{
	Use:   "install [packages...]",
	Short: "Install dependencies for the current project",
	Aliases: []string{"i", "add"},
	RunE: func(cmd *cobra.Command, args []string) error {
		startTime := time.Now()
		cwd, _ := cmd.Flags().GetString("cwd")
		if cwd != "" {
			if err := os.Chdir(cwd); err != nil {
				return err
			}
		}

		global, _ := cmd.Flags().GetBool("global")

		var projectRoot string
		var err error
		if global {
			home, _ := os.UserHomeDir()
			projectRoot = filepath.Join(home, ".xpm_global")
			os.MkdirAll(projectRoot, 0755)
		} else {
			projectRoot, err = os.Getwd()
			if err != nil {
				return err
			}
		}

		isDev, _ := cmd.Flags().GetBool("save-dev")
		isOptional, _ := cmd.Flags().GetBool("save-optional")
		force, _ := cmd.Flags().GetBool("force")

		xpmDir := filepath.Join(projectRoot, "node_modules", ".xpm")
		cas, err := core.NewCas(filepath.Join(xpmDir, "storage"))
		if err != nil {
			return err
		}

		registry := core.NewRegistryClient("", 3)
		registry.SetCacheDir(filepath.Join(xpmDir, "cache"))

		resolver := core.NewResolver(registry, cas)

		utils.Matrix("Analysing project roots...")

		rootDeps := make(map[string]string)
		directPkgs := make(map[string]string)
		pkgJsonPath := filepath.Join(projectRoot, "package.json")
		var pkg *core.PackageJson

		if _, err := os.Stat(pkgJsonPath); err == nil {
			pkg, _ = core.LoadPackageJson(pkgJsonPath)
			if pkg != nil {
				for name, req := range pkg.AllDependencies() {
					rootDeps[name] = req
				}
			}
		}

		if len(args) > 0 {
			resolver.ForcePackages = make(map[string]bool)
			for _, p := range args {
				parts := strings.Split(p, "@")
				name := p
				req := "latest"
				if len(parts) == 2 {
					name = parts[0]
					req = parts[1]
				} else if len(parts) == 3 && strings.HasPrefix(p, "@") {
					name = "@" + parts[1]
					req = parts[2]
				}
				rootDeps[name] = req
				directPkgs[name] = req
				resolver.ForcePackages[name] = true
			}
		} else if force {
			// updateCmd sets force=true but if args is empty, it's a global update
			resolver.Update = true
			for k, v := range rootDeps {
				directPkgs[k] = v
			}
		} else {
			for k, v := range rootDeps {
				directPkgs[k] = v
			}
		}

		if len(rootDeps) == 0 {
			return fmt.Errorf("no package.json found and no packages specified")
		}

		s, _ := pterm.DefaultSpinner.
			WithRemoveWhenDone(true).
			WithText("Analysing project tree...").
			Start()
		var mu sync.Mutex
		lastResolving := ""

		resolver.SetOnProgress(func(name string) {
			mu.Lock()
			lastResolving = name
			mu.Unlock()
		})

		stopSpinner := make(chan struct{})
		go func() {
			ticker := time.NewTicker(80 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					mu.Lock()
					name := lastResolving
					mu.Unlock()
					if name != "" {
						s.UpdateText("  " + utils.DimColor.Sprint("Searching: ") + utils.AccentColor.Sprint(name) + " " + utils.MatrixColor.Sprint(utils.RandomMatrixString(5)))
					}
				case <-stopSpinner:
					return
				}
			}
		}()
		
		resolved, rootVersions, err := resolver.ResolveTree(context.Background(), rootDeps)
		close(stopSpinner)
		s.Stop()

		if err != nil {
			return err
		}

		utils.Success("Dependency tree resolved successfully (%d total).", len(resolved))

		installer := core.NewInstaller(cas, registry, projectRoot)
		installer.Force = force
		installer.ForcePackages = resolver.ForcePackages
		installer.IsGlobal = global
		installer.DirectDeps = rootVersions
		if err := installer.Install(context.Background(), resolved); err != nil {
			return err
		}
		// Update package.json
		if pkg != nil && len(directPkgs) > 0 {
			for name := range directPkgs {
				// Find actual version in resolved
				version := "latest"
				for _, r := range resolved {
					if r.Name == name {
						version = "^" + r.Version
						break
					}
				}

				// If len(args) == 0, we don't move packages between sections.
				// We just find where it exists and update it there.
				if len(args) == 0 {
					if _, ok := pkg.Dependencies[name]; ok {
						pkg.Dependencies[name] = version
					} else if _, ok := pkg.DevDependencies[name]; ok {
						pkg.DevDependencies[name] = version
					} else if _, ok := pkg.OptionalDependencies[name]; ok {
						pkg.OptionalDependencies[name] = version
					} else if _, ok := pkg.PeerDependencies[name]; ok {
						pkg.PeerDependencies[name] = version
					}
				} else {
					// Specific package provided via command line: Remove from everywhere first
					delete(pkg.Dependencies, name)
					delete(pkg.DevDependencies, name)
					delete(pkg.OptionalDependencies, name)
					delete(pkg.PeerDependencies, name)

					if isDev {
						if pkg.DevDependencies == nil {
							pkg.DevDependencies = make(map[string]string)
						}
						pkg.DevDependencies[name] = version
					} else if isOptional {
						if pkg.OptionalDependencies == nil {
							pkg.OptionalDependencies = make(map[string]string)
						}
						pkg.OptionalDependencies[name] = version
					} else {
						if pkg.Dependencies == nil {
							pkg.Dependencies = make(map[string]string)
						}
						pkg.Dependencies[name] = version
					}
				}
			}
			pkg.Save(pkgJsonPath)
		}

		if len(args) == 1 {
			pkgName := args[0]
			if strings.HasPrefix(pkgName, "@") {
				parts := strings.Split(pkgName, "@")
				if len(parts) > 1 {
					pkgName = "@" + parts[1]
				}
			} else {
				pkgName = strings.Split(pkgName, "@")[0]
			}
			
			if v, ok := rootVersions[pkgName]; ok {
				pterm.Println()
				utils.Premium("Success", fmt.Sprintf("%s@%s installed successfully", pkgName, v))
			}
		}

		utils.PrintFooter(time.Since(startTime))
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
