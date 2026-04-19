package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var genKeyCmd = &cobra.Command{
	Use:   "gen-key",
	Short: "Generate an Ed25519 key pair for plugin signing",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		xfpmDir := filepath.Join(home, ".xfpm")
		if err := os.MkdirAll(xfpmDir, 0700); err != nil {
			return fmt.Errorf("failed to create %s: %w", xfpmDir, err)
		}

		privPath := filepath.Join(xfpmDir, "id_ed25519")
		pubPath := filepath.Join(xfpmDir, "id_ed25519.pub")

		if _, err := os.Stat(privPath); err == nil {
			options := []string{
				"Utiliser l'existant et régénérer le fichier .sig du projet",
				"Écraser l'identité actuelle, régénérer une nouvelle clé et signer",
				"Annuler",
			}
			selected, _ := pterm.DefaultInteractiveSelect.
				WithOptions(options).
				WithDefaultText("L'identité de développeur existe déjà. Que faire ?").
				Show()

			if selected == "Annuler" {
				return nil
			} else if selected == options[0] {
				return promptAndSign()
			} else if selected == options[1] {
				utils.Matrix("Suppression de l'ancienne clé...")
				os.Remove(privPath)
				os.Remove(pubPath)
			}
		}

		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate key: %w", err)
		}

		if err := os.WriteFile(privPath, priv, 0600); err != nil {
			return fmt.Errorf("failed to write private key: %w", err)
		}

		pubHex := fmt.Sprintf("ed25519:%s", hex.EncodeToString(pub))
		if err := os.WriteFile(pubPath, []byte(pubHex), 0644); err != nil {
			return fmt.Errorf("failed to write public key: %w", err)
		}

		utils.Success("Identity generated successfully!")
		utils.Info("Private Key: %s (KEEP SECRET)", privPath)
		utils.Info("Public Key: %s", pubPath)
		utils.Info("Developer ID: %s", pubHex)

		return promptAndSign()
	},
}

func promptAndSign() error {
	minVersion, _ := pterm.DefaultInteractiveTextInput.Show("Entrez la version minimale requise pour ce plugin (ex: 0.1.0)")
	if minVersion == "" {
		utils.Error("Opération annulée, min-version requis pour signer.")
		return nil
	}
	return signProject(".", minVersion)
}
