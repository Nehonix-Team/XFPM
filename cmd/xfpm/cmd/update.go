/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package cmd

import (
	"github.com/spf13/cobra"
)

// updateCmd re-resolves packages staying within their declared semver constraints.
//
// Behavior:
//   - Without args: updates ALL top-level deps in package.json to the latest
//     version that satisfies their current constraint (e.g. "^1.2.0" → "1.5.0",
//     never "2.0.0"). Equivalent to "respect the version defined in the package".
//   - With args: only those specific packages are re-resolved and re-extracted.
//     The constraint already defined for each package in package.json is used;
//     no new constraint is applied unless explicitly provided (e.g. pkg@^2).
//
// In both cases the sub-dependencies of updated packages are also refreshed,
// but they must satisfy the constraint imposed by the package that declares them.
var updateCmd = &cobra.Command{
	Use:     "update [packages...]",
	Short:   "Update dependencies to their latest compatible versions",
	Long:    "Update packages while respecting the version constraints declared in package.json. Without arguments, all top-level dependencies are refreshed.",
	Aliases: []string{"up"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set resolver into Update mode: always fetch fresh metadata from registry,
		// but still honour the semver constraint already in package.json.
		installCmd.Flags().Lookup("update").Value.Set("true")
		if global, _ := cmd.Flags().GetBool("global"); global {
			installCmd.Flags().Lookup("global").Value.Set("true")
		}
		return installCmd.RunE(installCmd, args)
	},
}

func init() {
	RootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolP("global", "g", false, "Update packages globally")
	updateCmd.Flags().Bool("verify", false, "Automatically verify plugins during update")
	updateCmd.Flags().BoolP("no-interact", "n", false, "Disable interactive prompts and auto-verify valid signatures")
}
