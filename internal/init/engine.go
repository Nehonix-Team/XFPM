/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 *
 * @author Nehonix
 * @license Nehonix OSL (NOSL)
 ***************************************************************************** */

package init

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/Nehonix-Team/XFMP/internal/xru"
)

// Orchestrator applies XRU rules against a target project directory.
type Orchestrator struct {
	TargetDir string
}

func NewOrchestrator(targetDir string) *Orchestrator {
	return &Orchestrator{TargetDir: targetDir}
}

// ApplyRulesFile parses an .xru file and applies every rule in order.
func (o *Orchestrator) ApplyRulesFile(path string) error {
	rf, err := xru.ParseFile(path)
	if err != nil {
		return err
	}
	for _, rule := range rf.Rules {
		if err := o.ApplyRule(rule); err != nil {
			label := rule.Target
			if label == "" {
				label = "<global>"
			}
			return fmt.Errorf("applying rule for %s: %w", label, err)
		}
	}
	return nil
}

// ApplyRule dispatches a single rule to the appropriate handler.
func (o *Orchestrator) ApplyRule(rule xru.Rule) error {
	switch rule.Type {
	case xru.RuleTypeGlobal:
		return o.applyGlobal(rule)
	case xru.RuleTypeCreate:
		return o.applyCreate(rule)
	case xru.RuleTypeBegin:
		return o.applyPatch(rule)
	}
	return nil
}

func (o *Orchestrator) applyGlobal(rule xru.Rule) error {
	return filepath.Walk(o.TargetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Global rules apply to common text-based config/source files
		ext := filepath.Ext(path)
		if info.IsDir() || (ext != ".ts" && ext != ".json" && ext != ".jsonc" && ext != ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		original := content

		for _, action := range rule.Actions {
			switch a := action.(type) {
			case xru.TSInjectAction:
				content = xru.InjectCode(content, a.Key, a.Code)
			case xru.PatchAction:
				content = xru.ApplyPatch(content, a.Op, a.Value)
			}
		}

		if content != original {
			return os.WriteFile(path, []byte(content), info.Mode())
		}
		return nil
	})
}

func (o *Orchestrator) applyCreate(rule xru.Rule) error {
	fullPath := filepath.Join(o.TargetDir, rule.Target)
	utils.Log("→", fmt.Sprintf("Creating %s", rule.Target))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(rule.Content), 0644)
}

func (o *Orchestrator) applyPatch(rule xru.Rule) error {
	fullPath := filepath.Join(o.TargetDir, rule.Target)
	utils.Log("→", fmt.Sprintf("Patching %s", rule.Target))

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		utils.Warn("Skipping rule for %s: file does not exist", rule.Target)
		return nil
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}
	content := string(data)

	for _, action := range rule.Actions {
		switch a := action.(type) {
		case xru.TSInjectAction:
			content = xru.InjectCode(content, a.Key, a.Code)
		case xru.PatchAction:
			content = xru.ApplyPatch(content, a.Op, a.Value)
		}
	}

	return os.WriteFile(fullPath, []byte(content), 0644)
}
