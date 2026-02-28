package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Nehonix-Team/xfpm-go/internal/utils"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec [command]",
	Short: "Execute a command from local node_modules/.bin",
	Aliases: []string{"x"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no command specified")
		}

		binName := args[0]
		binArgs := args[1:]

		projectRoot, _ := os.Getwd()
		binPath := filepath.Join(projectRoot, "node_modules", ".bin", binName)

		if _, err := os.Stat(binPath); err == nil {
			utils.Info("Executing: %s", binName)
			return executeCommand(binPath, binArgs, projectRoot)
		}

		// Fallback: try to execute directly if it's in the system PATH
		if _, err := exec.LookPath(binName); err == nil {
			return executeCommand(binName, binArgs, projectRoot)
		}

		return fmt.Errorf("command '%s' not found", binName)
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
