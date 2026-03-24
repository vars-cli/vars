//go:build integration

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var binary string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "secrets-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		os.Exit(1)
	}

	binary = filepath.Join(tmp, "secrets")
	out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "building binary: %v\n%s\n", err, out)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

type runner struct {
	t        *testing.T
	storeDir string
	workDir  string
	env      []string
}

func newRunner(t *testing.T) *runner {
	t.Helper()
	storeDir := t.TempDir()
	workDir := t.TempDir()
	r := &runner{
		t:        t,
		storeDir: storeDir,
		workDir:  workDir,
		env: []string{
			"SECRETS_STORE_DIR=" + storeDir,
			"HOME=" + t.TempDir(),
			"PATH=" + os.Getenv("PATH"),
		},
	}

	// Stop any auto-started agent when the test ends
	t.Cleanup(func() {
		r.run("agent", "stop")
		time.Sleep(50 * time.Millisecond)
	})

	return r
}

func (r *runner) run(args ...string) (string, string, error) {
	r.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = r.workDir
	cmd.Env = r.env
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (r *runner) mustRun(args ...string) string {
	r.t.Helper()
	stdout, stderr, err := r.run(args...)
	if err != nil {
		r.t.Fatalf("secrets %s failed: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout
}

func (r *runner) mustRunWithStderr(args ...string) (string, string) {
	r.t.Helper()
	stdout, stderr, err := r.run(args...)
	if err != nil {
		r.t.Fatalf("secrets %s failed: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout, stderr
}

func (r *runner) mustFail(args ...string) (string, string) {
	r.t.Helper()
	stdout, stderr, err := r.run(args...)
	if err == nil {
		r.t.Fatalf("secrets %s should have failed\nstdout: %s\nstderr: %s", strings.Join(args, " "), stdout, stderr)
	}
	return stdout, stderr
}

func (r *runner) runWithStdin(stdin string, args ...string) (string, string, error) {
	r.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = r.workDir
	cmd.Env = r.env
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (r *runner) mustRunWithStdin(stdin string, args ...string) string {
	r.t.Helper()
	stdout, stderr, err := r.runWithStdin(stdin, args...)
	if err != nil {
		r.t.Fatalf("secrets %s failed: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout
}

func (r *runner) initNoPassphrase() {
	r.t.Helper()
	r.mustRunWithStdin("\n\n", "init")
}

func (r *runner) writeFile(name, content string) {
	r.t.Helper()
	path := filepath.Join(r.workDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		r.t.Fatalf("creating dir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		r.t.Fatalf("writing %s: %v", name, err)
	}
}

// --- Tests ---

func TestIntegration_CRUDLifecycle(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	// set (agent auto-starts on first command)
	r.mustRun("set", "API_KEY", "my-secret-key")
	r.mustRun("set", "DB_URL", "postgres://localhost/db")
	r.mustRun("set", "TOKEN", "abc123")

	// get
	out := r.mustRun("get", "API_KEY")
	if out != "my-secret-key" {
		t.Fatalf("get API_KEY = %q, want %q", out, "my-secret-key")
	}
	if strings.HasSuffix(out, "\n") {
		t.Fatal("get should not have trailing newline")
	}

	// ls
	out = r.mustRun("ls")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("ls returned %d lines, want 3: %v", len(lines), lines)
	}
	if lines[0] != "API_KEY" || lines[1] != "DB_URL" || lines[2] != "TOKEN" {
		t.Fatalf("ls not sorted: %v", lines)
	}

	// rm
	r.mustRun("rm", "TOKEN", "--force")
	out = r.mustRun("ls")
	lines = strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("ls after rm returned %d lines, want 2", len(lines))
	}

	r.mustFail("get", "TOKEN")
	r.mustFail("rm", "NONEXISTENT", "--force")
}

func TestIntegration_SetOverwrite(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY", "first")
	r.mustRun("set", "KEY", "second", "--overwrite")

	out := r.mustRun("get", "KEY")
	if out != "second" {
		t.Fatalf("get after overwrite = %q, want %q", out, "second")
	}
}

func TestIntegration_Set_Idempotent(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY", "value")
	_, stderr := r.mustRunWithStderr("set", "KEY", "value")
	if !strings.Contains(stderr, "Already set") {
		t.Fatalf("expected idempotent message, got: %s", stderr)
	}

	if r.mustRun("get", "KEY") != "value" {
		t.Fatal("value should be unchanged")
	}
}

func TestIntegration_Set_Skip(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY", "original")
	r.mustRun("set", "KEY", "new", "--skip")

	if r.mustRun("get", "KEY") != "original" {
		t.Fatal("--skip should leave existing value unchanged")
	}
}

func TestIntegration_Set_NonTTYConflictFails(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY", "original")
	_, stderr := r.mustFail("set", "KEY", "new")
	if !strings.Contains(stderr, "--overwrite") || !strings.Contains(stderr, "--skip") {
		t.Fatalf("expected flag hint, got: %s", stderr)
	}
}

func TestIntegration_SpecialValues(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	cases := map[string]string{
		"SPACES": "value with spaces",
		"EQUALS": "key=value=extra",
		"URL":    "https://user:pass@host:8080/path?q=1&r=2",
		"HEX":    "0xdeadbeef0123456789abcdef",
	}

	for k, v := range cases {
		r.mustRun("set", k, v)
	}

	for k, want := range cases {
		got := r.mustRun("get", k)
		if got != want {
			t.Errorf("get %s = %q, want %q", k, got, want)
		}
	}
}

func TestIntegration_ResolvePosix(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "RPC_URL", "https://rpc.example.com")
	r.mustRun("set", "PRIVATE_KEY", "0xTESTKEY")

	r.writeFile(".secrets.yaml", `keys:
  - RPC_URL
  - PRIVATE_KEY
`)

	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"))
	if !strings.Contains(out, "export RPC_URL=") {
		t.Fatalf("posix export missing RPC_URL: %s", out)
	}
	if !strings.Contains(out, "export PRIVATE_KEY=") {
		t.Fatalf("posix export missing PRIVATE_KEY: %s", out)
	}
}

func TestIntegration_ResolveFish(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "MY_VAR", "hello world")

	r.writeFile(".secrets.yaml", `keys:
  - MY_VAR
`)

	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--format", "fish")
	if !strings.Contains(out, "set -x MY_VAR") {
		t.Fatalf("fish export missing set -x: %s", out)
	}
}

func TestIntegration_ResolveDotenv(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "MY_VAR", "hello world")

	r.writeFile(".secrets.yaml", `keys:
  - MY_VAR
`)

	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--format", "dotenv")
	if !strings.Contains(out, "MY_VAR=") {
		t.Fatalf("dotenv export missing MY_VAR: %s", out)
	}
}

func TestIntegration_ResolveLocalMappings(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "PRIVATE_KEY", "0xGLOBALKEY")

	r.writeFile(".secrets.yaml", `keys:
  - PROJECT_PK
`)
	r.writeFile(".secrets.local.yaml", `mappings:
  PROJECT_PK: PRIVATE_KEY
`)

	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"))
	if !strings.Contains(out, "export PROJECT_PK=") {
		t.Fatalf("mapped export missing PROJECT_PK: %s", out)
	}
	if !strings.Contains(out, "0xGLOBALKEY") {
		t.Fatalf("mapped export has wrong value: %s", out)
	}
}

func TestIntegration_ResolvePartial(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "EXISTS", "value")

	r.writeFile(".secrets.yaml", `keys:
  - EXISTS
  - MISSING
`)

	r.mustFail("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"))

	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--partial")
	if !strings.Contains(out, "EXISTS") {
		t.Fatalf("partial export missing EXISTS: %s", out)
	}
	if !strings.Contains(out, "MISSING") {
		t.Fatalf("partial export missing MISSING: %s", out)
	}
}

func TestIntegration_DumpAllFormats(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY1", "val1")
	r.mustRun("set", "KEY2", "val2")

	for _, format := range []string{"posix", "fish", "dotenv"} {
		out := r.mustRun("dump", "--format", format)
		if !strings.Contains(out, "KEY1") || !strings.Contains(out, "KEY2") {
			t.Errorf("dump --%s missing keys: %s", format, out)
		}
	}
}

func TestIntegration_AgentAutoStart(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	// Agent is auto-started by init; set uses it
	r.mustRun("set", "AUTO_KEY", "auto_value")

	// Agent should be running
	_, stderr, err := r.run("agent")
	if err != nil {
		t.Fatalf("agent check failed: %v", err)
	}
	if !strings.Contains(stderr, "already running") {
		t.Fatalf("agent should be auto-started, got: %s", stderr)
	}

	// Reads work via agent
	val := r.mustRun("get", "AUTO_KEY")
	if val != "auto_value" {
		t.Fatalf("get = %q, want %q", val, "auto_value")
	}
}

func TestIntegration_AgentExplicitStop(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	// Auto-start via any command
	r.mustRun("ls")

	// Stop
	r.mustRun("agent", "stop")
	time.Sleep(100 * time.Millisecond)

	// Next command should auto-start again
	r.mustRun("set", "KEY", "value")
	val := r.mustRun("get", "KEY")
	if val != "value" {
		t.Fatalf("get after restart = %q, want %q", val, "value")
	}
}

func TestIntegration_Passwd_EmptyToSet(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY", "value")

	// Change from empty to "newpass" — passwd goes through agent.
	// New passphrase prompted, then old passphrase tried (empty → accepted).
	r.mustRunWithStdin("newpass\nnewpass\n", "passwd")

	// Agent is still running with new passphrase. Reads still work.
	val := r.mustRun("get", "KEY")
	if val != "value" {
		t.Fatalf("get after passwd = %q, want %q", val, "value")
	}

	// Stop and restart to verify passphrase changed on disk
	r.mustRun("agent", "stop")
	time.Sleep(100 * time.Millisecond)

	// Now starting agent requires the new passphrase
	out, _, err := r.runWithStdin("newpass\n", "get", "KEY")
	if err != nil {
		t.Fatalf("get with new passphrase failed: %v", err)
	}
	if out != "value" {
		t.Fatalf("get = %q, want %q", out, "value")
	}
}

func TestIntegration_Passwd_SetToEmpty(t *testing.T) {
	r := newRunner(t)

	// Init with passphrase
	r.mustRunWithStdin("mypass\nmypass\n", "init")

	// Auto-start agent (needs passphrase) and set a value
	r.mustRunWithStdin("mypass\n", "set", "KEY", "value")

	// Change to empty passphrase.
	// Input: new passphrase (empty, confirm empty), then old passphrase (mypass).
	r.mustRunWithStdin("\n\nmypass\n", "passwd")

	// Reads still work (agent has the data)
	val := r.mustRun("get", "KEY")
	if val != "value" {
		t.Fatalf("get after passwd = %q, want %q", val, "value")
	}

	// Stop and restart — should not need passphrase now
	r.mustRun("agent", "stop")
	time.Sleep(100 * time.Millisecond)

	out := r.mustRun("get", "KEY")
	if out != "value" {
		t.Fatalf("get after restart = %q, want %q", out, "value")
	}
}

func TestIntegration_InitAlreadyExists(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	_, stderr, err := r.runWithStdin("\n\n", "init")
	if err != nil {
		t.Fatalf("second init should not error: %v", err)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Fatalf("second init should mention already exists: %s", stderr)
	}
}

func TestIntegration_NoStore(t *testing.T) {
	r := newRunner(t)

	_, stderr := r.mustFail("get", "KEY")
	if !strings.Contains(stderr, "No store found") {
		t.Fatalf("expected 'No store found' error, got: %s", stderr)
	}

	_, stderr = r.mustFail("set", "KEY", "value")
	if !strings.Contains(stderr, "No store found") {
		t.Fatalf("expected 'No store found' error, got: %s", stderr)
	}

	_, stderr = r.mustFail("ls")
	if !strings.Contains(stderr, "No store found") {
		t.Fatalf("expected 'No store found' error, got: %s", stderr)
	}
}

func TestIntegration_WrongPassphrase(t *testing.T) {
	r := newRunner(t)

	r.mustRunWithStdin("correctpass\ncorrectpass\n", "init")

	// init auto-starts the agent; stop it to simulate a fresh session
	r.mustRun("agent", "stop")
	time.Sleep(100 * time.Millisecond)

	// Agent auto-start with wrong passphrase should fail
	_, stderr, err := r.runWithStdin("wrongpass\n", "get", "KEY")
	if err == nil {
		t.Fatal("get with wrong passphrase should fail")
	}
	if !strings.Contains(stderr, "Incorrect passphrase") {
		t.Fatalf("expected passphrase error, got: %s", stderr)
	}
}

func TestIntegration_ResolveMissingManifest(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	_, stderr := r.mustFail("resolve", "-f", filepath.Join(r.workDir, "nonexistent.yaml"))
	if !strings.Contains(stderr, "manifest not found") {
		t.Fatalf("expected 'manifest not found' error, got: %s", stderr)
	}
}

func TestIntegration_ResolveInvalidFormat(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.writeFile(".secrets.yaml", `keys:
  - KEY
`)

	_, stderr := r.mustFail("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--format", "invalid")
	if !strings.Contains(stderr, "invalid") || !strings.Contains(strings.ToLower(stderr), "format") {
		t.Fatalf("expected format error, got: %s", stderr)
	}
}

func TestIntegration_Version(t *testing.T) {
	r := newRunner(t)
	out := r.mustRun("--version")
	if !strings.HasPrefix(out, "secrets ") {
		t.Fatalf("version output should start with 'secrets ', got: %q", out)
	}
}

func TestIntegration_ResolveCommittedMappings(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "GLOBAL_TOKEN", "tok123")

	r.writeFile(".secrets.yaml", `keys:
  - LOCAL_TOKEN
mappings:
  LOCAL_TOKEN: GLOBAL_TOKEN
`)

	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"))
	if !strings.Contains(out, "LOCAL_TOKEN") {
		t.Fatalf("committed mappings missing LOCAL_TOKEN: %s", out)
	}
	if !strings.Contains(out, "tok123") {
		t.Fatalf("committed mappings has wrong value: %s", out)
	}
}

