/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package main

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
	Aliases: []string{"up", "upgrade"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set resolver into Update mode: always fetch fresh metadata from registry,
		// but still honour the semver constraint already in package.json.
		installCmd.Flags().Lookup("update").Value.Set("true")
		return installCmd.RunE(installCmd, args)
	},
}
