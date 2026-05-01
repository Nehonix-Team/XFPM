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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/utils"
)

type ActionType string

const (
	ActionCreate ActionType = "CREATE"
	ActionBegin  ActionType = "BEGIN"
)

type SubActionType string

const (
	SubActionRM       SubActionType = "RM"
	SubActionRPK      SubActionType = "RPK"
	SubActionRPV      SubActionType = "RPV"
	SubActionMerge    SubActionType = "MERGE"
	SubActionTSInject SubActionType = "TSINJECT"
)

type Rule struct {
	Type       ActionType
	TargetFile string
	Content    string
	SubActions []SubAction
}

type SubAction struct {
	Type SubActionType
	Key  string // For TSINJECT
	Data string // JSON string or Code block
}

// Orchestrator handles the application of XRU rules.
type Orchestrator struct {
	TargetDir string
}

func NewOrchestrator(targetDir string) *Orchestrator {
	return &Orchestrator{TargetDir: targetDir}
}

// ApplyRulesFile reads and applies an .xru file.
func (o *Orchestrator) ApplyRulesFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	rules, err := ParseXRU(string(data))
	if err != nil {
		return fmt.Errorf("parsing rules at %s: %w", path, err)
	}

	for _, rule := range rules {
		if err := o.ApplyRule(rule); err != nil {
			return fmt.Errorf("applying rule for %s: %w", rule.TargetFile, err)
		}
	}

	return nil
}

// ApplyRule applies a single XRU rule.
func (o *Orchestrator) ApplyRule(rule Rule) error {
	fullPath := filepath.Join(o.TargetDir, rule.TargetFile)

	if rule.Type == ActionCreate {
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(fullPath, []byte(rule.Content), 0644)
	}

	if rule.Type == ActionBegin {
		// Read file
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		content := string(data)

		for _, sa := range rule.SubActions {
			switch sa.Type {
			case SubActionTSInject:
				content = injectCode(content, sa.Key, sa.Data)
			case SubActionRM, SubActionRPK, SubActionRPV, SubActionMerge:
				newContent, err := patchJSON(content, sa)
				if err != nil {
					utils.Warn("Failed to patch JSON in %s: %v", rule.TargetFile, err)
					continue
				}
				content = newContent
			}
		}

		return os.WriteFile(fullPath, []byte(content), 0644)
	}

	return nil
}

// ParseXRU parses the .xru file content into Rule structs.
func ParseXRU(content string) ([]Rule, error) {
	var rules []Rule
	lines := strings.Split(content, "\n")

	var currentRule *Rule
	var currentSubAction *SubAction
	var buffer []string
	inBuffer := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#BEGIN:") {
			currentRule = &Rule{Type: ActionBegin, TargetFile: strings.TrimPrefix(trimmed, "#BEGIN:")}
			continue
		}

		if strings.HasPrefix(trimmed, "#CREATE:") {
			currentRule = &Rule{Type: ActionCreate, TargetFile: strings.TrimPrefix(trimmed, "#CREATE:")}
			inBuffer = true
			buffer = []string{}
			continue
		}

		if trimmed == "#END" || (currentRule != nil && strings.HasPrefix(trimmed, "#END:"+currentRule.TargetFile)) {
			if currentRule != nil {
				if currentRule.Type == ActionCreate {
					currentRule.Content = strings.Join(buffer, "\n")
					inBuffer = false
				}
				rules = append(rules, *currentRule)
				currentRule = nil
			}
			continue
		}

		if currentRule == nil {
			continue
		}

		if currentRule.Type == ActionCreate {
			buffer = append(buffer, line)
			continue
		}

		// Handling SubActions inside BEGIN
		if strings.HasPrefix(trimmed, "@TSINJECT:") {
			currentSubAction = &SubAction{Type: SubActionTSInject, Key: strings.TrimSpace(strings.TrimPrefix(trimmed, "@TSINJECT:"))}
			inBuffer = true
			buffer = []string{}
			continue
		}

		if trimmed == "@END" {
			if currentSubAction != nil {
				currentSubAction.Data = strings.Join(buffer, "\n")
				currentRule.SubActions = append(currentRule.SubActions, *currentSubAction)
				currentSubAction = nil
				inBuffer = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "&rm:") {
			currentRule.SubActions = append(currentRule.SubActions, SubAction{Type: SubActionRM, Data: strings.TrimPrefix(trimmed, "&rm:")})
			continue
		}
		if strings.HasPrefix(trimmed, "&rp-k:") || strings.HasPrefix(trimmed, "&rp-0:") {
			currentRule.SubActions = append(currentRule.SubActions, SubAction{Type: SubActionRPK, Data: strings.TrimPrefix(trimmed, "&rp-k:")})
			continue
		}
		if strings.HasPrefix(trimmed, "&rp-v:") || strings.HasPrefix(trimmed, "&rp-1:") {
			currentRule.SubActions = append(currentRule.SubActions, SubAction{Type: SubActionRPV, Data: strings.TrimPrefix(trimmed, "&rp-v:")})
			continue
		}
		if strings.HasPrefix(trimmed, "&merge:") || strings.HasPrefix(trimmed, "&add:") {
			currentRule.SubActions = append(currentRule.SubActions, SubAction{Type: SubActionMerge, Data: strings.TrimPrefix(trimmed, "&merge:")})
			continue
		}

		if inBuffer {
			buffer = append(buffer, line)
		}
	}

	return rules, nil
}