func TestIntegration_Mv(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "OLD_KEY", "my-value")

	r.mustRun("mv", "OLD_KEY", "NEW_KEY")

	// Old key gone
	r.mustFail("get", "OLD_KEY")

	// New key has the value
	val := r.mustRun("get", "NEW_KEY")
	if val != "my-value" {
		t.Fatalf("get NEW_KEY = %q, want %q", val, "my-value")
	}
}

func TestIntegration_Mv_MissingKey(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	_, stderr := r.mustFail("mv", "NONEXISTENT", "NEW_KEY")
	if !strings.Contains(stderr, "NONEXISTENT") {
		t.Fatalf("expected error mentioning key name, got: %s", stderr)
	}
}

func TestIntegration_Mv_DestinationExists(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY_A", "a")
	r.mustRun("set", "KEY_B", "b")

	_, stderr := r.mustFail("mv", "KEY_A", "KEY_B")
	if !strings.Contains(stderr, "KEY_B") {
		t.Fatalf("expected error mentioning destination key, got: %s", stderr)
	}

	// Both keys untouched
	if r.mustRun("get", "KEY_A") != "a" {
		t.Fatal("KEY_A should be unchanged")
	}
	if r.mustRun("get", "KEY_B") != "b" {
		t.Fatal("KEY_B should be unchanged")
	}
}

