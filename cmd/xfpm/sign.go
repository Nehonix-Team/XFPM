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

		return signProject(targetDir, minVersion)
	},
}

func signProject(targetDir string, minVersion string) error {
	if minVersion == "" {
		return fmt.Errorf("--min-version is required")
	}

	absPath, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}

	pkgPath := filepath.Join(absPath, "package.json")
	pkg, err := core.LoadPackageJson(pkgPath)
	if err != nil {
		return fmt.Errorf("failed to read package.json: %w", err)
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

	utils.Matrix("Calculating content hash...")
	h := sha256.New()
	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == ".idea" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() == "xypriss.plugin.sig" {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to hash content: %w", err)
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

	sigFilePath := filepath.Join(absPath, "xypriss.plugin.sig")
	finalJSON, _ := json.MarshalIndent(sigData, "", "    ")
	if err := os.WriteFile(sigFilePath, finalJSON, 0644); err != nil {
		return fmt.Errorf("failed to write signature: %w", err)
	}

	utils.Success("Plugin signed successfully! Developer ID: %s", authorID)
	return nil
}

func init() {
	signCmd.Flags().String("min-version", "", "Minimum allowed version (anti-downgrade)")
}
