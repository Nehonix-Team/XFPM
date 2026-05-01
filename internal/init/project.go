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

package init

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/utils"
)

type InitOptions struct {
	Mode        string
	Security    string
	Guardrails  bool
	Storage     string
	ProjectName string
	Description string
	Author      string
	Version     string
	Alias       string
	Port        uint16
	MainPort    uint16
	AuthPort    uint16
	TargetDir   string
}

// RunOrchestration runs the full project initialization workflow.
func RunOrchestration(opts InitOptions) error {
	// 1. Download
	utils.Log("!", "Fetching latest XyPriss templates...")
	zipPath, err := DownloadLatestTemplate()
	if err != nil {
		return err
	}
	defer os.Remove(zipPath)

	// 2. Extract to temp
	tempDir, err := os.MkdirTemp("", "xfpm-init-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := unzip(zipPath, tempDir); err != nil {
		return err
	}

	orchestrator := NewOrchestrator(opts.TargetDir)

	// 3. Apply Base Files (Root files in zip)
	// Create target directory
	if err := os.MkdirAll(opts.TargetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Detect zip root folder (GitHub usually wraps everything in a folder)
	files, err := os.ReadDir(tempDir)
	if err == nil && len(files) == 1 && files[0].IsDir() {
		tempDir = filepath.Join(tempDir, files[0].Name())
	}

	utils.Log("!", "Setting up base project structure...")
	if err := o_copyDir(tempDir, opts.TargetDir, true); err != nil {
		return err
	}

	// 4. Apply Mode
	modeDir := filepath.Join(tempDir, "main", "mode", opts.Mode)
	if _, err := os.Stat(modeDir); err == nil {
		utils.Log("!", fmt.Sprintf("Applying mode: %s...", opts.Mode))
		if err := o_copyDir(modeDir, opts.TargetDir, false); err != nil {
			return err
		}
		rulesPath := filepath.Join(modeDir, "rules.xru")
		if _, err := os.Stat(rulesPath); err == nil {
			if err := orchestrator.ApplyRulesFile(rulesPath); err != nil {
				return err
			}
		}
	} else {
		utils.Warn("Mode '%s' not found in templates.", opts.Mode)
	}

	// 5. Apply Features
	features := []struct {
		name string
		path string
		cond bool
	}{
		{"security", filepath.Join(tempDir, "main", "features", "security", opts.Security), true},
		{"guardrails", filepath.Join(tempDir, "main", "features", "guardrails", "true"), opts.Guardrails},
		{"storage", filepath.Join(tempDir, "main", "features", "storage", opts.Storage), opts.Storage != "" && opts.Storage != "none"},
	}

	for _, f := range features {
		if !f.cond {
			continue
		}
		if _, err := os.Stat(f.path); err == nil {
			utils.Log("!", fmt.Sprintf("Applying feature: %s (%s)...", f.name, filepath.Base(f.path)))
			if err := o_copyDir(f.path, opts.TargetDir, false); err != nil {
				return err
			}
			rulesPath := filepath.Join(f.path, "rules.xru")
			if _, err := os.Stat(rulesPath); err == nil {
				if err := orchestrator.ApplyRulesFile(rulesPath); err != nil {
					return err
				}
			}
		}
	}

	// 6. Global Variable Injection
	utils.Log("!", "Injecting project variables...")
	if err := ReplaceVariables(opts); err != nil {
		return err
	}

	// 7. Cleanup/Renaming
	targetReadme := filepath.Join(opts.TargetDir, "TARGET_README.md")
	if _, err := os.Stat(targetReadme); err == nil {
		finalReadme := filepath.Join(opts.TargetDir, "README.md")
		if err := os.Rename(targetReadme, finalReadme); err != nil {
			utils.Warn("Failed to rename TARGET_README.md: %v", err)
		}
	}

	return nil
}

// ReplaceVariables scans the target directory and replaces placeholders.
func ReplaceVariables(opts InitOptions) error {
	vars := map[string]string{
		"{{SERVER_NAME}}": opts.ProjectName,
		"{{DESCRIPTION}}": opts.Description,
		"{{AUTHOR}}":      opts.Author,
		"{{VERSION}}":     opts.Version,
		"{{ALIAS}}":       opts.Alias,
		"{{PORT}}":        fmt.Sprintf("%d", opts.Port),
		"{{MAIN_PORT}}":   fmt.Sprintf("%d", opts.MainPort),
		"{{AUTH_PORT}}":   fmt.Sprintf("%d", opts.AuthPort),
	}

	return filepath.Walk(opts.TargetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.Contains(path, ".git") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		content := string(data)
		changed := false
		for k, v := range vars {
			if strings.Contains(content, k) {
				content = strings.ReplaceAll(content, k, v)
				changed = true
			}
		}

		if changed {
			return os.WriteFile(path, []byte(content), info.Mode())
		}
		return nil
	})
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
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

// o_copyDir copies files from src to dst. If rootOnly is true, it only copies files at the root of src.
// It skips rules.xru.
func o_copyDir(src, dst string, rootOnly bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return os.MkdirAll(dst, 0755)
		}

		// Skip main directory when copying from root
		if rootOnly && strings.HasPrefix(rel, "main") {
			return filepath.SkipDir
		}

		// Skip rules.xru
		if rel == "rules.xru" {
			return nil
		}

		targetPath := filepath.Join(dst, rel)

		if info.IsDir() {
			if rootOnly && rel != "." {
				// Don't recurse if rootOnly (except for root itself which is handled above)
				return filepath.SkipDir
			}
			return os.MkdirAll(targetPath, info.Mode())
		}

		// Copy file
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, in)
		return err
	})
}
