package utils

import (
	"errors"
	"os"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// UpdateJsonFile lit un fichier JSON/JSONC, met à jour les champs donnés et le sauvegarde,
// tout en préservant le formatage original et les commentaires hors des champs modifiés.
// Si le fichier n'existe pas, il crée un fichier avec "{}" et applique les mises à jour.
func UpdateJsonFile(path string, updates map[string]interface{}) error {
	var content string
	
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			content = "{}"
		} else {
			return err
		}
	} else {
		content = string(bytes)
		// If the file was completely empty, default it to empty JSON object
		if len(content) == 0 {
			content = "{}"
		}
	}

	for key, value := range updates {
		var setErr error
		content, setErr = sjson.Set(content, key, value)
		if setErr != nil {
			return setErr
		}
	}

	return os.WriteFile(path, []byte(content), 0644)
}

// RemoveFromJsonFile supprime les clés spécifiées d'un fichier JSON/JSONC sans altérer
// le reste de la structure ou les commentaires.
func RemoveFromJsonFile(path string, keys []string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err // On ne crée pas le fichier si on demande de supprimer qqch
	}
	
	content := string(bytes)
	for _, key := range keys {
		var delErr error
		content, delErr = sjson.Delete(content, key)
		if delErr != nil {
			return delErr
		}
	}

	return os.WriteFile(path, []byte(content), 0644)
}

// GetJsonValue lit une valeur depuis un fichier JSON/JSONC en utilisant le chemin (path) spécifié.
func GetJsonValue(path string, key string) (gjson.Result, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return gjson.Result{}, err
	}
	
	result := gjson.Get(string(bytes), key)
	if !result.Exists() {
		return gjson.Result{}, errors.New("key not found")
	}
	
	return result, nil
}
