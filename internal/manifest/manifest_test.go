package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets.yaml", `
project: myproject
keys:
  - FOUNDRY_PROFILE
  - RPC_URL_MAINNET
  - PRIVATE_KEY
map: .secrets-map.yaml
`)

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Project != "myproject" {
		t.Fatalf("Project = %q, want %q", m.Project, "myproject")
	}
	if len(m.Keys) != 3 {
		t.Fatalf("Keys len = %d, want 3", len(m.Keys))
	}
	if m.MapFile != ".secrets-map.yaml" {
		t.Fatalf("MapFile = %q, want .secrets-map.yaml", m.MapFile)
	}
}

func TestLoad_NoProject(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets.yaml", `
keys:
  - FOO
  - BAR
`)

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Project != "" {
		t.Fatalf("Project should be empty, got %q", m.Project)
	}
	if len(m.Keys) != 2 {
		t.Fatalf("Keys len = %d, want 2", len(m.Keys))
	}
}

func TestLoad_DefaultMapFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets.yaml", `
keys:
  - FOO
`)

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.MapFile != ".secrets-map.yaml" {
		t.Fatalf("MapFile = %q, want .secrets-map.yaml", m.MapFile)
	}
}

func TestLoad_CustomMapFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets.yaml", `
keys:
  - FOO
map: custom-map.yaml
`)

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.MapFile != "custom-map.yaml" {
		t.Fatalf("MapFile = %q, want custom-map.yaml", m.MapFile)
	}
}

func TestLoad_DuplicateKeys(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets.yaml", `
keys:
  - FOO
  - BAR
  - FOO
  - BAR
  - BAZ
`)

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Keys) != 3 {
		t.Fatalf("Keys len = %d, want 3 (deduped)", len(m.Keys))
	}
	expected := []string{"FOO", "BAR", "BAZ"}
	for i, k := range m.Keys {
		if k != expected[i] {
			t.Fatalf("Keys[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

func TestLoad_EmptyKeys(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets.yaml", `
project: test
keys: []
`)

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Keys) != 0 {
		t.Fatalf("Keys len = %d, want 0", len(m.Keys))
	}
}

func TestLoad_NotFound(t *testing.T) {
	_, err := Load("/nonexistent/.secrets.yaml")
	if err == nil {
		t.Fatal("Load nonexistent should fail")
	}
	if !strings.Contains(err.Error(), "no .secrets.yaml") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets.yaml", `
keys: [[[invalid
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("invalid YAML should fail")
	}
}

func TestLoad_UnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets.yaml", `
project: test
keys:
  - FOO
future_field: something
another: 42
`)

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load with unknown fields: %v", err)
	}
	if len(m.Keys) != 1 {
		t.Fatalf("Keys len = %d, want 1", len(m.Keys))
	}
}

func TestLoadMap_Valid(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets-map.yaml", `
PRIVATE_KEY: PRIVATE_KEY_alice
RPC_URL_MAINNET: RPC_URL_quicknode
`)

	m, err := LoadMap(path)
	if err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	if m["PRIVATE_KEY"] != "PRIVATE_KEY_alice" {
		t.Fatalf("PRIVATE_KEY = %q", m["PRIVATE_KEY"])
	}
	if m["RPC_URL_MAINNET"] != "RPC_URL_quicknode" {
		t.Fatalf("RPC_URL_MAINNET = %q", m["RPC_URL_MAINNET"])
	}
}

func TestLoadMap_NotFound(t *testing.T) {
	m, err := LoadMap("/nonexistent/.secrets-map.yaml")
	if err != nil {
		t.Fatalf("LoadMap nonexistent should not error: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("LoadMap nonexistent should return empty map, got %v", m)
	}
}

func TestLoadMap_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".secrets-map.yaml", `[[[invalid`)

	_, err := LoadMap(path)
	if err == nil {
		t.Fatal("invalid YAML should fail")
	}
}

func TestResolve_NoMapFile(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeFile(t, dir, ".secrets.yaml", `
keys:
  - FOO
  - BAR
`)

	vars, err := Resolve(manifestPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(vars) != 2 {
		t.Fatalf("vars len = %d, want 2", len(vars))
	}
	// Without map file, store key == env name
	for _, v := range vars {
		if v.EnvName != v.StoreKey {
			t.Fatalf("without map, EnvName %q should equal StoreKey %q", v.EnvName, v.StoreKey)
		}
	}
}

func TestResolve_WithMapFile(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeFile(t, dir, ".secrets.yaml", `
keys:
  - FOO
  - BAR
  - BAZ
`)
	writeFile(t, dir, ".secrets-map.yaml", `
FOO: FOO_remapped
BAZ: BAZ_personal
`)

	vars, err := Resolve(manifestPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(vars) != 3 {
		t.Fatalf("vars len = %d, want 3", len(vars))
	}

	expected := map[string]string{
		"FOO": "FOO_remapped",
		"BAR": "BAR",         // not in map → identity
		"BAZ": "BAZ_personal",
	}
	for _, v := range vars {
		want := expected[v.EnvName]
		if v.StoreKey != want {
			t.Fatalf("StoreKey for %q = %q, want %q", v.EnvName, v.StoreKey, want)
		}
	}
}

func TestResolve_ExtraMapKeysIgnored(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeFile(t, dir, ".secrets.yaml", `
keys:
  - FOO
`)
	writeFile(t, dir, ".secrets-map.yaml", `
FOO: FOO_mapped
EXTRA_KEY: not_in_manifest
ANOTHER: also_not_in_manifest
`)

	vars, err := Resolve(manifestPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(vars) != 1 {
		t.Fatalf("vars len = %d, want 1 (extra keys should be ignored)", len(vars))
	}
	if vars[0].StoreKey != "FOO_mapped" {
		t.Fatalf("StoreKey = %q, want FOO_mapped", vars[0].StoreKey)
	}
}

func TestResolve_CustomMapFileName(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeFile(t, dir, ".secrets.yaml", `
keys:
  - FOO
map: my-custom-map.yaml
`)
	writeFile(t, dir, "my-custom-map.yaml", `
FOO: FOO_custom
`)

	vars, err := Resolve(manifestPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if vars[0].StoreKey != "FOO_custom" {
		t.Fatalf("StoreKey = %q, want FOO_custom", vars[0].StoreKey)
	}
}

func TestResolve_PreservesOrder(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeFile(t, dir, ".secrets.yaml", `
keys:
  - CHARLIE
  - ALPHA
  - BRAVO
`)

	vars, err := Resolve(manifestPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	expected := []string{"CHARLIE", "ALPHA", "BRAVO"}
	for i, v := range vars {
		if v.EnvName != expected[i] {
			t.Fatalf("vars[%d].EnvName = %q, want %q", i, v.EnvName, expected[i])
		}
	}
}
