package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var noInteract bool

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
		pendingPath := filepath.Join(projectRoot, "node_modules", ".xpm", "pending_plugins.json")

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

		cas, _ := core.NewCas(filepath.Join(os.Getenv("HOME"), ".xpm", "storage"))
		registry := core.NewRegistryClient("https://registry.npmjs.org", 8)
		installer := core.NewInstaller(cas, registry, projectRoot)
		installer.NoInteract = noInteract

		for _, p := range pending {
			// pterm.Print("\033[H\033[2J") // Clear
			pkgName := p["name"]
			pkgVer := p["version"]
			pkg := &core.ResolvedPackage{Name: pkgName, Version: pkgVer}
			pkgVStoreName := strings.ReplaceAll(pkgName, "/", "+") + "@" + pkgVer
			pkgDir := filepath.Join(projectRoot, "node_modules", ".xpm", "vstore", pkgVStoreName, "node_modules", pkgName)
			sigPath := filepath.Join(pkgDir, "xypriss.plugin.xsig")

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
				pkgVStoreName := strings.ReplaceAll(n, "/", "+") + "@" + resolvedVer
				pkgDir := filepath.Join(projectRoot, "node_modules", ".xpm", "vstore", pkgVStoreName, "node_modules", n)
				sigPath := filepath.Join(pkgDir, "xypriss.plugin.xsig")

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
					configPath := filepath.Join(projectRoot, "xypriss.config.jsonc")
					if _, err := os.Stat(configPath); os.IsNotExist(err) {
						configPath = filepath.Join(projectRoot, "xypriss.config.json")
					}
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
		configPath := filepath.Join(projectRoot, "xypriss.config.jsonc")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = filepath.Join(projectRoot, "xypriss.config.json")
		}

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

		configPath := filepath.Join(projectRoot, "xypriss.config.jsonc")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = filepath.Join(projectRoot, "xypriss.config.json")
		}

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
			utils.Success("Trust revoked for %s.", pkgName)
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

				identity = extractIdentityFromXsig(xsigBytes)
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
						pkgVStoreName := strings.ReplaceAll(pkgName, "/", "+") + "@" + version
						sigPath = filepath.Join(projectRoot, "node_modules", ".xpm", "vstore", pkgVStoreName, "node_modules", pkgName, "xypriss.plugin.xsig")
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
				identity = extractIdentityFromXsig(xsigBytes)
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

func extractIdentityFromXsig(data []byte) string {
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "Identity: ") {
			return strings.TrimPrefix(l, "Identity: ")
		}
	}
	return ""
}

func init() {
	pluginVerifyCmd.Flags().BoolP("no-interact", "n", false, "Disable interactive prompts and auto-verify if signature is valid")
	pluginIDCmd.Flags().BoolP("online", "o", false, "Fetch plugin identity from registry instead of local installation")
	pluginCmd.AddCommand(pluginVerifyCmd)
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginTrustCmd)
	pluginCmd.AddCommand(pluginRevokeCmd)
	pluginCmd.AddCommand(pluginIDCmd)
}