func TestIntegration_Rm_MultiKey(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY_A", "a")
	r.mustRun("set", "KEY_B", "b")
	r.mustRun("set", "KEY_C", "c")

	r.mustRun("rm", "--force", "KEY_A", "KEY_B")

	r.mustFail("get", "KEY_A")
	r.mustFail("get", "KEY_B")

	// KEY_C untouched
	if r.mustRun("get", "KEY_C") != "c" {
		t.Fatal("KEY_C should be unchanged")
	}
}

func TestIntegration_Rm_MultiKey_OneMissing(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY_A", "a")

	_, stderr := r.mustFail("rm", "--force", "KEY_A", "NONEXISTENT")
	if !strings.Contains(stderr, "NONEXISTENT") {
		t.Fatalf("expected error mentioning missing key, got: %s", stderr)
	}

	// KEY_A should be untouched (validation happens before deletion)
	if r.mustRun("get", "KEY_A") != "a" {
		t.Fatal("KEY_A should be unchanged after partial failure")
	}
}

func TestIntegration_Import_Clean(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.writeFile(".env", "RPC_URL=https://rpc.example.com\nPRIVATE_KEY=0xABC\n")
	r.mustRun("import", filepath.Join(r.workDir, ".env"))

	if r.mustRun("get", "RPC_URL") != "https://rpc.example.com" {
		t.Fatal("RPC_URL not imported")
	}
	if r.mustRun("get", "PRIVATE_KEY") != "0xABC" {
		t.Fatal("PRIVATE_KEY not imported")
	}
}

