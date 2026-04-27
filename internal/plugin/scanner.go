package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
)

// ScanProjectPlugins performs a full scan of project dependencies to find plugins
func ScanProjectPlugins(projectRoot string, config map[string]interface{}) ([]PendingReq, [][]string) {
	utils.Matrix("Analyzing project dependencies for plugins...")
	reg := core.NewRegistryClient("", 3)

	pkg, err := core.LoadPackageJson(filepath.Join(projectRoot, "package.json"))
	if err != nil {
		utils.Error("Could not load package.json: %v", err)
		return nil, nil
	}

	allDeps := make(map[string]string)
	for k, v := range pkg.Dependencies {
		allDeps[k] = v
	}
	for k, v := range pkg.DevDependencies {
		allDeps[k] = v
	}

	pendingPath := paths.PendingPluginsPath(projectRoot)
	var pendingList []map[string]string
	if data, err := os.ReadFile(pendingPath); err == nil {
		json.Unmarshal(data, &pendingList)
	}

	var items [][]string
	items = append(items, []string{"Plugin", "Version", "Status", "Developer ID"})

	var reviewPrompt []PendingReq
	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, versionRange := range allDeps {
		wg.Add(1)
		go func(n, vr string) {
			defer wg.Done()

			utils.LiveLog(fmt.Sprintf("Analysis %s", n))

			pinnedKey := ""
			if internal, ok := config["$internal"].(map[string]interface{}); ok {
				if pluginCfg, ok := internal[n].(map[string]interface{}); ok {
					if sig, ok := pluginCfg["signature"].(map[string]interface{}); ok {
						pinnedKey, _ = sig["author_key"].(string)
					}
				}
			}

			resolvedVer := ""
			isPending := false
			for _, p := range pendingList {
				if p["name"] == n {
					resolvedVer = p["version"]
					isPending = true
					break
				}
			}

			pkgDir := paths.NodeModulesPkgDir(projectRoot, n)
			sigPath := paths.PackageSigPath(pkgDir)
			isPlugin := false
			var xsigBytes []byte

			if _, err := os.Stat(sigPath); err == nil {
				isPlugin = true
				xsigBytes, _ = os.ReadFile(sigPath)
				if resolvedVer == "" {
					if pj, err := core.LoadPackageJson(filepath.Join(pkgDir, "package.json")); err == nil {
						resolvedVer = pj.Version
					}
				}
			}

			if !isPlugin {
				vStorePath := paths.PackageVStoreDir(projectRoot, n, resolvedVer)
				sigPath = paths.PackageSigPath(vStorePath)
				if _, err := os.Stat(sigPath); err == nil {
					isPlugin = true
					xsigBytes, _ = os.ReadFile(sigPath)
				}
			}

			if !isPlugin && pinnedKey == "" {
				regPkg, err := reg.FetchPackage(context.Background(), n, false)
				if err == nil {
					resolvedVer = regPkg.DistTags["latest"]
					utils.LiveLog(fmt.Sprintf("Fetching metadata for %s@%s...", n, resolvedVer))
					meta, err := reg.FetchVersionMetadata(context.Background(), n, resolvedVer)
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
							xsigBytes, _ = reg.FetchFileFromTarball(context.Background(), meta.Dist.Tarball, "xypriss.plugin.xsig")
						} else {
							isSuspect := false
							if meta.Description != "" && strings.Contains(strings.ToLower(meta.Description), "plugin") {
								isSuspect = true
							}
							if isSuspect {
								xsigBytes, err = reg.FetchFileFromTarball(context.Background(), meta.Dist.Tarball, "xypriss.plugin.xsig")
								if err == nil && len(xsigBytes) > 0 {
									isPlugin = true
								}
							}
						}
					}
				}
			} else if pinnedKey != "" {
				isPlugin = true
			}

			if !isPlugin {
				return
			}

			authorID := "-"
			if len(xsigBytes) > 0 {
				authorID = ExtractIdentity(xsigBytes)
			}

			status := pterm.FgRed.Sprint("UNTRUSTED")
			isMissingLocally := false
			if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
				isMissingLocally = true
			}

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
			} else if isMissingLocally {
				status = pterm.FgMagenta.Sprint("NOT_INSTALLED")
			} else if hasConfigEntry {
				status = pterm.FgRed.Sprint("UNTRUSTED")
			}

			mu.Lock()
			items = append(items, []string{n, resolvedVer, status, authorID})
			privs := ""
			if len(xsigBytes) > 0 {
				privs = ExtractPrivileges(xsigBytes)
			}

			webStatus := "pending"
			if pinnedKey != "" && pinnedKey == authorID {
				webStatus = "authorized"
			} else if isMissingLocally {
				webStatus = "NOT_INSTALLED"
			}

			reviewPrompt = append(reviewPrompt, PendingReq{
				Name:       n,
				Version:    resolvedVer,
				Identity:   authorID,
				Privileges: privs,
				Status:     webStatus,
			})
			mu.Unlock()
		}(name, versionRange)
	}
	wg.Wait()

	return reviewPrompt, items
}
