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
	rootCmd = &cobra.Command{
		Use:   "xfpm",
		Short: "Official XyPriss Fast Package Manager & CLI (Go version)",
		Long:  `Official XyPriss Fast Package Manager (XFPM). XFPM is a high-performance, cross-platform CLI tool built for the XyPriss ecosystem. Written in Go, it delivers fast dependency resolution, strict package isolation through a virtual store, and a clean terminal interface designed for professional workflows.`,
		Version: utils.BinVersion,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			utils.SetupUI()
			
			cwd, _ := cmd.Flags().GetString("cwd")
			if cwd != "" {
				if err := os.Chdir(cwd); err != nil {
					return fmt.Errorf("failed to change directory to %s: %w", cwd, err)
				}
			}

			utils.CheckForUpdates()
			return nil
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
	installCmd.Flags().Bool("update", false, "Update mode: always fetch fresh metadata from registry")
	installCmd.Flags().BoolP("global", "g", false, "Install packages globally")
	installCmd.Flags().StringP("path", "P", "", "Install from a local path")
}


var installCmd = &cobra.Command{
	Use:   "install [packages...]",
	Short: "Install dependencies for the current project",
	Aliases: []string{"i", "add"},
	RunE: func(cmd *cobra.Command, args []string) error {
		startTime := time.Now()
		global, _ := cmd.Flags().GetBool("global")
		localPath, _ := cmd.Flags().GetString("path")

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
		update, _ := cmd.Flags().GetBool("update")

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
		
		// Map for local packages: name -> absolute path
		localPackages := make(map[string]string)

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

		// Handle --path flag
		if localPath != "" {
			absPath, err := filepath.Abs(localPath)
			if err == nil {
				localPkg, err := core.LoadPackageJson(filepath.Join(absPath, "package.json"))
				if err == nil {
					localPackages[localPkg.Name] = absPath
					rootDeps[localPkg.Name] = localPkg.Version
					directPkgs[localPkg.Name] = localPkg.Version
					if resolver.ForcePackages == nil {
						resolver.ForcePackages = make(map[string]bool)
					}
					resolver.ForcePackages[localPkg.Name] = true
				}
			}
		}

		if len(args) > 0 {
			if resolver.ForcePackages == nil {
				resolver.ForcePackages = make(map[string]bool)
			}
			for _, p := range args {
				// Check if p is a local path
				if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") || filepath.IsAbs(p) {
					absPath, err := filepath.Abs(p)
					if err == nil {
						if fi, err := os.Stat(absPath); err == nil && fi.IsDir() {
							localPkg, err := core.LoadPackageJson(filepath.Join(absPath, "package.json"))
							if err == nil {
								localPackages[localPkg.Name] = absPath
								rootDeps[localPkg.Name] = localPkg.Version
								directPkgs[localPkg.Name] = localPkg.Version
								resolver.ForcePackages[localPkg.Name] = true
								continue
							}
						}
					}
				}

				name := p
				req := ""
				
				if strings.HasPrefix(p, "@") {
					// Scoped package: @scope/pkg or @scope/pkg@version
					atIdx := strings.LastIndex(p, "@")
					if atIdx > 0 {
						name = p[:atIdx]
						req = p[atIdx+1:]
					}
				} else {
					// Normal package: pkg or pkg@version
					atIdx := strings.Index(p, "@")
					if atIdx != -1 {
						name = p[:atIdx]
						req = p[atIdx+1:]
					}
				}

				if req == "" {
					req = "latest"
				}
				
				rootDeps[name] = req
				directPkgs[name] = req
				resolver.ForcePackages[name] = true
			}
		} else if update {
			// Global update mode: fetch all fresh
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

		resolver.SetLocalPackages(localPackages)

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
		startTimeMatrix := time.Now()
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
						elapsed := time.Since(startTimeMatrix).Truncate(time.Millisecond)
						s.UpdateText("  " + utils.DimColor.Sprint("Searching: ") + utils.AccentColor.Sprint(name) + "  " + utils.DimColor.Sprint(elapsed.String()))
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

		for _, p := range resolved {
			if path, ok := localPackages[p.Name]; ok {
				p.LocalPath = path
			}
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
		if pkg != nil && len(directPkgs) > 0 && (len(args) > 0 || update) {
			for name := range directPkgs {
				version := "latest"
				if v, ok := rootVersions[name]; ok {
					version = "^" + v
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
		utils.Error("%v", err)
		os.Exit(1)
	}
}
