/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Manage XFPM shared storage",
}

var pruneCmd = &cobra.Command{
	Use:   "prune [path]",
	Short: "Remove unused or legacy storage entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		legacy, _ := cmd.Flags().GetBool("legacy")
		if !legacy {
			utils.Info("Use --legacy to clean up old XFPM versions.")
			return nil
		}

		targetPath := "."
		if len(args) > 0 {
			targetPath = args[0]
		}

		absPath, _ := filepath.Abs(targetPath)
		utils.Matrix(fmt.Sprintf("Scanning for legacy storage in: %s", absPath))

		s, _ := pterm.DefaultSpinner.Start("Searching...")
		roots := core.FindLegacyStorages(absPath)
		
		// SEARCH GLOBAL LEGACY
		globalLegacies := paths.LegacyStoragePaths()
		
		globalFound := []string{}
		for _, g := range globalLegacies {
			if fi, err := os.Stat(g); err == nil && fi.IsDir() {
				globalFound = append(globalFound, g)
			}
		}
		s.Stop()

		if len(roots) == 0 && len(globalFound) == 0 {
			utils.Success("No legacy XFPM storage found.")
			return nil
		}

		utils.Info("Found %d projects and %d global legacy stores.", len(roots), len(globalFound))
		
		// Map for selection
		var options []string
		for _, g := range globalFound {
			options = append(options, "[GLOBAL] "+filepath.Base(g))
		}
		for _, r := range roots {
			options = append(options, r)
		}
		options = append(options, "MIGRATE ALL")
		options = append(options, "CANCEL")

		selected, _ := pterm.DefaultInteractiveSelect.
			WithMaxHeight(15).
			WithOptions(options).
			WithDefaultText("Select storage to migrate").
			Show()

		if selected == "CANCEL" {
			return nil
		}

		var toMigrate []string
		xpmStore := paths.StorageDir()
		cas, _ := core.NewCas(xpmStore)

		if selected == "MIGRATE ALL" {
			toMigrate = append(globalFound, roots...)
		} else if strings.HasPrefix(selected, "[GLOBAL] ") {
			base := strings.TrimPrefix(selected, "[GLOBAL] ")
			toMigrate = []string{filepath.Join(paths.XpmHome(), base)}
		} else {
			toMigrate = []string{selected}
		}

		oldHome := paths.LegacyHome()
		newHome := paths.XpmHome()

		for _, item := range toMigrate {
			pterm.DefaultSection.Printf("Migrating %s", item)
			
			pb, _ := pterm.DefaultProgressbar.
				WithTotal(0).
				WithTitle("  " + pterm.Gray("->") + " Moving files").
				Start()

			// Check if it's the home directory migration
			if item == oldHome {
				err := core.MigrateLegacyHome(oldHome, newHome, cas, func(current, total int, message string) {
					pb.Total = total
					pb.Add(1)
					pb.Title = "  " + pterm.Gray("->") + " Migrated: " + pterm.LightCyan(message)
				})
				pb.Stop()
				if err != nil {
					utils.Error("Failed to migrate home: %v", err)
				} else {
					utils.Success("Completed home migration to %s", newHome)
				}
				fmt.Println()
				continue
			}

			// Traditional storage migration for other paths
			var migratePath string
			if strings.HasPrefix(item, paths.XpmHome()) {
				// Global path
				migratePath = item
			} else {
				// Project path
				migratePath = filepath.Join(paths.LocalXpmDir(item), "storage")
			}

			err := core.MigratePathToCas(migratePath, cas, func(current, total int, message string) {
				pb.Total = total
				pb.Add(1)
				pb.Title = "  " + pterm.Gray("->") + " Migrated: " + pterm.LightCyan(message)
			})

			pb.Stop()

			if err != nil {
				utils.Error("Failed to migrate %s: %v", item, err)
			} else {
				utils.Success("Completed migration for %s", item)
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(storeCmd)
	storeCmd.AddCommand(pruneCmd)
	pruneCmd.Flags().Bool("legacy", false, "Prune legacy storage from older XFPM versions")
}
