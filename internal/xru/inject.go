/***************************************************************************
 * XFPM — TypeScript Code Injection
 *
 * Replaces `// xfpm: {{KEY}}` markers with the provided code block.
 * The marker detection is whitespace-tolerant and handles both:
 *   - `// xfpm: {{KEY}}`  (with braces)
 *   - `// xfpm: KEY`      (without braces)
 ***************************************************************************** */

package xru

import (
	"fmt"
	"regexp"
	"strings"
)

// InjectCode replaces every line containing `// xfpm: <key>` with code.
// The match is whitespace-agnostic and works whether key has `{{}}` braces or not.
func InjectCode(content, key, code string) string {
	escaped := regexp.QuoteMeta(key)
	pattern := fmt.Sprintf(`(?m)^.*//\s*xfpm:\s*%s.*$`, escaped)
	re := regexp.MustCompile(pattern)

	// If the key has braces and we didn't match, try the bare inner name
	if !re.MatchString(content) && strings.HasPrefix(key, "{{") && strings.HasSuffix(key, "}}") {
		inner := key[2 : len(key)-2]
		pattern = fmt.Sprintf(`(?m)^.*//\s*xfpm:\s*%s.*$`, regexp.QuoteMeta(inner))
		re = regexp.MustCompile(pattern)
	}

	return re.ReplaceAllString(content, code)
}
