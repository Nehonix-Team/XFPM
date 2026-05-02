package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/Nehonix-Team/xru"
	"github.com/spf13/cobra"
)

var xruCmd = &cobra.Command{
	Use:   "xru <rule_file> [target_dir]",
	Short: "Apply XyPriss Rule Unit (XRU) patches to files",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintBanner()

		rulePath := args[0]
		targetDir := "."
		if len(args) > 1 {
			targetDir = args[1]
		}

		absTarget, err := filepath.Abs(targetDir)
		if err != nil {
			return err
		}

		utils.Premium("XRU Engine", fmt.Sprintf("Applying rules from %s", rulePath))

		rf, err := xru.ParseFile(rulePath)
		if err != nil {
			return err
		}

		for _, rule := range rf.Rules {
			if err := applyXruRule(absTarget, rule); err != nil {
				utils.Error("Rule failure: %v", err)
			}
		}

		utils.Success("XRU transformation complete.")
		return nil
	},
}

func applyXruRule(targetDir string, rule xru.Rule) error {
	switch rule.Type {
	case xru.RuleTypeCreate:
		fullPath := filepath.Join(targetDir, rule.Target)
		utils.Log("+", fmt.Sprintf("Creating %s", rule.Target))
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		return os.WriteFile(fullPath, []byte(rule.Content), 0644)

	case xru.RuleTypeBegin:
		fullPath := filepath.Join(targetDir, rule.Target)
		utils.Log("→", fmt.Sprintf("Patching %s", rule.Target))
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return fmt.Errorf("file %s does not exist", rule.Target)
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		content := string(data)
		for _, action := range rule.Actions {
			content = applyXruAction(content, action, filepath.Ext(fullPath))
		}
		return os.WriteFile(fullPath, []byte(content), 0644)

	case xru.RuleTypeGlobal:
		return filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			ext := filepath.Ext(path)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(data)
			original := content
			for _, action := range rule.Actions {
				content = applyXruAction(content, action, ext)
			}
			if content != original {
				utils.Log("*", fmt.Sprintf("Global match: %s", filepath.Base(path)))
				return os.WriteFile(path, []byte(content), info.Mode())
			}
			return nil
		})
	}
	return nil
}

func applyXruAction(content string, action xru.Action, fileExt string) string {
	switch a := action.(type) {
	case xru.InjectAction:
		if a.Lang != "" {
			targetExt := "." + strings.ToLower(a.Lang)
			if targetExt != fileExt {
				return content
			}
		}
		return xru.InjectCode(content, a.Key, a.Code)
	case xru.PatchAction:
		return xru.ApplyPatch(content, a.Op, a.Value)
	}
	return content
}

func init() {
	RootCmd.AddCommand(xruCmd)
}
