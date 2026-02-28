package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nehonix-Team/xfpm-go/internal/core"
	"github.com/Nehonix-Team/xfpm-go/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	version = "0.1.0-go"
	rootCmd = &cobra.Command{
		Use:   "xfpm",
		Short: "Official XyPriss Fast Package Manager & CLI (Go version)",
		Long:  `Official XyPriss Fast Package Manager & CLI rewritten in Go for cross-platform support.`,
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			utils.SetupUI()
		},
		Run: func(cmd *cobra.Command, args []string) {
			utils.PrintBanner()
			cmd.Help()
		},
	}
)

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.PersistentFlags().StringP("cwd", "C", "", "Change work directory")
	installCmd.Flags().BoolP("save-dev", "D", false, "Save to devDependencies")
	installCmd.Flags().BoolP("save-optional", "O", false, "Save to optionalDependencies")
	installCmd.Flags().BoolP("force", "f", false, "Force re-install even if already extracted")
}

var installCmd = &cobra.Command{
	Use:   "install [packages...]",
	Short: "Install dependencies for the current project",
	Aliases: []string{"i", "add"},
	RunE: func(cmd *cobra.Command, args []string) error {
		startTime := time.Now()
		cwd, _ := cmd.Flags().GetString("cwd")
		if cwd != "" {
			if err := os.Chdir(cwd); err != nil {
				return err
			}
		}

		projectRoot, err := os.Getwd()
		if err != nil {
			return err
		}

		isDev, _ := cmd.Flags().GetBool("save-dev")
		isOptional, _ := cmd.Flags().GetBool("save-optional")
		force, _ := cmd.Flags().GetBool("force")

		xpmDir := filepath.Join(projectRoot, "node_modules", ".xpm")
		cas, err := core.NewCas(filepath.Join(xpmDir, "storage"))
		if err != nil {
			return err
		}

		registry := core.NewRegistryClient("", 3)
		registry.SetCacheDir(filepath.Join(xpmDir, "cache"))

		resolver := core.NewResolver(registry, cas)
		
		utils.Matrix("Analysing project roots...")

		rootDeps := make(map[string]string)
		pkgJsonPath := filepath.Join(projectRoot, "package.json")
		var pkg *core.PackageJson

		if _, err := os.Stat(pkgJsonPath); err == nil {
			pkg, _ = core.LoadPackageJson(pkgJsonPath)
		}

		if len(args) > 0 {
			for _, p := range args {
				parts := strings.Split(p, "@")
				name := p
				req := "latest"
				if len(parts) == 2 {
					name = parts[0]
					req = parts[1]
				} else if len(parts) == 3 && strings.HasPrefix(p, "@") {
					name = "@" + parts[1]
					req = parts[2]
				}
				rootDeps[name] = req
			}
		} else if pkg != nil {
			rootDeps = pkg.AllDependencies()
		} else {
			return fmt.Errorf("no package.json found and no packages specified")
		}

		s, _ := pterm.DefaultSpinner.
			WithRemoveWhenDone(true).
			WithText("Analysing project tree...").
			Start()
		var mu sync.Mutex
		lastResolving := ""

		resolver.SetOnProgress(func(name string) {
			if rand.Float32() < 0.1 {
				pterm.Printf("   %s %-30s %s\n", utils.GreenColor.Sprint("├─"), name, utils.DimColor.Sprint("resolving..."))
			}
			mu.Lock()
			lastResolving = name
			mu.Unlock()
		})

		stopSpinner := make(chan struct{})
		go func() {
			ticker := time.NewTicker(80 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					mu.Lock()
					name := lastResolving
					mu.Unlock()
					if name != "" {
						s.UpdateText("  " + utils.DimColor.Sprint("Searching: ") + utils.AccentColor.Sprint(name) + " " + utils.MatrixColor.Sprint(utils.RandomMatrixString(5)))
					}
				case <-stopSpinner:
					return
				}
			}
		}()
		
		resolved, err := resolver.ResolveTree(context.Background(), rootDeps)
		close(stopSpinner)
		s.Stop()

		if err != nil {
			return err
		}

		utils.Success("Dependency tree resolved successfully (%d total).", len(resolved))

		installer := core.NewInstaller(cas, registry, projectRoot)
		installer.Force = force
		if err := installer.Install(context.Background(), resolved); err != nil {
			return err
		}
		// Update package.json if new packages were added
		if len(args) > 0 && pkg != nil {
			for name := range rootDeps {
				// Find actual version in resolved
				version := "latest"
				for _, r := range resolved {
					if r.Name == name {
						version = "^" + r.Version
						break
					}
				}

				if isDev {
					if pkg.DevDependencies == nil {
						pkg.DevDependencies = make(map[string]string)
					}
					pkg.DevDependencies[name] = version
				} else if isOptional {
					if pkg.OptionalDependencies == nil {
						pkg.OptionalDependencies = make(map[string]string)
					}
					pkg.OptionalDependencies[name] = version
				} else {
					if pkg.Dependencies == nil {
						pkg.Dependencies = make(map[string]string)
					}
					pkg.Dependencies[name] = version
				}
			}
			pkg.Save(pkgJsonPath)
		}

		utils.PrintFooter(time.Since(startTime))
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
