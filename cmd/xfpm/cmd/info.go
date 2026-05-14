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

		s, _ := pterm.DefaultSpinner.Start("Fetching package metadata from registry...")

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

		metadata, err := registry.FetchVersionMetadata(context.Background(), pkgName, version)
		s.Stop()

		if err != nil {
			return fmt.Errorf("failed to fetch version metadata: %w", err)
		}

		// Display Premium Header
		pterm.Println()
		pterm.DefaultBox.WithTitle(pterm.LightBlue(" PACKAGE INFORMATION ")).Println(
			fmt.Sprintf("%s %s\n%s", 
				pterm.Bold.Sprint(metadata.Name), 
				pterm.Gray("v"+metadata.Version),
				pterm.Italic.Sprint(metadata.Description)),
		)

		var data [][]string
		data = append(data, []string{pterm.LightCyan("License:"), metadata.License})
		
		// Author handling (can be string or object)
		authorStr := "Unknown"
		if metadata.Author != nil {
			switch a := metadata.Author.(type) {
			case string:
				authorStr = a
			case map[string]interface{}:
				if name, ok := a["name"].(string); ok {
					authorStr = name
					if email, ok := a["email"].(string); ok {
						authorStr += " <" + email + ">"
					}
				}
			}
		}
		data = append(data, []string{pterm.LightCyan("Author:"), authorStr})

		if len(metadata.Keywords) > 0 {
			data = append(data, []string{pterm.LightCyan("Keywords:"), strings.Join(metadata.Keywords, ", ")})
		}

		if metadata.Dist.UnpackedSize > 0 {
			sizeStr := fmt.Sprintf("%.2f MiB", float64(metadata.Dist.UnpackedSize)/(1024*1024))
			data = append(data, []string{pterm.LightCyan("Unpacked Size:"), sizeStr})
		}
		
		data = append(data, []string{pterm.LightCyan("File Count:"), fmt.Sprintf("%d", metadata.Dist.FileCount)})

		pterm.DefaultTable.WithData(data).Render()

		// Dependencies Section
		if len(metadata.Dependencies) > 0 {
			pterm.Println()
			pterm.DefaultSection.WithLevel(2).Println("Dependencies")
			
			var deps [][]string
			count := 0
			for k, v := range metadata.Dependencies {
				deps = append(deps, []string{"• " + k, pterm.Gray(v)})
				count++
				if count > 20 { // Cap the display for very large packages
					deps = append(deps, []string{pterm.Gray("... and more"), ""})
					break
				}
			}
			pterm.DefaultTable.WithData(deps).Render()
		}

		pterm.Println()
		return nil
	},
}

func init() {
	RootCmd.AddCommand(infoCmd)
}
