package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show detailed XFPM version and check for updates",
	Run: func(cmd *cobra.Command, args []string) {
		s, _ := pterm.DefaultSpinner.Start("Checking registry for updates...")

		client := http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(utils.VersionCheckURL)
		
		desc := "The ultra-fast package manager for the XyPriss ecosystem."
		
		if err != nil || resp.StatusCode != http.StatusOK {
			s.Fail("Could not connect to Nehonix Registry. You are currently offline or the server is unreachable.")
			pterm.DefaultBox.WithTitle(pterm.LightBlue(" XFPM ENVIRONMENT ")).Println(
				fmt.Sprintf("%s %s\n%s",
					pterm.Bold.Sprint("XyPriss Fast Package Manager"),
					pterm.Gray(utils.BinVersion),
					pterm.Italic.Sprint(desc),
				),
			)
			return
		}
		defer resp.Body.Close()

		var data struct {
			Latest      string `json:"latest"`
			Message     string `json:"message"`
			Description string `json:"description"`
			Metadata    struct {
				ReleaseDate string `json:"releaseDate"`
				Channel     string `json:"channel"`
			} `json:"metadata"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			s.Fail("Received malformed data from registry.")
			return
		}
 
		s.Success("Connected to Nehonix Registry")
		pterm.Println()

		if data.Description != "" {
			desc = data.Description
		}

		pterm.DefaultBox.WithTitle(pterm.LightBlue(" XFPM ENVIRONMENT ")).Println(
			fmt.Sprintf("%s %s\n%s",
				pterm.Bold.Sprint("XyPriss Fast Package Manager"),
				pterm.Gray(utils.BinVersion),
				pterm.Italic.Sprint(desc),
			),
		)

		var tableData [][]string
		
		status := pterm.FgGreen.Sprint("Up to date")
		if data.Latest != utils.BinVersion && data.Latest != "" {
			// A simple lexicographical comparison since we use vG0.1.x
			if data.Latest > utils.BinVersion {
				status = pterm.FgYellow.Sprint("Update Available (Run 'xfpm upgrade')")
			} else {
				status = pterm.FgCyan.Sprint("Ahead of stable release")
			}
		}

		tableData = append(tableData, []string{pterm.LightCyan("Local Version:"), utils.BinVersion, status})
		tableData = append(tableData, []string{pterm.LightCyan("Remote Version:"), data.Latest, ""})
		
		if data.Metadata.ReleaseDate != "" {
			tableData = append(tableData, []string{pterm.LightCyan("Release Date:"), data.Metadata.ReleaseDate, ""})
		}
		if data.Metadata.Channel != "" {
			tableData = append(tableData, []string{pterm.LightCyan("Channel:"), data.Metadata.Channel, ""})
		}

		pterm.DefaultTable.WithData(tableData).Render()

		if data.Message != "" {
			pterm.Println()
			pterm.Printf("   %s %s\n", pterm.FgCyan.Sprint("ℹ"), pterm.Italic.Sprint(data.Message))
		}
		pterm.Println()
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
