/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package main

import (
	"github.com/spf13/cobra"
)

// updateCmd is an alias to installCmd with force=true and update semantics.
// It re-resolves the full dep tree (always fetching latest matching semver).
var updateCmd = &cobra.Command{
	Use:     "update [packages...]",
	Short:   "Update dependencies to their latest compatible versions",
	Aliases: []string{"up", "upgrade"},
	RunE: func(cmd *cobra.Command, args []string) error {
		installCmd.Flags().Set("force", "true")
		return installCmd.RunE(installCmd, args)
	},
}
