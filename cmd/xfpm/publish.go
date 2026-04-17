package main

import (
	"os"
	"os/exec"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var publishCmd = &cobra.Command{
	Use:                "publish",
	Short:              "Publish a package to the registry",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintBanner()

		// Handle help flag manually since DisableFlagParsing is true
		for _, arg := range args {
			if arg == "--help" || arg == "-h" {
				cmd.Help()
				return nil
			}
		}

		utils.Matrix("Preparing to publish package...")

		npmArgs := append([]string{"publish"}, args...)
		c := exec.Command("npm", npmArgs...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin

		err := c.Run()
		if err != nil {
			utils.Error("\nFailed to publish: %v", err)
			return err
		}

		utils.Success("Package published successfully!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(publishCmd)
}
