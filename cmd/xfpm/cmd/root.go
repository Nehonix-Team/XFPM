/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package cmd

import (
	"fmt"
	"os"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "xfpm",
	Short: "Official XyPriss Fast Package Manager & CLI (Go version)",
	Long:  `Official XyPriss Fast Package Manager (XFPM). XFPM is a high-performance, cross-platform CLI tool built for the XyPriss ecosystem. Written in Go, it delivers fast dependency resolution, strict package isolation through a virtual store, and a clean terminal interface designed for professional workflows.`,
	Version: utils.BinVersion,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		utils.SetupUI()
		
		cwd, _ := cmd.Flags().GetString("cwd")
		if cwd != "" {
			if err := os.Chdir(cwd); err != nil {
				return fmt.Errorf("failed to change directory to %s: %w", cwd, err)
			}
		}

		// Automated JS Runtime Check (Bun)
		if err := core.EnsureRuntime(); err != nil {
			utils.Error("Runtime verification failed: %v", err)
		}

		utils.CheckForUpdates(false)
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		utils.PrintBanner()
		cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		utils.Error("%v", err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringP("cwd", "C", "", "Change work directory")
}
