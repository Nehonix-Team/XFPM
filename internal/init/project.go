/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 *
 * @author Nehonix
 * @license Nehonix OSL (NOSL)
 *
 * Copyright (c) 2025 Nehonix. All rights reserved.
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
	ProjectName string
	TargetDir   string
	Version     string
	Description string
	Author      string
	Port        string
	CustomArgs  []string
}

// RunOrchestration runs the full project initialization workflow.
func RunOrchestration(opts InitOptions) error {
	tempDir, err := DownloadLatestTemplate()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// 2. Unzip
	utils.Log("!", "Fetching latest XyPriss templates...")
	zipPath := tempDir
	extractDir, err := os.MkdirTemp("", "xfpm-template-extract")
	if err != nil {
		return fmt.Errorf("failed to create extraction directory: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if err := unzip(zipPath, extractDir); err != nil {
		return err
	}

	orchestrator := NewOrchestrator(opts.TargetDir)

	// 3. Create target directory and copy EVERYTHING
	if err := os.MkdirAll(opts.TargetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Detect zip root folder (GitHub usually wraps everything in a folder)
	files, err := os.ReadDir(extractDir)
	root := extractDir
	if err == nil && len(files) == 1 && files[0].IsDir() {
		root = filepath.Join(extractDir, files[0].Name())
	}

	utils.Log("!", "Setting up project structure...")
	if err := o_copyDir(root, opts.TargetDir, true); err != nil {
		return err
	}

	// 4. Apply Orchestration Rule
	// We prioritize the unified orchestrate.xru, otherwise fallback to mode-specific rules.
	orchestratePath := filepath.Join(root, "rules", "orchestrate.xru")
	modeRulePath := filepath.Join(root, "rules", fmt.Sprintf("%s.xru", opts.Mode))

	if _, err := os.Stat(orchestratePath); err == nil {
		utils.Log("!", "Applying unified orchestration rule...")
		// Execute orchestrate.xru with provided metadata
		args := map[string]string{
			"MODE":        opts.Mode,
			"NAME":        opts.ProjectName,
			"VERSION":     opts.Version,
			"DESCRIPTION": opts.Description,
			"AUTHOR":      opts.Author,
			"PORT":        opts.Port,
		}

		// Inject custom arguments (key=value)
		for _, arg := range opts.CustomArgs {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				args[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}

		if err := orchestrator.ApplyRulesFileWithArgs(orchestratePath, args); err != nil {
			return err
		}
	} else if _, err := os.Stat(modeRulePath); err == nil {
		utils.Log("!", fmt.Sprintf("Applying %s orchestration rule...", opts.Mode))
		if err := orchestrator.ApplyRulesFile(modeRulePath); err != nil {
			return err
		}
	} else {
		utils.Warn("No orchestration rule found (tried orchestrate.xru and %s.xru). Skipping.", opts.Mode)
	}

	return nil
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

// o_copyDir copies files from src to dst. If isBase is true, it skips the internal 'main/' folder.
func o_copyDir(src, dst string, isBase bool) error {
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

		// Skip internal orchestration metadata (main/) when copying from root
		if isBase && (rel == "main" || strings.HasPrefix(rel, "main" + string(os.PathSeparator))) {
			return filepath.SkipDir
		}

		targetPath := filepath.Join(dst, rel)

		if info.IsDir() {
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
