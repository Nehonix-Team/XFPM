package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/core"

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
			pkgPath = filepath.Join(absPath, "package.json")
		}
		
		absPkgPath, err := filepath.Abs(pkgPath)
		if err != nil {
			return err
		}

		pkg, err := core.LoadPackageJson(absPkgPath)
		if err != nil {
			return fmt.Errorf("failed to read package.json at %s: %w", absPkgPath, err)
		}

		if len(pkg.Files) == 0 {
			return fmt.Errorf("mandatory 'files' field is missing or empty in package.json. XFPM refuses to sign unsafe wide-packages")
		}

		home, _ := os.UserHomeDir()
		privPath := filepath.Join(home, ".xfpm", "id_ed25519")
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

		utils.Matrix("Calculating content hash for: " + targetDir)
		
		// Collect matching files
		allFilesMap := make(map[string]bool)
		pkgDir := filepath.Dir(absPkgPath)

		for _, pattern := range pkg.Files {
			fullPattern := filepath.Join(pkgDir, pattern)
			
			// Resolve directory vs file vs glob
			matches, err := filepath.Glob(fullPattern)
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
						if info.Name() == "xypriss.xsig" {
							return nil
						}
						allFilesMap[path] = true
						return nil
					})
				} else {
					// Skip the signature file if matched by glob/file
					if info.Name() != "xypriss.xsig" {
						allFilesMap[m] = true
					}
				}
			}
		}

		// Convert to sorted slice for deterministic hashing
		var fileList []string
		for f := range allFilesMap {
			fileList = append(fileList, f)
		}
		
		// Byte-wise sort (natural Go string sort)
		sort.Strings(fileList)

		h := sha256.New()
		for _, path := range fileList {
			rel, _ := filepath.Rel(absPath, path)
			utils.StickyStatus(rel, "hashing...")

			f, err := os.Open(path)
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


		sigData := map[string]interface{}{
			"name":          pkg.Name,
			"version":       pkg.Version,
			"min_version":   minVersion,
			"content_hash":  contentHash,
			"prev_sig_hash": prevSigHash,
			"author_key":    authorID,
			"expires_at":    expiresAt,
		}

		payloadJSON, _ := json.Marshal(sigData)
		signatureBytes := ed25519.Sign(privKey, payloadJSON)
		sigData["signature"] = fmt.Sprintf("base64:%s", base64.StdEncoding.EncodeToString(signatureBytes))

		// Enforce "xypriss.xsig" in the files array
		sigInFiles := false
		for _, f := range pkg.Files {
			if f == "xypriss.xsig" {
				sigInFiles = true
				break
			}
		}

		if !sigInFiles {
			return fmt.Errorf("\"xypriss.xsig\" MUST be present in the \"files\" array of package.json")
		}

		// Save signature to the same directory as package.json (root of plugin)
		packageDir := filepath.Dir(absPkgPath)
		sigFilePath := filepath.Join(packageDir, "xypriss.xsig")
		
		finalJSON, _ := json.MarshalIndent(sigData, "", "    ")
		if err := os.WriteFile(sigFilePath, finalJSON, 0644); err != nil {
			return fmt.Errorf("failed to write signature: %w", err)
		}

		utils.Success("Plugin signed successfully! Developer ID: %s", authorID)
		utils.Info("Signature saved to root: %s", sigFilePath)
		return nil
	},
}

func init() {
	signCmd.Flags().String("min-version", "", "Minimum allowed version (anti-downgrade)")
	signCmd.Flags().StringP("package", "p", "", "Path to package.json (default: path/package.json)")
}

