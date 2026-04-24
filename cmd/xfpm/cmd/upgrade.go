/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package cmd

import (
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Standard check and installation of the latest XFPM version",
	Long:  `The upgrade command forces a check against the Nehonix GitHub registry to see if a newer version of XFPM is available. If found, it will prompt to execute the official install script to update the binary in-place.`,
	Run: func(cmd *cobra.Command, args []string) {
		utils.CheckForUpdates(true)
	},
}

func init() {
	RootCmd.AddCommand(upgradeCmd)
}
