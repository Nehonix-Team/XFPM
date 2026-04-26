package plugin

import (
	"strings"
)

func ExtractIdentity(data []byte) string {
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "Identity: ") {
			return strings.TrimPrefix(l, "Identity: ")
		}
	}
	return ""
}

func ExtractPrivileges(data []byte) string {
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "Privileges: ") {
			return strings.TrimPrefix(l, "Privileges: ")
		}
	}
	return ""
}
