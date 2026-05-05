/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 *
 * @author Nehonix
 * @license Nehonix OSL (NOSL)
 ***************************************************************************** */

package init

import (
	"fmt"
	"github.com/Nehonix-Team/xru/pkg/engine"
)

// Orchestrator applies XRU rules against a target project directory.
type Orchestrator struct {
	TargetDir string
	Args      map[string]string
}

func NewOrchestrator(targetDir string) *Orchestrator {
	return &Orchestrator{
		TargetDir: targetDir,
		Args:      make(map[string]string),
	}
}

// ApplyRulesFile parses an .xru file and applies every rule in order.
func (o *Orchestrator) ApplyRulesFile(path string) error {
	return o.ApplyRulesFileWithArgs(path, nil)
}

// ApplyRulesFileWithArgs applies rules while injecting specific arguments.
func (o *Orchestrator) ApplyRulesFileWithArgs(path string, args map[string]string) error {
	// Merge args
	for k, v := range args {
		o.Args[k] = v
	}

	runner := engine.NewRunner()
	
	// Convert Args map to XRU's expected --arg NAME=VAL format
	for k, v := range o.Args {
		runner.TerminalArgs = append(runner.TerminalArgs, "--arg", fmt.Sprintf("%s=%s", k, v))
	}

	return runner.Run(path, o.TargetDir)
}
