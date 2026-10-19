package cmd

import (
	xfpmInit "github.com/Nehonix-Team/XFMP/internal/init"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:                "init [name]",
	Short:              "Initialize a new XyPriss project using the Orchestration Engine",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintBanner()
		return xfpmInit.RunInit(args)
	},
}

func init() {
	RootCmd.AddCommand(initCmd)
}
