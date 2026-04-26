/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

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
			projectRoot = paths.GlobalsDir()
			utils.CreateDirAllSecure(projectRoot)
		} else {
			projectRoot, err = os.Getwd()
			if err != nil {
				return err
			}
		}

		// Legacy storage detection and migration prompt
		if !global && core.HasLegacyStorage(projectRoot) {
			pterm.DefaultBox.
				WithTitle(pterm.LightYellow(" LEGACY STORE DETECTED ")).
				WithTitleBottomRight().
				Printfln("This project uses an old version of storage.\nThe new version is now optimized to save gigabytes of space.\n\nTo clean up all legacy projects, run: %s", pterm.LightCyan("xfpm store prune --legacy"))

			options := []string{
				"Migrate this project now (Fast)",
				"Migrate and search for ALL other legacy projects on my machine",
				"Skip (I'll run 'xfpm store prune --legacy' later)",
			}
			
			selected, _ := pterm.DefaultInteractiveSelect.
				WithOptions(options).
				WithDefaultText("What would you like to do?").
				Show()

			xpmStore := paths.StorageDir()
			cas, _ := core.NewCas(xpmStore)

			if selected == options[0] {
				pb, _ := pterm.DefaultProgressbar.
					WithTotal(0).
					WithTitle("  " + pterm.Gray("->") + " Migrating files").
					Start()

				core.MigrateLegacyStorage(projectRoot, cas, func(current, total int, message string) {
					pb.Total = total
					pb.Add(1)
					pb.Title = "  " + pterm.Gray("->") + " Migrated: " + pterm.LightCyan(message)
				})

				pb.Stop()
				pterm.Success.Println("Project migrated successfully!")
			} else if selected == options[1] {
				// Search everything from HOME
				home, _ := os.UserHomeDir()
				pterm.Info.Printfln("Searching for legacy stores in %s... (this might take a few seconds)", home)
				s, _ := pterm.DefaultSpinner.Start("Scanning deep...")
				roots := core.FindLegacyStorages(home)
				s.Stop()
				
				if len(roots) > 0 {
					pterm.Info.Printfln("Found %d legacy projects. Starting mass migration...", len(roots))
					for _, r := range roots {
						pterm.DefaultSection.Printf("Migrating %s", r)
						
						pb, _ := pterm.DefaultProgressbar.
							WithTotal(0).
							WithTitle("  " + pterm.Gray("->") + " Moving files").
							Start()

						core.MigrateLegacyStorage(r, cas, func(current, total int, message string) {
							pb.Total = total
							pb.Add(1)
							pb.Title = "  " + pterm.Gray("->") + " Migrated: " + pterm.LightCyan(message)
						})
						pb.Stop()
					}
					pterm.Success.Println("All projects migrated!")
				} else {
					pterm.Info.Println("No other legacy projects found.")
				}
			}
		}

		var (
			isDev, isOptional, isPeer, force, update bool
		)

		isDev, _ = cmd.Flags().GetBool("save-dev")
		isOptional, _ = cmd.Flags().GetBool("save-optional")
		isPeer, _ = cmd.Flags().GetBool("save-peer")
		force, _ = cmd.Flags().GetBool("force")
		update, _ = cmd.Flags().GetBool("update")

		xpmStore := paths.StorageDir()
		
		cas, err := core.NewCas(xpmStore)
		if err != nil {
			return err
		}

		xpmDir := paths.LocalXpmDir(projectRoot)
		registry := core.NewRegistryClient("", 3)
		registry.SetCacheDir(paths.RegistryCacheDir(xpmDir))

		resolver := core.NewResolver(registry, cas)

		utils.Matrix("Analysing project roots...")

		rootDeps := make(map[string]string)
		directPkgs := make(map[string]string)
		hasLatest := false
		
		// Map for local packages: name -> absolute path
		localPackages := make(map[string]string)

		pkgJsonPath := filepath.Join(projectRoot, "package.json")
		var pkg *core.PackageJson

		if _, err := os.Stat(pkgJsonPath); err == nil {
			pkg, _ = core.LoadPackageJson(pkgJsonPath)
			if pkg != nil {
				for name, req := range pkg.AllDependencies() {
					if strings.HasPrefix(req, "file:") {
						path := strings.TrimPrefix(req, "file:")
						absPath := path
						if !filepath.IsAbs(path) {
							absPath = filepath.Join(projectRoot, path)
						}
						if localPkg, err := core.LoadPackageJson(filepath.Join(absPath, "package.json")); err == nil {
							localPackages[localPkg.Name] = absPath
							directPkgs[localPkg.Name] = req
							if resolver.ForcePackages == nil {
								resolver.ForcePackages = make(map[string]bool)
							}
							resolver.ForcePackages[localPkg.Name] = true
							rootDeps[name] = localPkg.Version // Use version for resolution, not the file: string
							continue
						}
					}
					rootDeps[name] = req
					if req == "latest" {
						hasLatest = true
					}
				}
			}
		} else if len(args) > 0 {
			// Initialize a default package.json if it doesn't exist and we are adding packages
			pkg = &core.PackageJson{
				Name:         filepath.Base(projectRoot),
				Version:      "1.0.0",
				Dependencies: make(map[string]string),
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

		hasRedirects := len(resolver.Redirects) > 0
		for src, dest := range resolver.Redirects {
			if rv, ok := rootVersions[src]; ok {
				rootVersions[dest] = rv
				delete(rootVersions, src)
			}
			if dv, ok := directPkgs[src]; ok {
				directPkgs[dest] = dv
				delete(directPkgs, src)
			}
			if pkg != nil {
				if _, ok := pkg.Dependencies[src]; ok {
					delete(pkg.Dependencies, src)
					if pkg.Dependencies == nil { pkg.Dependencies = make(map[string]string) }
					pkg.Dependencies[dest] = "latest"
				} else if _, ok := pkg.DevDependencies[src]; ok {
					delete(pkg.DevDependencies, src)
					if pkg.DevDependencies == nil { pkg.DevDependencies = make(map[string]string) }
					pkg.DevDependencies[dest] = "latest"
				} else if _, ok := pkg.OptionalDependencies[src]; ok {
					delete(pkg.OptionalDependencies, src)
					if pkg.OptionalDependencies == nil { pkg.OptionalDependencies = make(map[string]string) }
					pkg.OptionalDependencies[dest] = "latest"
				} else if _, ok := pkg.PeerDependencies[src]; ok {
					delete(pkg.PeerDependencies, src)
					if pkg.PeerDependencies == nil { pkg.PeerDependencies = make(map[string]string) }
					pkg.PeerDependencies[dest] = "latest"
				}
			}
		}

		utils.Success("Dependency tree resolved successfully (%d total).", len(resolved))

		// targetedPackages for lifecycle scripts
		var targetedPackages map[string]bool
		if len(args) > 0 || update {
			targetedPackages = make(map[string]bool)
			// Start with direct packages
			for name := range directPkgs {
				targetedPackages[name] = true
			}
			// Recursive walk to include transitive dependencies of targeted packages
			var walk func(pkgName string)
			walk = func(pkgName string) {
				for _, p := range resolved {
					if p.Name == pkgName {
						for depName := range p.ResolvedDependencies {
							if !targetedPackages[depName] {
								targetedPackages[depName] = true
								walk(depName)
							}
						}
						break
					}
				}
			}
			for name := range directPkgs {
				walk(name)
			}
		}

		installer := core.NewInstaller(cas, registry, projectRoot)
		installer.Force = force
		installer.ForcePackages = resolver.ForcePackages
		installer.IsGlobal = global
		installer.DirectDeps = rootVersions
		installer.TargetPackages = targetedPackages

		autoVerify, _ := cmd.Flags().GetBool("verify")
		noInteract, _ := cmd.Flags().GetBool("no-interact")
		installer.AutoVerify = autoVerify
		installer.NoInteract = noInteract

		if err := installer.Install(context.Background(), resolved); err != nil {
			return err
		}
		// Update package.json
		if pkg != nil && len(directPkgs) > 0 && (len(args) > 0 || update || hasRedirects || hasLatest) {
			for name := range directPkgs {
				version := "latest"
				if v, ok := rootVersions[name]; ok {
					version = "^" + v
				}

				origReq := ""
				if r, ok := rootDeps[name]; ok { origReq = r }
				if strings.HasPrefix(origReq, "file:") || strings.HasPrefix(origReq, "workspace:") {
					version = origReq
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
					var targetSection string

					if isDev {
						targetSection = "dev"
					} else if isOptional {
						targetSection = "optional"
					} else if isPeer {
						targetSection = "peer"
					} else {
						// determine original section
						if _, ok := pkg.Dependencies[name]; ok {
							targetSection = "dep"
						} else if _, ok := pkg.DevDependencies[name]; ok {
							targetSection = "dev"
						} else if _, ok := pkg.OptionalDependencies[name]; ok {
							targetSection = "optional"
						} else if _, ok := pkg.PeerDependencies[name]; ok {
							targetSection = "peer"
						} else {
							targetSection = "dep" // default
						}
					}

					delete(pkg.Dependencies, name)
					delete(pkg.DevDependencies, name)
					delete(pkg.OptionalDependencies, name)
					delete(pkg.PeerDependencies, name)

					switch targetSection {
					case "dev":
						if pkg.DevDependencies == nil { pkg.DevDependencies = make(map[string]string) }
						pkg.DevDependencies[name] = version
					case "optional":
						if pkg.OptionalDependencies == nil { pkg.OptionalDependencies = make(map[string]string) }
						pkg.OptionalDependencies[name] = version
					case "peer":
						if pkg.PeerDependencies == nil { pkg.PeerDependencies = make(map[string]string) }
						pkg.PeerDependencies[name] = version
					default:
						if pkg.Dependencies == nil { pkg.Dependencies = make(map[string]string) }
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

func init() {
	RootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolP("save-dev", "D", false, "Save to devDependencies")
	installCmd.Flags().BoolP("save-optional", "O", false, "Save to optionalDependencies")
	installCmd.Flags().BoolP("save-peer", "R", false, "Save to peerDependencies")
	installCmd.Flags().BoolP("force", "f", false, "Force re-install even if already extracted")
	installCmd.Flags().Bool("update", false, "Update mode: always fetch fresh metadata from registry")
	installCmd.Flags().BoolP("global", "g", false, "Install packages globally")
	installCmd.Flags().StringP("path", "P", "", "Install from a local path")
	installCmd.Flags().BoolP("verify", "v", false, "Automatically verify plugins during installation")
	installCmd.Flags().BoolP("no-interact", "n", false, "Disable interactive prompts and auto-verify valid signatures")
}
