package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/plugin"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	verifyNoInteract bool
	verifyHtml       bool
	revokeNoPending  bool
	listLocal        bool
	listReview       bool
)

var pluginCmd = &cobra.Command{
	Use:     "plugin",
	Aliases: []string{"plugins", "pl", "v"},
	Short:   "Manage project plugins and security trust",
}

var pluginVerifyCmd = &cobra.Command{
	Use:     "verify",
	Aliases: []string{"resolve", "v"},
	Short:   "Verify and finalize pending plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		noInteract, _ := cmd.Flags().GetBool("no-interact")
		projectRoot, _ := os.Getwd()
		pendingPath := paths.PendingPluginsPath(projectRoot)

		if _, err := os.Stat(pendingPath); os.IsNotExist(err) {
			utils.Info("No pending plugins to verify.")
			return nil
		}

		data, err := os.ReadFile(pendingPath)
		if err != nil { return err }

		var pending []map[string]string
		if err := json.Unmarshal(data, &pending); err != nil { return err }

		if len(pending) == 0 {
			utils.Info("No pending plugins to verify.")
			return nil
		}

		cas, _ := core.NewCas(paths.StorageDir())
		registry := core.NewRegistryClient("https://registry.npmjs.org", 8)
		installer := core.NewInstaller(cas, registry, projectRoot)
		installer.NoInteract = noInteract

		// --- BATCH UI PROMPT FOR PRIVILEGES ---
		if !noInteract {
			var requiresPrompt []plugin.PendingReq

			configPath := paths.ConfigPath(projectRoot)
			var config map[string]interface{}
			var cleanLines []string
			
			if cfgBytes, err := os.ReadFile(configPath); err == nil {
				lines := strings.Split(string(cfgBytes), "\n")
				for _, line := range lines {
					if idx := strings.Index(line, "//"); idx != -1 {
						line = line[:idx]
					}
					cleanLines = append(cleanLines, line)
				}
				configStr := strings.Join(cleanLines, "\n")
				re := regexp.MustCompile(`,(\s*[}\]])`)
				configStr = re.ReplaceAllString(configStr, "$1")
				json.Unmarshal([]byte(configStr), &config)
			}
			if config == nil { config = make(map[string]interface{}) }

			var trustedPlugins []string
			if pkgJson, err := core.LoadPackageJson(paths.PackageJsonPath(projectRoot)); err == nil && pkgJson != nil {
				if pkgJson.Xfpm != nil {
					trustedPlugins = pkgJson.Xfpm.TrustedPlugins
				}
			}

			isTrustedInPkgJson := func(name string) bool {
				for _, t := range trustedPlugins {
					if t == name { return true }
				}
				return false
			}

			for _, p := range pending {
				pkgName := p["name"]
				pkgVer := p["version"]
				pkgDir := paths.PackageVStoreDir(projectRoot, pkgName, pkgVer)
				sigPath := paths.PackageSigPath(pkgDir)

				sigBytes, err := os.ReadFile(sigPath)
				if err != nil {
					// Fallback to CAS if possible
					index, errGet := cas.GetIndex(pkgName, pkgVer)
					if errGet == nil && index != nil {
						for path, hash := range index {
							if strings.HasSuffix(path, "xypriss.plugin.xsig") {
								sigBytes, _ = os.ReadFile(cas.GetFilePath(hash))
								break
							}
						}
					}
				}

				if len(sigBytes) == 0 { continue } // Handled later by VerifySignatureInternal

				identity := plugin.ExtractIdentity(sigBytes)
				privs := plugin.ExtractPrivileges(sigBytes)

				trusted := isTrustedInPkgJson(pkgName)
				if !trusted {
					if internal, ok := config["$internal"].(map[string]interface{}); ok {
						if pluginCfg, ok := internal[pkgName].(map[string]interface{}); ok {
							if sigCfg, ok := pluginCfg["signature"].(map[string]interface{}); ok {
								if ak, ok := sigCfg["author_key"].(string); ok && ak == identity {
									trusted = true
								}
							}
						}
					}
				}

				if !trusted && identity != "" {
					status := "pending"
					pkgDir := paths.NodeModulesPkgDir(projectRoot, pkgName)
					if _, err := os.Stat(pkgDir); os.IsNotExist(err) && status != "authorized" {
						status = "NOT_INSTALLED"
					}

					requiresPrompt = append(requiresPrompt, plugin.PendingReq{
						Name:       pkgName,
						Version:    pkgVer,
						Identity:   identity,
						Privileges: privs,
						Status:     status,
					})
				}
			}

			if len(requiresPrompt) > 0 {
				if verifyHtml {
					plugin.HandleHtmlVerify(projectRoot, requiresPrompt, config, configPath, false)
					
					// IMPORTANT: Reload config after web UI session finishes to see updated trust/permissions
					if b, err := os.ReadFile(configPath); err == nil {
						lines := strings.Split(string(b), "\n")
						var cleanLines []string
						for _, l := range lines {
							if idx := strings.Index(l, "//"); idx != -1 { l = l[:idx] }
							cleanLines = append(cleanLines, l)
						}
						configStr := strings.Join(cleanLines, "\n")
						re := regexp.MustCompile(`,(\s*[}\]])`)
						configStr = re.ReplaceAllString(configStr, "$1")
						json.Unmarshal([]byte(configStr), &config)
					}
				} else {
					pterm.Println()
					pterm.Warning.Println("The following plugins require your explicit trust and authorization:")
					pterm.Println()

					// Use interactive multiselect for more granular control
					var options []string
					for _, req := range requiresPrompt {
						options = append(options, req.Name)
					}

					selected, _ := pterm.DefaultInteractiveMultiselect.
						WithOptions(options).
						WithDefaultText("Select plugins to trust and authorize").
						Show()

					if len(selected) > 0 {
						internalRaw, ok := config["$internal"]
						if !ok { internalRaw = make(map[string]interface{}) }
						internal, ok := internalRaw.(map[string]interface{})
						if !ok { internal = make(map[string]interface{}) }

						selectedSet := make(map[string]bool)
						for _, s := range selected { selectedSet[s] = true }

						for _, req := range requiresPrompt {
							if !selectedSet[req.Name] { continue }

							pluginCfgRaw, ok := internal[req.Name]
							if !ok { pluginCfgRaw = make(map[string]interface{}) }
							pluginCfg, ok := pluginCfgRaw.(map[string]interface{})
							if !ok { pluginCfg = make(map[string]interface{}) }

							sigCfgRaw, ok := pluginCfg["signature"]
							if !ok { sigCfgRaw = make(map[string]interface{}) }
							sigCfg, ok := sigCfgRaw.(map[string]interface{})
							if !ok { sigCfg = make(map[string]interface{}) }
							sigCfg["author_key"] = req.Identity
							pluginCfg["signature"] = sigCfg

							if req.Privileges != "" && req.Privileges != "none" {
								permCfgRaw, ok := pluginCfg["permissions"]
								if !ok { permCfgRaw = make(map[string]interface{}) }
								permCfg, ok := permCfgRaw.(map[string]interface{})
								if !ok { permCfg = make(map[string]interface{}) }
								permCfg["allowedHooks"] = strings.Split(req.Privileges, ",")
								pluginCfg["permissions"] = permCfg
							}
							internal[req.Name] = pluginCfg
						}
						config["$internal"] = internal

						if out, err := json.MarshalIndent(config, "", "    "); err == nil {
							os.WriteFile(configPath, out, 0644)
							utils.Success("Authorization granted for %d plugin(s).", len(selected))
						}
					} else {
						utils.Info("No plugins selected for authorization.")
					}
				}
			}
		}

		for _, p := range pending {
			// pterm.Print("\033[H\033[2J") // Clear
			pkgName := p["name"]
			pkgVer := p["version"]
			pkg := &core.ResolvedPackage{Name: pkgName, Version: pkgVer}
			pkgDir := paths.PackageVStoreDir(projectRoot, pkgName, pkgVer)
			sigPath := paths.PackageSigPath(pkgDir)

			index, err := cas.GetIndex(pkgName, pkgVer)
			if err != nil || index == nil {
				utils.Error("Package index not found in CAS for %s. Please re-run install.", pkgName)
				continue
			}

			if err := installer.VerifySignatureInternal(sigPath, pkg, index); err != nil {
				return err
			}

			utils.Info("Finalizing installation for %s@%s...", pkgName, pkgVer)
			installer.LinkFilesToDir(pkgDir, index)
			rootDest := paths.NodeModulesPkgDir(projectRoot, pkgName)
			utils.LinkDir(pkgDir, rootDest)
			utils.Success("Plugin %s@%s fully installed.", pkgName, pkgVer)
		}

		os.Remove(pendingPath)
		utils.Success("All pending plugins verified and installed.")
		return nil
	},
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all project plugins and their trust status",
	RunE: func(cmd *cobra.Command, args []string) error {
		localOnly, _ := cmd.Flags().GetBool("local")
		reviewMode, _ := cmd.Flags().GetBool("review")
		projectRoot, _ := os.Getwd()

		pkgJsonPath := paths.PackageJsonPath(projectRoot)
		pkg, err := core.LoadPackageJson(pkgJsonPath)
		if err != nil {
			return fmt.Errorf("failed to load package.json: %w", err)
		}

		configPath := paths.ConfigPath(projectRoot)
		var config map[string]interface{}
		if b, err := os.ReadFile(configPath); err == nil {
			// Strip comments if it's jsonc
			lines := strings.Split(string(b), "\n")
			var clean []string
			for _, l := range lines {
				if idx := strings.Index(l, "//"); idx != -1 {
					l = l[:idx]
				}
				clean = append(clean, l)
			}
			json.Unmarshal([]byte(strings.Join(clean, "\n")), &config)
		}
		if config == nil { config = make(map[string]interface{}) }

		pendingPath := paths.PendingPluginsPath(projectRoot)
		var pendingList []map[string]string
		if b, err := os.ReadFile(pendingPath); err == nil {
			json.Unmarshal(b, &pendingList)
		}

		utils.Matrix("Analyzing project dependencies for plugins...")
		s, _ := pterm.DefaultSpinner.WithRemoveWhenDone(true).WithText("Scanning dependencies...").Start()

		allDeps := make(map[string]string)
		// Include both dependencies and devDependencies
		for k, v := range pkg.Dependencies { allDeps[k] = v }
		for k, v := range pkg.DevDependencies { allDeps[k] = v }

		// In review mode, also include orphans present in config
		if reviewMode {
			if internal, ok := config["$internal"].(map[string]interface{}); ok {
				for name := range internal {
					if _, exists := allDeps[name]; !exists {
						allDeps[name] = "latest" // unknown version range, use latest
					}
				}
			}
		}

		registry := core.NewRegistryClient("https://registry.npmjs.org", 8)
		
		var items [][]string
		items = append(items, []string{"Plugin", "Version", "Status", "Developer ID"})

		var wg sync.WaitGroup
		var mu sync.Mutex
		sem := make(chan struct{}, 32)

		var reviewPrompt []plugin.PendingReq

		for name, versionRange := range allDeps {
			wg.Add(1)
			go func(n, vr string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				status := pterm.FgGray.Sprint("UNKNOWN")
				authorID := "-"
				resolvedVer := vr
				isPlugin := false
				isPending := false

				// Check pending list first
				for _, p := range pendingList {
					if p["name"] == n {
						isPlugin = true
						isPending = true
						resolvedVer = p["version"]
						break
					}
				}

				utils.Log("Analysis", n)

				// Detect if it is already verified in config
				pinnedKey := ""
				if internal, ok := config["$internal"].(map[string]interface{}); ok {
					if pCfg, ok := internal[n].(map[string]interface{}); ok {
						if sigCfg, ok := pCfg["signature"].(map[string]interface{}); ok {
							pinnedKey, _ = sigCfg["author_key"].(string)
						}
					}
				}

				// 1. Check local node_modules
				pkgDir := paths.NodeModulesPkgDir(projectRoot, n)
				sigPath := paths.PackageSigPath(pkgDir)
				
				var xsigBytes []byte
				if _, err := os.Stat(sigPath); err == nil {
					isPlugin = true
					isPending = false // Found locally - clear pending flag
					if pj, err := core.LoadPackageJson(paths.PackageJsonPath(pkgDir)); err == nil {
						resolvedVer = pj.Version
					}
					xsigBytes, _ = os.ReadFile(sigPath)
				}
				
				// 1b. Check virtual store if not found in node_modules
				if !isPlugin {
					// Use resolvedVer from pending or try latest from package.json if it exists
					vStorePath := paths.PackageVStoreDir(projectRoot, n, resolvedVer)
					sigPath = paths.PackageSigPath(vStorePath)
					if _, err := os.Stat(sigPath); err == nil {
						isPlugin = true
						xsigBytes, _ = os.ReadFile(sigPath)
					}
				}
				
				// 2. Fallback to registry if not found locally and not localOnly
				// But ONLY if not already known to be a plugin from config
				if !isPlugin && pinnedKey == "" && !localOnly {
					regPkg, err := registry.FetchPackage(context.Background(), n, false)
					if err == nil {
						resolvedVer = regPkg.DistTags["latest"]
						utils.LiveLog(fmt.Sprintf("Fetching metadata for %s@%s...", n, resolvedVer))
						meta, err := registry.FetchVersionMetadata(context.Background(), n, resolvedVer)
						if err == nil {
							inFiles := false
							for _, f := range meta.Files {
								if f == "xypriss.plugin.xsig" {
									inFiles = true
									break
								}
							}
							
							if inFiles {
								isPlugin = true
								utils.LiveLog(fmt.Sprintf("Signature confirmed in metadata for %s", n))
								xsigBytes, _ = registry.FetchFileFromTarball(context.Background(), meta.Dist.Tarball, "xypriss.plugin.xsig")
							} else {
								// Metadata Files missing or doesn't mention sig - only download if we have strong reason (e.g. keywords)
								isSuspect := false
								if meta.Description != "" && strings.Contains(strings.ToLower(meta.Description), "plugin") { isSuspect = true }
								
								if isSuspect {
									utils.LiveLog(fmt.Sprintf("Searching for signature in %s tarball (description suspect)...", n))
									xsigBytes, err = registry.FetchFileFromTarball(context.Background(), meta.Dist.Tarball, "xypriss.plugin.xsig")
									if err == nil && len(xsigBytes) > 0 {
										isPlugin = true
									}
								}
							}
						}
					}
				} else if pinnedKey != "" {
					isPlugin = true // Definitely a plugin since it's in config
				}

				if !isPlugin { return }

				if len(xsigBytes) > 0 {
					authorID = plugin.ExtractIdentity(xsigBytes)
					utils.LiveLog(fmt.Sprintf("Identity verified: %s", authorID))
				}

				// Determine Status
				status = pterm.FgRed.Sprint("UNTRUSTED")
				
				isMissingLocally := false
				if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
					isMissingLocally = true
				}

				// Check if plugin has ANY config entry (permissions or signature)
				hasConfigEntry := false
				if internal, ok := config["$internal"].(map[string]interface{}); ok {
					_, hasConfigEntry = internal[n]
				}

				if pinnedKey != "" {
					if authorID == "-" {
						if isMissingLocally {
							status = pterm.FgMagenta.Sprint("NOT_INSTALLED")
						} else {
							status = pterm.FgYellow.Sprint("KEY_MISMATCH")
						}
					} else if pinnedKey != authorID {
						status = pterm.FgYellow.Sprint("KEY_MISMATCH")
					} else {
						status = pterm.FgGreen.Sprint("VERIFIED")
					}
				} else if isPending {
					status = pterm.FgBlue.Sprint("PENDING")
				} else if isMissingLocally && !localOnly {
					status = pterm.FgMagenta.Sprint("NOT_INSTALLED")
				} else if hasConfigEntry {
					// Plugin has a config entry with permissions but no signature yet -> UNTRUSTED (needs trust)
					status = pterm.FgRed.Sprint("UNTRUSTED")
				}

				mu.Lock()
				items = append(items, []string{n, resolvedVer, status, authorID})
				if reviewMode {
					privs := ""
					if len(xsigBytes) > 0 {
						privs = plugin.ExtractPrivileges(xsigBytes)
					}
					
					webStatus := "pending"
					if pinnedKey != "" && pinnedKey == authorID {
						webStatus = "authorized"
					} else if isMissingLocally {
						webStatus = "NOT_INSTALLED"
					} else if isPending {
						webStatus = "pending"
					}

					reviewPrompt = append(reviewPrompt, plugin.PendingReq{
						Name:       n,
						Version:    resolvedVer,
						Identity:   authorID,
						Privileges: privs,
						Status:     webStatus,
					})
				}
				mu.Unlock()
			}(name, versionRange)
		}
		wg.Wait()
		s.Stop()

		if len(items) <= 1 {
			utils.Info("No plugins found in dependencies.")
			return nil
		}

		if reviewMode && len(reviewPrompt) > 0 {
			utils.Info("Opening browser for permission review...")
			plugin.HandleHtmlVerify(projectRoot, reviewPrompt, config, configPath, true)
		} else {
			table, _ := pterm.DefaultTable.WithHasHeader().WithData(items).Srender()
			pterm.DefaultBox.WithTitle(utils.AccentColor.Sprint("PROJECT PLUGINS")).Println(table)
		}

		return nil
	},
}

