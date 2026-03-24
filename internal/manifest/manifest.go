// Package manifest parses .secrets.yaml and .secrets.local.yaml
// and resolves variable names to store keys.
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest represents a parsed .secrets.yaml file (committed, team-owned).
type Manifest struct {
	Keys     []string                     `yaml:"keys"`
	Mappings map[string]string            `yaml:"mappings"`
	Profiles map[string]map[string]string `yaml:"profiles"`
}

// LocalManifest represents a parsed .secrets.local.yaml file (personal, git-ignored).
type LocalManifest struct {
	Mappings map[string]string            `yaml:"mappings"`
	Profiles map[string]map[string]string `yaml:"profiles"`
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
			return nil, fmt.Errorf("manifest not found: %s", path)
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	m.Keys = dedup(m.Keys)
	return &m, nil
}

// LoadLocal parses a .secrets.local.yaml file (personal overrides, git-ignored).
// Returns an empty LocalManifest if the file does not exist.
func LoadLocal(path string) (*LocalManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &LocalManifest{}, nil
		}
		return nil, fmt.Errorf("reading local manifest: %w", err)
	}

	var m LocalManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing local manifest: %w", err)
	}
	return &m, nil
}

// Resolve maps each manifest key to a store key.
// localPath is the path to .secrets.local.yaml (may not exist).
// profile is the active profile name (empty string = auto-detect "default" if present).
//
// Resolution priority for each key:
//  1. Active profile, local file override
//  2. Active profile, committed manifest
//  3. Local mappings
//  4. Committed mappings
//  5. Bare key (identity)
func Resolve(manifestPath, localPath, profile string) ([]ResolvedVar, error) {
	m, err := Load(manifestPath)
	if err != nil {
		return nil, err
	}

	local, err := LoadLocal(localPath)
	if err != nil {
		return nil, err
	}

	// Auto-apply "default" profile when no profile is specified and one exists.
	if profile == "" {
		if _, ok := m.Profiles["default"]; ok {
			profile = "default"
		} else if _, ok := local.Profiles["default"]; ok {
			profile = "default"
		}
	}

	vars := make([]ResolvedVar, 0, len(m.Keys))
	for _, key := range m.Keys {
		vars = append(vars, ResolvedVar{
			EnvName:  key,
			StoreKey: resolveKey(key, profile, m, local),
		})
	}
	return vars, nil
}

// resolveKey returns the store key for a given env var name.
func resolveKey(key, profile string, m *Manifest, local *LocalManifest) string {
	if profile != "" {
		// Local profile override takes precedence over committed profile
		if local.Profiles != nil {
			if profMap, ok := local.Profiles[profile]; ok {
				if v, ok := profMap[key]; ok {
					return v
				}
			}
		}
		if m.Profiles != nil {
			if profMap, ok := m.Profiles[profile]; ok {
				if v, ok := profMap[key]; ok {
					return v
				}
			}
		}
	}
	if local.Mappings != nil {
		if v, ok := local.Mappings[key]; ok {
			return v
		}
	}
	if m.Mappings != nil {
		if v, ok := m.Mappings[key]; ok {
			return v
		}
	}
	return key
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
