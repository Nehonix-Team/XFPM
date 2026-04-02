package core

import (
	"encoding/json"
	"fmt"
	"os"
)

type PackageJson struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Description          string            `json:"description,omitempty"`
	Author               any               `json:"author,omitempty"`
	License              string            `json:"license,omitempty"`
	Private              *bool             `json:"private,omitempty"`
	Type                 string            `json:"type,omitempty"`
	Main                 string            `json:"main,omitempty"`
	Module               string            `json:"module,omitempty"`
	Browser              string            `json:"browser,omitempty"`
	Types                string            `json:"types,omitempty"`
	Typings              string            `json:"typings,omitempty"`
	Exports              any               `json:"exports,omitempty"`
	Files                []string          `json:"files,omitempty"`
	Bin                  any               `json:"bin,omitempty"`
	Engines              map[string]string `json:"engines,omitempty"`
	Repository           any               `json:"repository,omitempty"`
	Keywords             []string          `json:"keywords,omitempty"`
	PackageManager       string            `json:"packageManager,omitempty"`
	Workspaces           []string          `json:"workspaces,omitempty"`
	Resolutions          map[string]string `json:"resolutions,omitempty"`
	Overrides            map[string]string `json:"overrides,omitempty"`
	PublishConfig        any               `json:"publishConfig,omitempty"`
	Scripts              map[string]string `json:"scripts,omitempty"`
	Dependencies         map[string]string `json:"dependencies,omitempty"`
	DevDependencies      map[string]string `json:"devDependencies,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	PeerDependencies     map[string]string `json:"peerDependencies,omitempty"`
}

func LoadPackageJson(path string) (*PackageJson, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pkg PackageJson
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &pkg, nil
}

func (p *PackageJson) AllDependencies() map[string]string {
	all := make(map[string]string)
	for k, v := range p.Dependencies {
		all[k] = v
	}
	for k, v := range p.DevDependencies {
		all[k] = v
	}
	for k, v := range p.OptionalDependencies {
		all[k] = v
	}
	for k, v := range p.PeerDependencies {
		all[k] = v
	}
	return all
}

func (p *PackageJson) Save(path string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
