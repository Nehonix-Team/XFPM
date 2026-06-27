/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2026 Nehonix. All rights reserved.
 ***************************************************************************** */

package cmd

import (
	_ "embed"
	"os"
	"strconv"

	"github.com/Nehonix-Team/XFMP/internal/serve"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

//go:embed templates/serve_ui.xfpml
var serveUiTmpl string

var serveCmd = &cobra.Command{
	Use:   "serve [port] [dir]",
	Short: "Start a modern local file server",
	Long:  `Starts a secure, modern local HTTP server to browse and manage static files.`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")
		
		cwd, err := os.Getwd()
		if err != nil {
			utils.Error("Failed to get current directory: %v", err)
			os.Exit(1)
		}

		// Parse positional arguments
		if len(args) > 0 {
			// If first arg is a number, treat as port
			if p, err := strconv.Atoi(args[0]); err == nil {
				port = p
				if len(args) > 1 {
					cwd = args[1]
				}
			} else {
				// Otherwise treat as directory
				cwd = args[0]
			}
		}

		err = serve.StartServer(port, cwd, serveUiTmpl)
		if err != nil {
			utils.Error("Server error: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	serveCmd.Flags().IntP("port", "p", 6537, "Port to run the server on")
	RootCmd.AddCommand(serveCmd)
}
