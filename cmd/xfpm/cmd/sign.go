package cmd

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var signCmd = &cobra.Command{
	Use:   "sign [path]",
	Short: "Sign a XyPriss plugin for Zero-Trust distribution",
	RunE: func(cmd *cobra.Command, args []string) error {
		minVersion, _ := cmd.Flags().GetString("min-version")
		if minVersion == "" {
			return fmt.Errorf("--min-version is required")
		}

		targetDir := "."
		if len(args) > 0 {
			targetDir = args[0]
		}

		absPath, err := filepath.Abs(targetDir)
		if err != nil {
			return err
		}

		pkgPath, _ := cmd.Flags().GetString("package")
		if pkgPath == "" {
			pkgPath = paths.PackageJsonPath(absPath)
		}
		
		absPkgPath, err := filepath.Abs(pkgPath)
		if err != nil {
			return err
		}

		pkg, err := core.LoadPackageJson(absPkgPath)
		if err != nil {
			return fmt.Errorf("failed to read package.json at %s: %w", absPkgPath, err)
		}

		packageDir := filepath.Dir(absPkgPath)

		privPath := paths.IdentityPath()
		privBytes, err := os.ReadFile(privPath)
		if err != nil {
			return fmt.Errorf("identity not found. Run 'xfpm gen-key' first")
		}

		if len(privBytes) != ed25519.PrivateKeySize {
			return fmt.Errorf("invalid private key file")
		}
		privKey := ed25519.PrivateKey(privBytes)

		var pubKey ed25519.PublicKey = make([]byte, ed25519.PublicKeySize)
		copy(pubKey, privKey[32:])
		authorID := fmt.Sprintf("ed25519:%s", hex.EncodeToString(pubKey))

		utils.Matrix("Fetching remote XyPriss permissions manifest...")
		resp, err := http.Get("https://raw.githubusercontent.com/Nehonix-Team/XyPriss/master/.data/base/permissions.json")
		if err != nil {
			return fmt.Errorf("failed to fetch authoritative permissions file: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("authoritative permissions file fetch failed with status: %d", resp.StatusCode)
		}

		var masterPermissions map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&masterPermissions); err != nil {
			return fmt.Errorf("failed to parse authoritative permissions: %w", err)
		}

		fixFlag, _ := cmd.Flags().GetBool("fix")

		requestedPermsStr := ""
		if pkg.Xfpm == nil || pkg.Xfpm.Permissions == nil {
			return fmt.Errorf("the 'xfpm.permissions' field is mandatory in package.json for signing. If your plugin requires no permissions, use an empty array: \"permissions\": []")
		}

		if len(pkg.Xfpm.Permissions) > 0 {
			utils.Matrix("Validating requested permissions...")
			seen := make(map[string]bool)
			var uniquePerms []string
			hasDuplicates := false

			for _, perm := range pkg.Xfpm.Permissions {
				if _, exists := masterPermissions[perm]; !exists {
					return fmt.Errorf("permission '%s' is strictly invalid or does not exist in the XyPriss system", perm)
				}
				if seen[perm] {
					hasDuplicates = true
				} else {
					seen[perm] = true
					uniquePerms = append(uniquePerms, perm)
				}
			}

			if hasDuplicates {
				if fixFlag {
					utils.Warn("Duplicate permissions detected. Auto-correcting package.json because --fix is enabled.")
					pkg.Xfpm.Permissions = uniquePerms
					pkgData, _ := json.MarshalIndent(pkg, "", "  ")
					os.WriteFile(absPkgPath, pkgData, 0644)
				} else {
					return fmt.Errorf("duplicate permissions detected in package.json. Remove duplicates manually or run with --fix to auto-correct")
				}
			}

			requestedPermsStr = strings.Join(uniquePerms, ",")
		} else {
			requestedPermsStr = "none"
		}

		// --- Mandatory Plugin Config Enforcement ---
		configPath := paths.ConfigPath(packageDir)
		configBase := filepath.Base(configPath)

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			if fixFlag {
				utils.Success("Creating missing 'xypriss.config.jsonc'...")
				content := "{\n    \"$internal\": {\n        \"$(pkg).name\": {\n            \"type\": \"plugin\"\n        }\n    }\n}\n"
				os.WriteFile(configPath, []byte(content), 0644)

				// Also ensure it's in pkg.Files
				found := false
				for _, f := range pkg.Files {
					if f == configBase || (configBase == "xypriss.config.jsonc" && f == "xypriss.config.json") {
						found = true
						break
					}
				}
				if !found {
					pkg.Files = append(pkg.Files, configBase)
					pkgData, _ := json.MarshalIndent(pkg, "", "  ")
					os.WriteFile(absPkgPath, pkgData, 0644)
				}
			}
		} else {
			// File exists, check content
			data, _ := os.ReadFile(configPath)
			content := string(data)
			if !strings.Contains(content, "\"type\": \"plugin\"") || !strings.Contains(content, "$(pkg).name") {
				if fixFlag {
					utils.Warn("Incomplete '%s' detected. Auto-fixing...", configBase)
					var cfg map[string]interface{}
					
					// Simple JSONC stripping for parsing
					lines := strings.Split(content, "\n")
					var cleanLines []string
					for _, l := range lines {
						trimmed := strings.TrimSpace(l)
						if !strings.HasPrefix(trimmed, "//") && trimmed != "" {
							if idx := strings.Index(l, "//"); idx != -1 {
								cleanLines = append(cleanLines, l[:idx])
							} else {
								cleanLines = append(cleanLines, l)
							}
						}
					}
					json.Unmarshal([]byte(strings.Join(cleanLines, "\n")), &cfg)
					
					if cfg == nil {
						cfg = make(map[string]interface{})
					}

					internal, ok := cfg["$internal"].(map[string]interface{})
					if !ok {
						internal = make(map[string]interface{})
						cfg["$internal"] = internal
					}
					pkgNode, ok := internal["$(pkg).name"].(map[string]interface{})
					if !ok {
						pkgNode = make(map[string]interface{})
						internal["$(pkg).name"] = pkgNode
					}
					pkgNode["type"] = "plugin"

					newData, _ := json.MarshalIndent(cfg, "", "  ")
					os.WriteFile(configPath, newData, 0644)
				} else {
					return fmt.Errorf("mandatory '$internal' plugin metadata is missing in '%s'. Run with --fix to correct it", configBase)
				}
			}
		}

		utils.Matrix("Calculating content hash for: " + targetDir)
		
		// Collect matching files
		allFilesMap := make(map[string]bool)
		pkgDir := filepath.Dir(absPkgPath)

		// Strict validation: check if all files listed in package.json exist
		skipValidation := false
		for _, arg := range args {
			if arg == "--" {
				skipValidation = true
				break
			}
		}

		for _, pattern := range pkg.Files {
			// Skip validation for the signature file itself
			if pattern == paths.SigFileName {
				continue
			}

			fullPattern := filepath.Join(pkgDir, pattern)
			
			// Resolve directory vs file vs glob
			matches, err := filepath.Glob(fullPattern)
			
			if !skipValidation {
				if err != nil || len(matches) == 0 {
					return fmt.Errorf("file or directory '%s' listed in package.json does not exist. Use '--' at the end of command to ignore this validation", pattern)
				}
			}

			if err != nil {
				// If glob fails, try direct path
				matches = []string{fullPattern}
			}

			for _, m := range matches {
				info, err := os.Stat(m)
				if err != nil {
					continue
				}

				if info.IsDir() {
					filepath.Walk(m, func(path string, info os.FileInfo, err error) error {
						if err != nil { return nil }
						if info.IsDir() {
							name := info.Name()
							if name == "node_modules" || name == ".git" || name == ".idea" {
								return filepath.SkipDir
							}
							return nil
						}
						// Skip the signature file itself
						if info.Name() == paths.SigFileName {
							return nil
						}
						allFilesMap[path] = true
						return nil
					})
				} else {
					// Skip the signature file if matched by glob/file
					if info.Name() != paths.SigFileName {
						allFilesMap[m] = true
					}
				}
			}
		}

		// Convert to sorted slice for deterministic hashing
		type fileRel struct {
			abs string
			rel string
		}
		var fileList []fileRel
		for f := range allFilesMap {
			rel, _ := filepath.Rel(packageDir, f)
			fileList = append(fileList, fileRel{abs: f, rel: rel})
		}
		
		// Sort by relative path (essential for cross-machine hashing)
		sort.Slice(fileList, func(i, j int) bool {
			return fileList[i].rel < fileList[j].rel
		})

		h := sha256.New()
		for _, fEntry := range fileList {
			utils.StickyStatus(fEntry.rel, "hashing...")

			f, err := os.Open(fEntry.abs)
			if err != nil {
				return err
			}
			if _, err := io.Copy(h, f); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}

		contentHash := fmt.Sprintf("sha256:%s", hex.EncodeToString(h.Sum(nil)))
		prevSigHash := "sha256:none"
		expiresAt := time.Now().AddDate(1, 0, 0).Format(time.RFC3339)


		// Generate custom branded signature format
		sigContent := fmt.Sprintf(
			"--- XYPRISS SIGNATURE (G3) ---\n"+
				"Manifest: %s@%s\n"+
				"Min-Engine: %s\n"+
				"Fingerprint: %s\n"+
				"Identity: %s\n"+
				"Privileges: %s\n"+
				"Expires: %s\n"+
				"Revision: %s\n",
			pkg.Name, pkg.Version, minVersion, contentHash, authorID, requestedPermsStr, expiresAt, prevSigHash,
		)

		signatureBytes := ed25519.Sign(privKey, []byte(sigContent))
		signatureBase64 := base64.StdEncoding.EncodeToString(signatureBytes)

		finalSig := fmt.Sprintf(
			"%s--- BEGIN CRYPTOGRAPHIC PROOF ---\nbase64:%s\n--- END XYPRISS SIGNATURE ---\n",
			sigContent, signatureBase64,
		)

		// Enforce signature file in the files array
		sigInFiles := false
		for _, f := range pkg.Files {
			if f == paths.SigFileName {
				sigInFiles = true
				break
			}
		}

		if !sigInFiles {
			return fmt.Errorf("\"%s\" MUST be present in the \"files\" array of package.json", paths.SigFileName)
		}

		// Save signature to the same directory as package.json (root of plugin)
		sigFilePath := paths.PackageSigPath(packageDir)

		if err := os.WriteFile(sigFilePath, []byte(finalSig), 0644); err != nil {
			return fmt.Errorf("failed to write signature: %w", err)
		}

		utils.Success("Plugin signed successfully! Developer ID: %s", authorID)
		utils.Info("Signature saved to root: %s", sigFilePath)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(signCmd)
	signCmd.Flags().StringP("min-version", "m", "", "Minimum allowed version (anti-downgrade)")
	signCmd.Flags().StringP("package", "p", "", "Path to package.json (default: path/package.json)")
	signCmd.Flags().Bool("fix", false, "Auto-correct errors in package.json like duplicate permissions")
}
