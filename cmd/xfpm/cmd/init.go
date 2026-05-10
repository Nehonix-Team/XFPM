package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"strings"

	xfpmInit "github.com/Nehonix-Team/XFMP/internal/init"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Initialize a new XyPriss project using the Orchestration Engine",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintBanner()

		var name string
		var customArgs []string

		for _, arg := range args {
			if strings.Contains(arg, "=") {
				customArgs = append(customArgs, arg)
			} else if name == "" {
				name = arg
			}
		}

		mode, _ := cmd.Flags().GetString("mode")
		force, _ := cmd.Flags().GetBool("force")
		version, _ := cmd.Flags().GetString("version")
		description, _ := cmd.Flags().GetString("description")
		author, _ := cmd.Flags().GetString("author")
		port, _ := cmd.Flags().GetString("port")

		// Determine if we should go into interactive mode for missing fields
		// We go full interactive if no positional arguments were provided
		isInteractive := len(args) == 0

		// Add custom args from --arg flag if any
		extraArgs, _ := cmd.Flags().GetStringSlice("arg")
		customArgs = append(customArgs, extraArgs...)

		// Interactive prompts
		if name == "" {
			name, _ = pterm.DefaultInteractiveTextInput.WithDefaultText("my-xypriss-app").Show("Project Name")
			if name == "" {
				return fmt.Errorf("project name is required")
			}
		}

		if isInteractive || (cmd.Flags().Changed("mode") && !cmd.Flags().Changed("version")) {
			if !cmd.Flags().Changed("version") {
				version, _ = pterm.DefaultInteractiveTextInput.WithDefaultText(version).Show("Project Version")
			}
			if !cmd.Flags().Changed("description") {
				description, _ = pterm.DefaultInteractiveTextInput.WithDefaultText(description).Show("Project Description")
			}
			if !cmd.Flags().Changed("author") {
				author, _ = pterm.DefaultInteractiveTextInput.WithDefaultText(author).Show("Project Author")
			}
			if !cmd.Flags().Changed("port") {
				port, _ = pterm.DefaultInteractiveTextInput.WithDefaultText(port).Show("Project Port")
			}
		}

		targetDir, _ := os.Getwd()
		targetDir = filepath.Join(targetDir, name)

		if _, err := os.Stat(targetDir); err == nil {
			if !force {
				utils.Warn("Directory '%s' already exists.", name)
				return fmt.Errorf("directory already exists (use --force to overwrite)")
			}
			utils.Log("!", "Removing existing directory...")
			os.RemoveAll(targetDir)
		}

		opts := xfpmInit.InitOptions{
			Mode:        mode,
			ProjectName: name,
			TargetDir:   targetDir,
			Version:     version,
			Description: description,
			Author:      author,
			Port:        port,
			CustomArgs:  customArgs,
		}

		if err := xfpmInit.RunOrchestration(opts); err != nil {
			utils.Error("Initialization failed: %v", err)
			return err
		}

		utils.Success("Project orchestration complete.")

		// Dependencies initialization
		utils.Log("!", "Initializing dependencies...")

		// Check if bun is available either in PATH or in XFPM's bin directory
		hasBun := false
		if _, err := exec.LookPath("bun"); err == nil {
			hasBun = true
		} else {
			xfpmBun := filepath.Join(paths.BinDir(), "bun")
			if runtime.GOOS == "windows" { xfpmBun += ".exe" }
			if _, err := os.Stat(xfpmBun); err == nil {
				hasBun = true
			}
		}

		if !hasBun {
			utils.Warn("Bun not found globally. Installing it via xfpm...")
			bunInstall := exec.Command(os.Args[0], "install", "-g", "bun")
			bunInstall.Env = append(os.Environ(), "XFPM_INTERNAL_SPAWN=true")
			bunInstall.Stdout = os.Stdout
			bunInstall.Stderr = os.Stderr
			if err := bunInstall.Run(); err != nil {
				utils.Warn("Could not install bun automatically: %v", err)
			}
		}

		selfInstall := exec.Command(os.Args[0], "install", "--force")
		selfInstall.Env = append(os.Environ(), "XFPM_INTERNAL_SPAWN=true")
		selfInstall.Dir = targetDir
		selfInstall.Stdout = os.Stdout
		selfInstall.Stderr = os.Stderr

		if err := selfInstall.Run(); err != nil {
			utils.Error("Failed to auto-install dependencies: %v", err)
		} else {
			utils.Success("Dependencies installed successfully.")
		}

		fmt.Println()
		utils.Premium("Ready", "Neural bridge active")
		utils.Log("→", fmt.Sprintf("cd %s", name))
		utils.Log("→", "xfpm run dev")
		fmt.Println()

		return nil
	},
}

func init() {
	RootCmd.AddCommand(initCmd)
	initCmd.Flags().StringP("mode", "m", "default", "Orchestration mode (default or xms)")
	initCmd.Flags().BoolP("force", "f", false, "Force overwrite existing directory")
	initCmd.Flags().StringP("version", "v", "1.0.0", "Project version")
	initCmd.Flags().StringP("description", "d", "A high-performance XyPriss server.", "Project description")
	initCmd.Flags().StringP("author", "a", "Nehonix-Team", "Project author")
	initCmd.Flags().StringP("port", "p", "8088", "Default server port")
	initCmd.Flags().StringSlice("arg", []string{}, "Custom arguments for orchestration (key=value)")
}