func TestIntegration_Import_Scope(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.writeFile(".env", "RPC_URL=https://rpc.example.com\n")
	r.mustRun("import", "dev", filepath.Join(r.workDir, ".env"))

	r.mustFail("get", "RPC_URL")
	if r.mustRun("get", "dev/RPC_URL") != "https://rpc.example.com" {
		t.Fatal("dev/RPC_URL not imported")
	}
}

func TestIntegration_Ls_Scope(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "GLOBAL_KEY", "g")
	r.mustRun("set", "prod/KEY_A", "a")
	r.mustRun("set", "prod/KEY_B", "b")
	r.mustRun("set", "staging/KEY_A", "sa")

	// ls (default scope) — only GLOBAL_KEY
	out := r.mustRun("ls")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || lines[0] != "GLOBAL_KEY" {
		t.Fatalf("ls: expected [GLOBAL_KEY], got %v", lines)
	}

	// ls prod — KEY_A and KEY_B without prefix
	out = r.mustRun("ls", "prod")
	lines = strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "KEY_A" || lines[1] != "KEY_B" {
		t.Fatalf("ls prod: expected [KEY_A KEY_B], got %v", lines)
	}

	// ls staging — KEY_A without prefix
	out = r.mustRun("ls", "staging")
	lines = strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || lines[0] != "KEY_A" {
		t.Fatalf("ls staging: expected [KEY_A], got %v", lines)
	}
}

