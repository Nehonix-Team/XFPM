/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 *
 * @author Nehonix
 * @license Nehonix OSL (NOSL)
 *
 * Copyright (c) 2025 Nehonix. All rights reserved.
 *
 * This License governs the use, modification, and distribution of software
 * provided by NEHONIX under its open source projects.
 * NEHONIX is committed to fostering collaborative innovation while strictly
 * protecting its intellectual property rights.
 * Violation of any term of this License will result in immediate termination of all granted rights
 * and may subject the violator to legal action.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED,
 * INCLUDING BUT NOT LIMITED TO WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE,
 * AND NON-INFRINGEMENT.
 * IN NO EVENT SHALL NEHONIX BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY,
 * OR CONSEQUENTIAL DAMAGES ARISING FROM THE USE OR INABILITY TO USE THE SOFTWARE,
 * EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGES.
 *
 ***************************************************************************** */

package cmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/google/uuid"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const TemplateURL = "https://dll.nehonix.com/dl/mds/xypriss/templates/initdr.zip"

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new XyPriss project",
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintBanner()
		name, _ := cmd.Flags().GetString("name")
		desc, _ := cmd.Flags().GetString("desc")
		lang, _ := cmd.Flags().GetString("lang")
		port, _ := cmd.Flags().GetUint16("port")
		author, _ := cmd.Flags().GetString("author")
		alias, _ := cmd.Flags().GetString("alias")

		if name == "" {
			name = "my-xypriss-app"
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

		langFolder := "TS"
		if strings.ToLower(lang) == "js" || strings.ToLower(lang) == "javascript" {
			langFolder = "JS"
		}


		s, _ := pterm.DefaultSpinner.WithText("Downloading template...").Start()
		zipPath, err := downloadTemplate()
		if err != nil {
			s.Fail("Failed to download template")
			return err
		}
		s.Success("Template downloaded")
		defer os.Remove(zipPath)

		s, _ = pterm.DefaultSpinner.WithText("Extracting project structure...").Start()
		if err := extractTemplate(zipPath, targetDir, langFolder); err != nil {
			s.Fail("Failed to extract template")
			return err
		}
		s.Success("Template extracted")

		s, _ = pterm.DefaultSpinner.WithText("Configuring metadata...").Start()
		if err := customizeProject(targetDir, name, desc, author, alias, port); err != nil {
			s.Fail("Failed to configure metadata")
			return err
		}

		if err := processDotConfig(targetDir); err != nil {
			utils.Warn("Failed to process .config file: %v", err)
		}
		s.Success("Metadata configured")

		utils.Success("Project setup complete. Initializing dependencies...")

		// Check for bun — install via our own global installer, not npm
		if _, err := exec.LookPath("bun"); err != nil {
			utils.Warn("Bun not found globally. Installing it via xfpm...")
			bunInstall := exec.Command(os.Args[0], "install", "-g", "bun")
			bunInstall.Stdout = os.Stdout
			bunInstall.Stderr = os.Stderr
			if err := bunInstall.Run(); err != nil {
				utils.Warn("Could not install bun automatically: %v", err)
			} else {
				utils.Success("Bun installed globally.")
			}
		}

		// Run full install via our own installer
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
	initCmd.Flags().StringP("lang", "l", "ts", "Language (ts or js)")
	initCmd.Flags().Uint16P("port", "p", 8080, "Server port")
	initCmd.Flags().StringP("author", "a", "", "Author name")
	initCmd.Flags().String("alias", "", "App alias")
	initCmd.Flags().BoolP("force", "f", false, "Force overwrite existing directory")
}

func downloadTemplate() (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest("GET", TemplateURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "XyPriss/1.0 (Go; +https://github.com/Nehonix-Team/XyPriss)")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download template: HTTP %d", resp.StatusCode)
	}

	tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("xfpm-template-%s.zip", uuid.New().String()))
	out, err := os.Create(tempPath)
	if err != nil {
		return "", err
	}

	n, err := io.Copy(out, resp.Body)
	out.Close() // Close BEFORE returning to ensure flush and allow opening by zip reader

	if err != nil {
		return "", fmt.Errorf("failed to write template: %w", err)
	}

	if n == 0 {
		return "", fmt.Errorf("downloaded template is empty")
	}

	return tempPath, nil
}

func extractTemplate(zipPath, targetDir, langFolder string) error {
	f, err := os.Open(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Try to detect format. Server sends TGZ but names it .zip
	header := make([]byte, 2)
	f.Read(header)
	f.Seek(0, 0)

	if header[0] == 0x1f && header[1] == 0x8b {
		// Gzip detected
		return extractTarGz(f, targetDir, langFolder)
	}

	// Fallback to Zip
	return extractZip(zipPath, targetDir, langFolder)
}

func extractTarGz(r io.Reader, targetDir, langFolder string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	prefix := "templates/" + langFolder + "/"

	os.MkdirAll(targetDir, 0755)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if !strings.HasPrefix(header.Name, prefix) {
			continue
		}

		relPath := strings.TrimPrefix(header.Name, prefix)
		if relPath == "" {
			continue
		}

		fpath := filepath.Join(targetDir, relPath)

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(fpath, 0755)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(fpath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
			os.Chmod(fpath, os.FileMode(header.Mode))
		}
	}
	return nil
}

