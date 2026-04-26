/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 *
 * @author Nehonix
 * @license Nehonix OSL (NOSL)
 *
 * Copyright (c) 2025 Nehonix. All rights reserved.
 *
 * This License governs the use, modification, and distribution of software
 * provided by NEHONIX under its open source projects.
 * NEHONIX is committed to fostering collaborative innovation while strictly
 * protecting its intellectual property rights.
 * Violation of any term of this License will result in immediate termination of all granted rights
 * and may subject the violator to legal action.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED,
 * INCLUDING BUT NOT LIMITED TO WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE,
 * AND NON-INFRINGEMENT.
 * IN NO EVENT SHALL NEHONIX BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY,
 * OR CONSEQUENTIAL DAMAGES ARISING FROM THE USE OR INABILITY TO USE THE SOFTWARE,
 * EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGES.
 *
 ***************************************************************************** */

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec [command]",
	Short: "Execute a command from local node_modules/.bin",
	Aliases: []string{"x"},
	DisableFlagParsing: true,
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
	RootCmd.AddCommand(execCmd)
}