var pluginTrustCmd = &cobra.Command{
	Use:   "trust <package> <author_key>",
	Short: "Manually trust a plugin author",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, _ := os.Getwd()
		pkgName := args[0]
		authorKey := args[1]

		err := plugin.TrustPlugin(projectRoot, pkgName, authorKey)
		if err != nil {
			return err
		}

		return nil
	},
}

var pluginRevokeCmd = &cobra.Command{
	Use:   "revoke <package>",
	Short: "Revoke trust for a plugin author",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, _ := os.Getwd()
		pkgName := args[0]
		noPending, _ := cmd.Flags().GetBool("no-pending")

		err := plugin.RevokeTrust(projectRoot, pkgName, noPending)
		if err != nil {
			return err
		}

		return nil
	},
}

var pluginIDCmd = &cobra.Command{
	Use:     "id <package...>",
	Aliases: []string{"get-id", "get", "info"},
	Short:   "Retrieve detailed plugin information and identity",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		online, _ := cmd.Flags().GetBool("online")
		// If command was called as 'get' or 'info', force online mode
		if cmd.CalledAs() == "get" || cmd.CalledAs() == "info" {
			online = true
		}
		projectRoot, _ := os.Getwd()

		registry := core.NewRegistryClient("https://registry.npmjs.org", 3)

		utils.Matrix("Fetching plugin identity information...")

		for _, pkgName := range args {
			var identity string
			var version string
			var description string
			var source string

			if online {
				source = "Registry"
				regPkg, err := registry.FetchPackage(context.Background(), pkgName, false)
				if err != nil {
					utils.Error("Failed to fetch %s from registry: %v", pkgName, err)
					continue
				}
				version = regPkg.DistTags["latest"]
				meta, err := registry.FetchVersionMetadata(context.Background(), pkgName, version)
				if err != nil {
					utils.Error("Failed to fetch metadata for %s@%s: %v", pkgName, version, err)
					continue
				}
				description = meta.Description

				xsigBytes, err := registry.FetchFileFromTarball(context.Background(), meta.Dist.Tarball, "xypriss.plugin.xsig")
				if err != nil {
					utils.Error("%s is not a signed plugin: %v", pkgName, err)
					continue
				}

				identity = plugin.ExtractIdentity(xsigBytes)
			} else {
				source = "Local"
				// Try to find in node_modules
				pkgDir := paths.NodeModulesPkgDir(projectRoot, pkgName)
				sigPath := paths.PackageSigPath(pkgDir)

				if _, err := os.Stat(sigPath); os.IsNotExist(err) {
					// Try to find in vstore (might be a different version)
					// This is a bit more complex as we need to find the installed version
					pj, err := core.LoadPackageJson(paths.PackageJsonPath(pkgDir))
					if err == nil {
						version = pj.Version
						description = pj.Description
						sigPath = paths.PackageSigPath(paths.PackageVStoreDir(projectRoot, pkgName, version))
					}
				} else {
					pj, _ := core.LoadPackageJson(paths.PackageJsonPath(pkgDir))
					if pj != nil {
						version = pj.Version
						description = pj.Description
					}
				}

				xsigBytes, err := os.ReadFile(sigPath)
				if err != nil {
					utils.Error("Plugin %s is not installed locally or not signed. Use --online or 'xfpm plugin get' to fetch from registry.", pkgName)
					continue
				}
				identity = plugin.ExtractIdentity(xsigBytes)
			}

			if identity == "" {
				utils.Error("Could not extract Identity from %s signature.", pkgName)
				continue
			}

			pterm.DefaultSection.Printf("Plugin: %s", pkgName)
			if description != "" {
				pterm.Info.Printfln("  Detail:   %s", pterm.FgGray.Sprint(description))
			}
			pterm.Info.Printfln("  Version:  %s (%s)", pterm.FgCyan.Sprint(version), source)
			pterm.Info.Printfln("  Identity: %s", pterm.FgGreen.Sprint(identity))
			fmt.Println()
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(pluginCmd)
	pluginVerifyCmd.Flags().BoolVarP(&verifyNoInteract, "no-interact", "n", false, "Disable interactive prompts")
	pluginVerifyCmd.Flags().BoolVarP(&verifyHtml, "html", "w", false, "Open interactive verification dashboard in browser")
	pluginCmd.AddCommand(pluginVerifyCmd)

	pluginListCmd.Flags().BoolVarP(&listLocal, "local", "l", false, "Only check locally installed packages")
	pluginListCmd.Flags().BoolVarP(&listReview, "review", "r", false, "Review and update plugin permissions via web dashboard")
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginTrustCmd)
	pluginRevokeCmd.Flags().BoolVar(&revokeNoPending, "no-pending", false, "Do not add the plugin back to pending list after revocation")
	pluginCmd.AddCommand(pluginRevokeCmd)
	pluginCmd.AddCommand(pluginIDCmd)
}
