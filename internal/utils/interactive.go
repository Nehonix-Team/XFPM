package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pterm/pterm"
)

// AskYesNo prompts the user with a yes/no question and returns true if the answer is "y" or "Y".
// This helper reliably reads from standard input across platforms, specifically handling Windows line endings correctly.
func AskYesNo(question string) bool {
	pterm.Printf("   %s %s\n", 
		pterm.FgYellow.Sprint("?"), 
		pterm.Bold.Sprint(question))
	
	fmt.Printf("   %s ", pterm.FgCyan.Sprint(">"))
	
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	return strings.ToLower(input) == "y"
}
