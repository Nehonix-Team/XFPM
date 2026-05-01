package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	xfpmInit "github.com/Nehonix-Team/XFMP/internal/init"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new XyPriss project using the Orchestration Engine",
	Annotations: map[string]string{
		"requireRuntime": "true",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintBanner()

		name, _ := cmd.Flags().GetString("name")
		desc, _ := cmd.Flags().GetString("desc")
		version, _ := cmd.Flags().GetString("version")
		mode, _ := cmd.Flags().GetString("mode")
		security, _ := cmd.Flags().GetString("security")
		storage, _ := cmd.Flags().GetString("storage")
		guardrails, _ := cmd.Flags().GetBool("guardrails")
		port, _ := cmd.Flags().GetUint16("port")
		author, _ := cmd.Flags().GetString("author")
		alias, _ := cmd.Flags().GetString("alias")

		// Detect interactive mode
		isInteractive := name == "" || desc == "" || version == ""

		if isInteractive {
			utils.Premium("Setup", "Entering guided orchestration sequence")
			
			// --- Section: Project Identity ---
			pterm.DefaultSection.Println("Project Identity")
			
			if name == "" {
				for {
					name, _ = pterm.DefaultInteractiveTextInput.
						WithDefaultText("my-xypriss-app").
						Show("Project Name")
					if name != "" {
						break
					}
					utils.Error("Project name cannot be empty")
				}
			}

			if desc == "" {
				for {
					desc, _ = pterm.DefaultInteractiveTextInput.
						WithDefaultText("A high-performance XyPriss project").
						Show("Description")
					if desc != "" {
						break
					}
					utils.Error("Description cannot be empty")
				}
			}

			if version == "" {
				for {
					version, _ = pterm.DefaultInteractiveTextInput.
						WithDefaultText("1.0.0").
						Show("Initial Version")
					if version != "" {
						break
					}
					utils.Error("Version cannot be empty")
				}
			}

			// --- Section: System Architecture ---
			fmt.Println()
			pterm.DefaultSection.Println("System Architecture")

			if !cmd.Flags().Changed("mode") {
				mode, _ = pterm.DefaultInteractiveSelect.
					WithOptions([]string{"default", "xms"}).
					WithDefaultOption("default").
					Show("Orchestration Mode")
			}

			if !cmd.Flags().Changed("security") {
				security, _ = pterm.DefaultInteractiveSelect.
					WithOptions([]string{"standard", "api", "web"}).
					WithDefaultOption("standard").
					Show("Security Profile")
			}

			if !cmd.Flags().Changed("storage") {
				storage, _ = pterm.DefaultInteractiveSelect.
					WithOptions([]string{"none", "xems"}).
					WithDefaultOption("none").
					Show("Storage Engine")
			}

			if !cmd.Flags().Changed("guardrails") {
				guardrails, _ = pterm.DefaultInteractiveConfirm.WithDefaultValue(false).Show("Enable Network Guardrails?")
			}

			if !cmd.Flags().Changed("port") {
				pStr, _ := pterm.DefaultInteractiveTextInput.WithDefaultText("8080").Show("Primary Network Port")
				fmt.Sscanf(pStr, "%d", &port)
			}

			// --- Section: Review & Launch ---
			fmt.Println()
			pterm.DefaultSection.Println("Review & Launch")
			
			tableData := [][]string{
				{"Parameter", "Selected Value"},
				{"Project Name", name},
				{"Description", desc},
				{"Version", version},
				{"Mode", mode},
				{"Security", security},
				{"Storage", storage},
				{"Port", fmt.Sprintf("%d", port)},
				{"Guardrails", fmt.Sprintf("%v", guardrails)},
			}

			pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
			fmt.Println()

			confirm, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).Show("Proceed with orchestration?")
			if !confirm {
				utils.Warn("Orchestration aborted by user.")
				return nil
			}
			fmt.Println()
		} else {
			// In non-interactive mode, validate required flags
			if name == "" || desc == "" || version == "" {
				return fmt.Errorf("missing required flags: --name, --desc, --version")
			}
		}

		targetDir, _ := os.Getwd()
		targetDir = filepath.Join(targetDir, name)

		force, _ := cmd.Flags().GetBool("force")

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
			Security:    security,
			Guardrails:  guardrails,
			Storage:     storage,
			ProjectName: name,
			Description: desc,
			Author:      author,
			Version:     version,
			Alias:       alias,
			Port:        port,
			MainPort:    port,
			AuthPort:    port + 1,
			TargetDir:   targetDir,
		}

		if err := xfpmInit.RunOrchestration(opts); err != nil {
			utils.Error("Initialization failed: %v", err)
			return err
		}

		utils.Success("Project orchestration complete.")

		// Dependencies initialization
		utils.Log("!", "Initializing dependencies...")

		if _, err := exec.LookPath("bun"); err != nil {
			utils.Warn("Bun not found globally. Installing it via xfpm...")
			bunInstall := exec.Command(os.Args[0], "install", "-g", "bun")
			bunInstall.Stdout = os.Stdout
			bunInstall.Stderr = os.Stderr
			if err := bunInstall.Run(); err != nil {
				utils.Warn("Could not install bun automatically: %v", err)
			}
		}

		selfInstall := exec.Command(os.Args[0], "install", "--force")
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
	initCmd.Flags().StringP("name", "n", "", "Project name")
	initCmd.Flags().StringP("desc", "d", "", "Project description")
	initCmd.Flags().StringP("version", "v", "", "Initial version")
	initCmd.Flags().StringP("author", "a", "XyPriss Developer", "Author name")
	initCmd.Flags().StringP("alias", "A", "", "Project alias")
	initCmd.Flags().StringP("mode", "m", "default", "Orchestration mode (default or xms)")
	initCmd.Flags().StringP("security", "s", "standard", "Security level (standard, api, or web)")
	initCmd.Flags().BoolP("guardrails", "g", false, "Enable network guardrails")
	initCmd.Flags().StringP("storage", "S", "none", "Storage engine (none or xems)")
	initCmd.Flags().Uint16P("port", "p", 8080, "Base server port")
	initCmd.Flags().BoolP("force", "f", false, "Force overwrite existing directory")
}