func injectCode(content, key, code string) string {
	marker := fmt.Sprintf(`// xfpm: %s`, key)
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		if strings.Contains(line, marker) {
			result = append(result, code)
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

func patchJSON(content string, sa SubAction) (string, error) {
	// Strip comments for parsing
	stripped := stripJSONComments(content)
	
	var data interface{}
	if err := json.Unmarshal([]byte(stripped), &data); err != nil {
		return "", err
	}

	patchDataRaw := strings.TrimSpace(sa.Data)
	var patchData interface{}
	if err := json.Unmarshal([]byte(patchDataRaw), &patchData); err != nil {
		return "", fmt.Errorf("invalid patch data: %w", err)
	}

	switch sa.Type {
	case SubActionRM:
		data = removeKeys(data, patchData)
	case SubActionRPK:
		data = renameKeys(data, patchData)
	case SubActionRPV:
		data = updateValues(data, patchData)
	case SubActionMerge:
		data = mergeJSON(data, patchData)
	}

	res, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	return string(res), nil
}

func stripJSONComments(content string) string {
	// Remove // comments, including those at the end of a line
	// Note: this is a simple regex and might fail on strings containing //
	re := regexp.MustCompile(`//.*$`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = re.ReplaceAllString(line, "")
	}
	return strings.Join(lines, "\n")
}

func removeKeys(base, patch interface{}) interface{} {
	switch p := patch.(type) {
	case []interface{}:
		// Top level keys removal
		if b, ok := base.(map[string]interface{}); ok {
			for _, k := range p {
				if ks, ok := k.(string); ok {
					delete(b, ks)
				}
			}
		}
	case map[string]interface{}:
		// Nested removal
		if b, ok := base.(map[string]interface{}); ok {
			for k, v := range p {
				if v == "" {
					delete(b, k)
				} else {
					b[k] = removeKeys(b[k], v)
				}
			}
		}
	}
	return base
}

func renameKeys(base, patch interface{}) interface{} {
	p, okP := patch.(map[string]interface{})
	b, okB := base.(map[string]interface{})
	if !okP || !okB {
		return base
	}

	for oldK, newK := range p {
		if newKS, ok := newK.(string); ok {
			if val, exists := b[oldK]; exists {
				b[newKS] = val
				delete(b, oldK)
			}
		}
	}
	return b
}

func updateValues(base, patch interface{}) interface{} {
	p, okP := patch.(map[string]interface{})
	b, okB := base.(map[string]interface{})
	if !okP || !okB {
		return base
	}

	for k, v := range p {
		if _, exists := b[k]; exists {
			b[k] = v
		}
	}
	return b
}

func mergeJSON(base, patch interface{}) interface{} {
	p, okP := patch.(map[string]interface{})
	b, okB := base.(map[string]interface{})
	if !okP || !okB {
		return patch // If not both objects, patch wins
	}

	for k, v := range p {
		if bv, exists := b[k]; exists {
			b[k] = mergeJSON(bv, v)
		} else {
			b[k] = v
		}
	}
	return b
}