func TestIntegration_Resolve_Profile(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "prod/RPC_URL", "https://prod.rpc")
	r.mustRun("set", "ETHERSCAN_API", "shared-key")

	r.writeFile(".secrets.yaml", `keys:
  - RPC_URL
  - ETHERSCAN_API
profiles:
  mainnet:
    RPC_URL: prod/RPC_URL
`)

	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--profile", "mainnet")
	if !strings.Contains(out, "https://prod.rpc") {
		t.Fatalf("expected profile RPC_URL, got: %s", out)
	}
	if !strings.Contains(out, "shared-key") {
		t.Fatalf("expected bare ETHERSCAN_API, got: %s", out)
	}
}

func TestIntegration_Resolve_LocalProfileOverride(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "prod/PRIVATE_KEY_alice", "0xALICE")

	r.writeFile(".secrets.yaml", `keys:
  - PRIVATE_KEY
profiles:
  mainnet:
    PRIVATE_KEY: prod/PRIVATE_KEY_team
`)
	r.writeFile(".secrets.local.yaml", `profiles:
  mainnet:
    PRIVATE_KEY: prod/PRIVATE_KEY_alice
`)

	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--profile", "mainnet")
	if !strings.Contains(out, "0xALICE") {
		t.Fatalf("local profile should override committed: got %s", out)
	}
}

func TestIntegration_Import_SkipConflict(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "RPC_URL", "original")
	r.writeFile(".env", "RPC_URL=new_value\nNEW_KEY=new\n")
	r.mustRun("import", filepath.Join(r.workDir, ".env"), "--skip")

	// Existing key unchanged
	if r.mustRun("get", "RPC_URL") != "original" {
		t.Fatal("RPC_URL should be unchanged with --skip")
	}
	// New key imported
	if r.mustRun("get", "NEW_KEY") != "new" {
		t.Fatal("NEW_KEY should be imported")
	}
}

func TestIntegration_Import_OverwriteConflict(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "RPC_URL", "original")
	r.writeFile(".env", "RPC_URL=updated\n")
	r.mustRun("import", filepath.Join(r.workDir, ".env"), "--overwrite")

	if r.mustRun("get", "RPC_URL") != "updated" {
		t.Fatal("RPC_URL should be overwritten")
	}
}

func TestIntegration_Import_SameValueSkipped(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "RPC_URL", "same_value")
	r.writeFile(".env", "RPC_URL=same_value\n")
	// No --skip or --overwrite needed — same value is not a conflict
	r.mustRun("import", filepath.Join(r.workDir, ".env"))

	if r.mustRun("get", "RPC_URL") != "same_value" {
		t.Fatal("RPC_URL should still have same_value")
	}
}

func TestIntegration_Import_NonTTYConflictFails(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "RPC_URL", "original")
	r.writeFile(".env", "RPC_URL=new_value\n")

	// Non-TTY with conflict and no flag should fail
	_, stderr := r.mustFail("import", filepath.Join(r.workDir, ".env"))
	if !strings.Contains(stderr, "--overwrite") || !strings.Contains(stderr, "--skip") {
		t.Fatalf("expected hint about --overwrite/--skip, got: %s", stderr)
	}
}