func extractZip(zipPath, targetDir, langFolder string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("zip: %w", err)
	}
	defer r.Close()

	os.MkdirAll(targetDir, 0755)

	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, langFolder+"/") {
			continue
		}

		relPath := strings.TrimPrefix(f.Name, langFolder+"/")
		if relPath == "" {
			continue
		}

		fpath := filepath.Join(targetDir, relPath)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

func customizeProject(targetDir, name, desc, author, alias string, port uint16) error {
	// Customize package.json
	pkgPath := filepath.Join(targetDir, "package.json")
	if pkg, err := core.LoadPackageJson(pkgPath); err == nil {
		pkg.Name = strings.ToLower(name)
		if desc != "" {
			pkg.Description = desc
		}
		if author != "" {
			pkg.Author = author
		}
		if err := pkg.Save(pkgPath); err != nil {
			return fmt.Errorf("failed to save package.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		utils.Warn("Could not load package.json for customization: %v", err)
	}

	// Customize .env
	envPath := filepath.Join(targetDir, ".env")
	if data, err := os.ReadFile(envPath); err == nil {
		content := string(data)
		content = strings.ReplaceAll(content, "{{PORT}}", fmt.Sprintf("%d", port))
		content = strings.ReplaceAll(content, "PORT=8080", fmt.Sprintf("PORT=%d", port))
		if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write to .env file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		utils.Warn("Could not load .env for customization: %v", err)
	}

	// Customize xypriss.config.json
	customizeConfig(targetDir, name, desc, author, alias, port)

	// Customize README.md
	customizeReadme(targetDir, name, desc, port)

	return nil
}

func customizeConfig(root, name, desc, author, alias string, port uint16) {
	path := filepath.Join(root, "xypriss.config.json")
	
	// Simply create/update the __sys__ metadata
	// A more robust implementation would use a proper JSON map
	utils.Log("!", "Updating xypriss.config.json...")
	
	if desc == "" {
		desc = "XyPriss project: " + name
	}
	if author == "" {
		author = "User"
	}
	if alias == "" {
		alias = name
	}

	// Quick hack for customization since we don't have a full Config struct yet
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	content := string(data)
	// This is slightly dangerous but works for the template's structure
	metadata := fmt.Sprintf(`  "__sys__": {
    "__version__": "1.0.0",
    "__author__": "%s",
    "__name__": "%s",
    "__description__": "%s",
    "__alias__": "%s",
    "__port__": %d,
    "__PORT__": %d
  },`, author, name, desc, alias, port, port)

	if strings.Contains(content, "{") {
		content = strings.Replace(content, "{", "{\n"+metadata, 1)
		os.WriteFile(path, []byte(content), 0644)
	}
}

func customizeReadme(root, name, desc string, port uint16) {
	path := filepath.Join(root, "README.md")
	if data, err := os.ReadFile(path); err == nil {
		content := string(data)
		content = strings.ReplaceAll(content, "{{PROJECT_NAME}}", name)
		content = strings.ReplaceAll(content, "{{PROJECT_DESCRIPTION}}", desc)
		content = strings.ReplaceAll(content, "{{PORT}}", fmt.Sprintf("%d", port))
		content = strings.ReplaceAll(content, "{{FEATURES}}", "")
		os.WriteFile(path, []byte(content), 0644)
	}
}

func processDotConfig(root string) error {
	configPath := filepath.Join(root, ".config")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}

	utils.Log("!", "Processing .config for dependencies...")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var deps, devDeps []string
	inDeps, inDevDeps := false, false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "deps:") || strings.HasPrefix(lower, "dependencies:") {
			inDeps = true
			inDevDeps = false
			continue
		} else if strings.HasPrefix(lower, "devdeps:") || strings.HasPrefix(lower, "devdependencies:") {
			inDeps = false
			inDevDeps = true
			continue
		}

		// Handle list syntax "- package"
		pkgName := strings.TrimPrefix(trimmed, "-")
		pkgName = strings.TrimSpace(pkgName)

		if inDeps {
			deps = append(deps, pkgName)
		} else if inDevDeps {
			devDeps = append(devDeps, pkgName)
		}
	}

	// Update package.json
	pkgPath := filepath.Join(root, "package.json")
	pkg, err := core.LoadPackageJson(pkgPath)
	if err != nil {
		return err
	}

	if pkg.Dependencies == nil {
		pkg.Dependencies = make(map[string]string)
	}
	if pkg.DevDependencies == nil {
		pkg.DevDependencies = make(map[string]string)
	}

	for _, d := range deps {
		pkg.Dependencies[d] = "latest"
	}
	for _, d := range devDeps {
		pkg.DevDependencies[d] = "latest"
	}

	if err := pkg.Save(pkgPath); err != nil {
		return err
	}

	// Cleanup
	os.Remove(configPath)
	return nil
}
