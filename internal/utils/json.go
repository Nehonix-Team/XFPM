package utils

import (
	"errors"
	"os"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// findJsonRoot locates the start and end offsets of the top-level JSON object or array in a JSONC string,
// ignoring comments and leading/trailing whitespace.
func findJsonRoot(content string) (int, int, error) {
	n := len(content)
	start := -1

	// 1. Find the first '{' or '[' outside comments
	i := 0
	for i < n {
		if content[i] == '/' && i+1 < n {
			if content[i+1] == '/' {
				// Line comment: skip to newline
				i += 2
				for i < n && content[i] != '\n' {
					i++
				}
				continue
			} else if content[i+1] == '*' {
				// Block comment: skip to */
				i += 2
				for i+1 < n && !(content[i] == '*' && content[i+1] == '/') {
					i++
				}
				i += 2
				continue
			}
		}
		if content[i] == '{' || content[i] == '[' {
			start = i
			break
		}
		i++
	}

	if start == -1 {
		return 0, 0, errors.New("no JSON root object or array found")
	}

	// 2. Scan to find the matching closing bracket
	depth := 0
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false
	end := -1

	for i := start; i < n; i++ {
		ch := content[i]

		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < n && content[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if ch == '/' && i+1 < n {
			if content[i+1] == '/' {
				inLineComment = true
				i++
				continue
			} else if content[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		if ch == '"' {
			inString = true
			continue
		}

		if ch == '{' || ch == '[' {
			depth++
		} else if ch == '}' || ch == ']' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}

	if end == -1 || depth != 0 {
		return 0, 0, errors.New("unbalanced JSON brackets")
	}

	return start, end, nil
}

// UpdateJsonFile lit un fichier JSON/JSONC, met à jour les champs donnés et le sauvegarde,
// tout en préservant le formatage original et les commentaires hors des champs modifiés.
// Si le fichier n'existe pas, il crée un fichier avec "{}" et applique les mises à jour.
func UpdateJsonFile(path string, updates map[string]interface{}) error {
	var content string
	
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			content = "{}\n"
		} else {
			return err
		}
	} else {
		content = string(bytes)
		if len(strings.TrimSpace(content)) == 0 {
			content = "{}\n"
		}
	}

	start, end, err := findJsonRoot(content)
	var prefix, jsonBody, suffix string
	if err == nil {
		prefix = content[:start]
		jsonBody = content[start:end]
		suffix = content[end:]
	} else {
		jsonBody = "{}"
	}

	for key, value := range updates {
		var setErr error
		jsonBody, setErr = sjson.Set(jsonBody, key, value)
		if setErr != nil {
			return setErr
		}
	}

	return os.WriteFile(path, []byte(prefix+jsonBody+suffix), 0644)
}

// RemoveFromJsonFile supprime les clés spécifiées d'un fichier JSON/JSONC sans altérer
// le reste de la structure ou les commentaires.
func RemoveFromJsonFile(path string, keys []string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err // On ne crée pas le fichier si on demande de supprimer qqch
	}
	
	content := string(bytes)
	start, end, err := findJsonRoot(content)
	var prefix, jsonBody, suffix string
	if err == nil {
		prefix = content[:start]
		jsonBody = content[start:end]
		suffix = content[end:]
	} else {
		jsonBody = content
	}

	for _, key := range keys {
		var delErr error
		jsonBody, delErr = sjson.Delete(jsonBody, key)
		if delErr != nil {
			return delErr
		}
	}

	return os.WriteFile(path, []byte(prefix+jsonBody+suffix), 0644)
}

// GetJsonValue lit une valeur depuis un fichier JSON/JSONC en utilisant le chemin (path) spécifié.
func GetJsonValue(path string, key string) (gjson.Result, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return gjson.Result{}, err
	}
	
	content := string(bytes)
	start, end, err := findJsonRoot(content)
	if err == nil {
		content = content[start:end]
	}

	result := gjson.Get(content, key)
	if !result.Exists() {
		return gjson.Result{}, errors.New("key not found")
	}
	
	return result, nil
}
