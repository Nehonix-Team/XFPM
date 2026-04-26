package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
			if pkgJson, err := core.LoadPackageJson(filepath.Join(projectRoot, "package.json")); err == nil && pkgJson != nil {
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
					requiresPrompt = append(requiresPrompt, plugin.PendingReq{
						Name:       pkgName,
						Identity:   identity,
						Privileges: privs,
					})
				}
			}

			if len(requiresPrompt) > 0 {
				if verifyHtml {
					plugin.HandleHtmlVerify(projectRoot, requiresPrompt, config, configPath)
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
			rootDest := filepath.Join(projectRoot, "node_modules", pkgName)
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
		projectRoot, _ := os.Getwd()
		pkgJsonPath := filepath.Join(projectRoot, "package.json")
		pkg, err := core.LoadPackageJson(pkgJsonPath)
		if err != nil {
			return fmt.Errorf("failed to load package.json: %w", err)
		}

		utils.Matrix("Analyzing project dependencies for plugins...")
		
		s, _ := pterm.DefaultSpinner.
			WithRemoveWhenDone(true).
			WithText("Initializing analysis...").
			Start()

		allDeps := pkg.AllDependencies()
		registry := core.NewRegistryClient("https://registry.npmjs.org", 8)
		registry.SetCacheDir("")
		
		var items [][]string
		items = append(items, []string{"Plugin", "Version", "Status", "Developer ID"})

		// Use a wait group to fetch metadata in parallel for speed
		var wg sync.WaitGroup
		var mu sync.Mutex
		
		count := 0
		total := len(allDeps)
		sem := make(chan struct{}, 10)
		for name, versionRange := range allDeps {
			wg.Add(1)
			go func(n, vr string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				mu.Lock()
				count++
				s.UpdateText(fmt.Sprintf(" [%d/%d] Fetching metadata for %s...", count, total, n))
				mu.Unlock()

				// Resolve version first
				regPkg, err := registry.FetchPackage(context.Background(), n, false)
				if err != nil { return }
				
				// Standard resolution (simplification: take the version if it matches exactly or use dist-tag)
				resolvedVer := vr
				if _, ok := regPkg.Versions[vr]; !ok {
					resolvedVer = regPkg.DistTags["latest"]
				}
				
				meta, err := registry.FetchVersionMetadata(context.Background(), n, resolvedVer)
				if err != nil { return }

				isPlugin := false
				// 1. Check Files array in registry metadata
				for _, f := range meta.Files {
					if f == "xypriss.plugin.xsig" {
						isPlugin = true
						break
					}
				}
				
				// 2. fallback: check if already in vstore
				pkgDir := paths.PackageVStoreDir(projectRoot, n, resolvedVer)
				sigPath := paths.PackageSigPath(pkgDir)

				if !isPlugin {
					if _, err := os.Stat(sigPath); err == nil {
						isPlugin = true
					}
				}

				if isPlugin {
					authorID := "-"
					pinnedKey := ""
					// Extract author ID from xsig if present
					if b, err := os.ReadFile(sigPath); err == nil {
						lines := strings.Split(string(b), "\n")
						for _, l := range lines {
							if strings.HasPrefix(l, "Identity: ") {
								authorID = strings.TrimPrefix(l, "Identity: ")
								break
							}
						}
					}

					// Get pinned key from config
					configPath := paths.ConfigPath(projectRoot)
					if cfgBytes, err := os.ReadFile(configPath); err == nil {
						lines := strings.Split(string(cfgBytes), "\n")
						var cleanLines []string
						for _, line := range lines {
							if idx := strings.Index(line, "//"); idx != -1 {
								line = line[:idx]
							}
							cleanLines = append(cleanLines, line)
						}
						var config map[string]interface{}
						if err := json.Unmarshal([]byte(strings.Join(cleanLines, "\n")), &config); err == nil {
							if internal, ok := config["$internal"].(map[string]interface{}); ok {
								if pluginCfg, ok := internal[n].(map[string]interface{}); ok {
									if sigCfg, ok := pluginCfg["signature"].(map[string]interface{}); ok {
										pinnedKey, _ = sigCfg["author_key"].(string)
									}
								}
							}
						}
					}

					trusted := pinnedKey != "" && pinnedKey == authorID
					trustStr := pterm.FgRed.Sprint("UNTRUSTED")
					if trusted { trustStr = pterm.FgGreen.Sprint("VERIFIED") }
					if pinnedKey != "" && pinnedKey != authorID {
						trustStr = pterm.FgYellow.Sprint("KEY_MISMATCH")
					}

					mu.Lock()
					items = append(items, []string{n, resolvedVer, trustStr, authorID})
					mu.Unlock()
				}
			}(name, versionRange)
		}
		wg.Wait()
		s.Stop()

		if len(items) <= 1 {
			utils.Info("No plugins found in direct dependencies.")
			return nil
		}

		table, _ := pterm.DefaultTable.WithHasHeader().WithData(items).Srender()
		pterm.DefaultBox.WithTitle(utils.AccentColor.Sprint("ACTIVE PLUGINS FOR THIS PROJECT")).Println(table)
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

		// We reuse the logic but we need a specific helper or just do it here
		configPath := paths.ConfigPath(projectRoot)

		var config map[string]interface{}
		if cfgBytes, err := os.ReadFile(configPath); err == nil {
			lines := strings.Split(string(cfgBytes), "\n")
			var cleanLines []string
			for _, line := range lines {
				if idx := strings.Index(line, "//"); idx != -1 {
					line = line[:idx]
				}
				cleanLines = append(cleanLines, line)
			}
			json.Unmarshal([]byte(strings.Join(cleanLines, "\n")), &config)
		}
		if config == nil { config = make(map[string]interface{}) }

		internalRaw, ok := config["$internal"]
		if !ok { internalRaw = make(map[string]interface{}) }
		internal, ok := internalRaw.(map[string]interface{})
		if !ok { internal = make(map[string]interface{}) }

		pluginCfgRaw, ok := internal[pkgName]
		if !ok { pluginCfgRaw = make(map[string]interface{}) }
		pluginCfg, ok := pluginCfgRaw.(map[string]interface{})
		if !ok { pluginCfg = make(map[string]interface{}) }

		sigCfgRaw, ok := pluginCfg["signature"]
		if !ok { sigCfgRaw = make(map[string]interface{}) }
		sigCfg, ok := sigCfgRaw.(map[string]interface{})
		if !ok { sigCfg = make(map[string]interface{}) }

		sigCfg["author_key"] = authorKey
		pluginCfg["signature"] = sigCfg
		internal[pkgName] = pluginCfg
		config["$internal"] = internal

		if out, err := json.MarshalIndent(config, "", "    "); err == nil {
			os.WriteFile(configPath, out, 0644)
			utils.Success("Manual trust pinned for %s.", pkgName)
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
		configPath := paths.ConfigPath(projectRoot)

		b, err := os.ReadFile(configPath)
		if err != nil { return err }

		// Pre-revocation check: Ensure it's an actual plugin in node_modules
		pkgDir := filepath.Join(projectRoot, "node_modules", pkgName)
		if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
			return fmt.Errorf("package %s is not installed. Revocation requires an active installation for verification", pkgName)
		}
		
		sigPath := filepath.Join(pkgDir, "xypriss.plugin.xsig")
		if _, err := os.Stat(sigPath); os.IsNotExist(err) {
			return fmt.Errorf("package %s is not a verified plugin (missing .xsig).", pkgName)
		}

		pj, err := core.LoadPackageJson(filepath.Join(pkgDir, "package.json"))
		if err != nil {
			return fmt.Errorf("failed to load package.json for %s: %w", pkgName, err)
		}
		pkgVer := pj.Version

		var config map[string]interface{}
		json.Unmarshal(b, &config)

		internal, ok := config["$internal"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("no trusted plugins found in project configuration")
		}

		if _, exists := internal[pkgName]; !exists {
			return fmt.Errorf("plugin %s is not currently trusted in this project", pkgName)
		}

		delete(internal, pkgName)
		config["$internal"] = internal
		if out, err := json.MarshalIndent(config, "", "    "); err == nil {
			os.WriteFile(configPath, out, 0644)
			utils.Success("Trust revoked and permissions retired for %s.", pkgName)
		}

		if !noPending {
			pendingPath := paths.PendingPluginsPath(projectRoot)
			var pending []map[string]string
			if data, err := os.ReadFile(pendingPath); err == nil {
				json.Unmarshal(data, &pending)
			}
			
			// Check if already pending
			found := false
			for _, p := range pending {
				if p["name"] == pkgName && p["version"] == pkgVer {
					found = true
					break
				}
			}
			
			if !found {
				pending = append(pending, map[string]string{
					"name":    pkgName,
					"version": pkgVer,
				})
				if out, err := json.MarshalIndent(pending, "", "  "); err == nil {
					os.MkdirAll(filepath.Dir(pendingPath), 0755)
					os.WriteFile(pendingPath, out, 0644)
					utils.Info("Plugin %s@%s moved back to pending list.", pkgName, pkgVer)
				}
			}
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
				pkgDir := filepath.Join(projectRoot, "node_modules", pkgName)
				sigPath := filepath.Join(pkgDir, "xypriss.plugin.xsig")

				if _, err := os.Stat(sigPath); os.IsNotExist(err) {
					// Try to find in vstore (might be a different version)
					// This is a bit more complex as we need to find the installed version
					pkgJsonPath := filepath.Join(pkgDir, "package.json")
					pj, err := core.LoadPackageJson(pkgJsonPath)
					if err == nil {
						version = pj.Version
						description = pj.Description
						sigPath = paths.PackageSigPath(paths.PackageVStoreDir(projectRoot, pkgName, version))
					}
				} else {
					pj, _ := core.LoadPackageJson(filepath.Join(pkgDir, "package.json"))
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
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginTrustCmd)
	pluginRevokeCmd.Flags().BoolVar(&revokeNoPending, "no-pending", false, "Do not add the plugin back to pending list after revocation")
	pluginCmd.AddCommand(pluginRevokeCmd)
	pluginCmd.AddCommand(pluginIDCmd)
}
