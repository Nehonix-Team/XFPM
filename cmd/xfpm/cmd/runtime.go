/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package cmd

import (
	"fmt"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/spf13/cobra"
)

var runtimeCmd = &cobra.Command{
	Use:   "runtime [action] [target]",
	Short: "Manage XFPM execution runtimes (Bun)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("missing action or target (e.g., 'xfpm runtime rm bun')")
		}

		action := strings.ToLower(args[0])
		target := strings.ToLower(args[1])

		if action != "rm" && action != "remove" {
			return fmt.Errorf("unknown action: %s", action)
		}

		switch target {
		case "bun":
			return core.RemoveRuntime()
		default:
			return fmt.Errorf("unknown runtime target: %s", target)
		}
	},
}

func init() {
	RootCmd.AddCommand(runtimeCmd)
}
