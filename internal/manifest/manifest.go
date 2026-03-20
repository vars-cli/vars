// Package manifest parses .secrets.yaml and .secrets-map.yaml files
// and resolves variable names to store keys.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Manifest represents a parsed .secrets.yaml file.
type Manifest struct {
	Project string   `yaml:"project"`
	Keys    []string `yaml:"keys"`
	MapFile string   `yaml:"map"`
}

// ResolvedVar is a variable name mapped to its store key.
type ResolvedVar struct {
	EnvName  string // the env var name to export
	StoreKey string // the key to look up in the store
}

// Load parses a .secrets.yaml file.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no .secrets.yaml found in the current directory. Use -f to specify a file path")
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// Deduplicate keys while preserving order
	m.Keys = dedup(m.Keys)

	// Default map file
	if m.MapFile == "" {
		m.MapFile = ".secrets-map.yaml"
	}

	return &m, nil
}

// LoadMap parses a .secrets-map.yaml file (simple key→value YAML dict).
// Returns an empty map if the file does not exist.
func LoadMap(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("reading map file: %w", err)
	}

	m := make(map[string]string)
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing map file: %w", err)
	}
	return m, nil
}

// Resolve maps each manifest key to a store key using the map file.
// The map file is looked for alongside the manifest.
func Resolve(manifestPath string) ([]ResolvedVar, error) {
	m, err := Load(manifestPath)
	if err != nil {
		return nil, err
	}

	mapPath := filepath.Join(filepath.Dir(manifestPath), m.MapFile)
	mapping, err := LoadMap(mapPath)
	if err != nil {
		return nil, err
	}

	vars := make([]ResolvedVar, 0, len(m.Keys))
	for _, key := range m.Keys {
		storeKey := key
		if mapped, ok := mapping[key]; ok {
			storeKey = mapped
		}
		vars = append(vars, ResolvedVar{
			EnvName:  key,
			StoreKey: storeKey,
		})
	}
	return vars, nil
}

// dedup removes duplicate strings while preserving order.
func dedup(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
