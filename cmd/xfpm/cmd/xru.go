package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var xruCmd = &cobra.Command{
	Use:   "xru <rule_file> [target_dir]",
	Short: "Apply XyPriss Rule Unit (XRU) patches to files",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintBanner()

		rulePath := args[0]
		targetDir := "."
		if len(args) > 1 {
			targetDir = args[1]
		}

		xruPath, err := exec.LookPath("xru")
		if err != nil {
			xruPath = filepath.Join(filepath.Dir(os.Args[0]), "xru")
			if _, err := os.Stat(xruPath); err != nil {
				utils.Error("XRU binary not found in PATH or project root")
				return fmt.Errorf("xru not found")
			}
		}

		utils.Premium("XRU Engine", fmt.Sprintf("Executing %s on %s", rulePath, targetDir))

		xcmd := exec.Command(xruPath, rulePath, targetDir)
		xcmd.Stdout = os.Stdout
		xcmd.Stderr = os.Stderr

		if err := xcmd.Run(); err != nil {
			utils.Error("XRU execution failed: %v", err)
			return err
		}

		utils.Success("XRU transformation complete.")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(xruCmd)
}
