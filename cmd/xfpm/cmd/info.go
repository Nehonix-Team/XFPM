package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info [package]",
	Short: "Show information about a package",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("package name is required")
		}

		pkgName := args[0]
		version := "latest"

		if strings.HasPrefix(pkgName, "@") {
			parts := strings.Split(pkgName, "@")
			if len(parts) > 2 {
				pkgName = "@" + parts[1]
				version = parts[2]
			}
		} else {
			parts := strings.Split(pkgName, "@")
			if len(parts) > 1 {
				pkgName = parts[0]
				version = parts[1]
			}
		}

		registry := core.NewRegistryClient("", 3)

		s, _ := pterm.DefaultSpinner.Start("Fetching package metadata...")

		pkgData, err := registry.FetchPackage(context.Background(), pkgName, false)
		if err != nil {
			s.Stop()
			return fmt.Errorf("failed to fetch package info: %w", err)
		}

		if version == "latest" {
			if v, ok := pkgData.DistTags["latest"]; ok {
				version = v
			}
		}

		metadata, ok := pkgData.Versions[version]
		s.Stop()

		if !ok {
			return fmt.Errorf("version %s not found for package %s", version, pkgName)
		}

		// Display formatted info
		pterm.DefaultSection.Printf("%s@%s", pkgName, version)

		var data [][]string
		data = append(data, []string{pterm.LightCyan("Name:"), metadata.Name})
		data = append(data, []string{pterm.LightCyan("Version:"), metadata.Version})

		if metadata.Dist.Tarball != "" {
			data = append(data, []string{pterm.LightCyan("Tarball:"), metadata.Dist.Tarball})
		}
		if metadata.Dist.UnpackedSize > 0 {
			data = append(data, []string{pterm.LightCyan("Size:"), fmt.Sprintf("%d bytes", metadata.Dist.UnpackedSize)})
		}

		pterm.DefaultTable.WithData(data).Render()

		if len(metadata.Dependencies) > 0 {
			pterm.Println()
			pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgDarkGray)).WithTextStyle(pterm.NewStyle(pterm.FgLightBlue)).Println("Dependencies")
			var deps [][]string
			for k, v := range metadata.Dependencies {
				deps = append(deps, []string{k, pterm.LightGreen(v)})
			}
			pterm.DefaultTable.WithData(deps).Render()
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(infoCmd)
}