func TestIntegration_DumpEmpty(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	out := r.mustRun("dump")
	if strings.TrimSpace(out) != "" {
		t.Fatalf("dump of empty store should be empty, got: %q", out)
	}
}

func TestIntegration_Scopes(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "BARE_KEY", "bare")
	r.mustRun("set", "prod/RPC_URL", "p1")
	r.mustRun("set", "prod/PRIVATE_KEY", "p2")
	r.mustRun("set", "staging/RPC_URL", "s1")

	out := r.mustRun("scope", "ls")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("scopes returned %d lines, want 2: %v", len(lines), lines)
	}
	if lines[0] != "prod" || lines[1] != "staging" {
		t.Fatalf("scopes = %v, want [prod staging]", lines)
	}
}

func TestIntegration_Scopes_Hierarchical(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "main/dev/RPC_URL", "v1")
	r.mustRun("set", "main/RPC_URL", "v2")
	r.mustRun("set", "prod/KEY", "v3")

	out := r.mustRun("scope", "ls")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// expect: main, main/dev, prod — sorted
	want := []string{"main", "main/dev", "prod"}
	if len(lines) != len(want) {
		t.Fatalf("scopes = %v, want %v", lines, want)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Fatalf("scopes[%d] = %q, want %q", i, lines[i], w)
		}
	}
}

func TestIntegration_Scopes_Empty(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "BARE_KEY", "bare")

	out := r.mustRun("scope", "ls")
	if strings.TrimSpace(out) != "" {
		t.Fatalf("scopes with no scoped keys should be empty, got: %q", out)
	}
}

func TestIntegration_ResolveOrderPreserved(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "ZEBRA", "z")
	r.mustRun("set", "ALPHA", "a")
	r.mustRun("set", "MIKE", "m")

	r.writeFile(".secrets.yaml", `keys:
  - MIKE
  - ZEBRA
  - ALPHA
`)

	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"))
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("export returned %d lines, want 3", len(lines))
	}
	if !strings.Contains(lines[0], "MIKE") {
		t.Fatalf("first line should contain MIKE, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "ZEBRA") {
		t.Fatalf("second line should contain ZEBRA, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "ALPHA") {
		t.Fatalf("third line should contain ALPHA, got: %s", lines[2])
	}
}

func TestIntegration_Resolve_DefaultProfile(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "prod/RPC_URL", "https://prod.rpc")
	r.mustRun("set", "ETHERSCAN_API", "shared-key")

	r.writeFile(".secrets.yaml", `keys:
  - RPC_URL
  - ETHERSCAN_API
profiles:
  default:
    RPC_URL: prod/RPC_URL
`)

	// No --profile flag: "default" profile is auto-applied
	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"))
	if !strings.Contains(out, "https://prod.rpc") {
		t.Fatalf("default profile should be auto-applied: got %s", out)
	}
	if !strings.Contains(out, "shared-key") {
		t.Fatalf("bare key fallback should still work: got %s", out)
	}
}

func TestIntegration_Resolve_HierarchicalFallback(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	// Only the base key exists; manifest maps to a more-specific key
	r.mustRun("set", "RPC_URL", "https://base.rpc")

	r.writeFile(".secrets.yaml", `keys:
  - RPC_URL
profiles:
  mainnet:
    RPC_URL: main/dev/RPC_URL
`)

	// main/dev/RPC_URL not in store → main/RPC_URL not in store → RPC_URL found
	out := r.mustRun("resolve", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--profile", "mainnet")
	if !strings.Contains(out, "https://base.rpc") {
		t.Fatalf("hierarchical fallback should resolve to base key: got %s", out)
	}
}

func TestIntegration_Ls_All(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "BARE_KEY", "b")
	r.mustRun("set", "prod/RPC_URL", "p")
	r.mustRun("set", "staging/API_KEY", "s")

	out := r.mustRun("ls", "--all")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("ls --all should list 3 keys, got: %v", lines)
	}

	// Default ls still only shows unscoped keys
	out = r.mustRun("ls")
	lines = strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || lines[0] != "BARE_KEY" {
		t.Fatalf("ls should only show unscoped keys, got: %v", lines)
	}
}
