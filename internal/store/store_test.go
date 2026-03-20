package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brickpop/secrets/internal/crypto/age"
)

// helper: create a temp dir and set SECRETS_STORE_DIR
func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SECRETS_STORE_DIR", dir)
	return dir
}

// helper: init a store and return the backend
func initTestStore(t *testing.T, passphrase string) *age.ScryptBackend {
	t.Helper()
	backend := age.New(passphrase)
	if err := Init(backend); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return backend
}

func TestDir_Default(t *testing.T) {
	t.Setenv("SECRETS_STORE_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	d := Dir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "share", "secrets")
	if d != expected {
		t.Fatalf("Dir() = %q, want %q", d, expected)
	}
}

func TestDir_XDG(t *testing.T) {
	t.Setenv("SECRETS_STORE_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	expected := filepath.Join("/tmp/xdg-data", "secrets")
	if d := Dir(); d != expected {
		t.Fatalf("Dir() = %q, want %q", d, expected)
	}
}

func TestDir_Override(t *testing.T) {
	t.Setenv("SECRETS_STORE_DIR", "/tmp/custom-secrets")
	if d := Dir(); d != "/tmp/custom-secrets" {
		t.Fatalf("Dir() = %q, want /tmp/custom-secrets", d)
	}
}

func TestInit_CreatesDirectoryAndFile(t *testing.T) {
	dir := setupTestDir(t)
	initTestStore(t, "test-passphrase")

	// Directory exists
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("store dir is not a directory")
	}
	if perm := info.Mode().Perm(); perm != dirPerm {
		t.Fatalf("dir perm = %04o, want %04o", perm, dirPerm)
	}

	// File exists
	storePath := filepath.Join(dir, storeFileName)
	info, err = os.Stat(storePath)
	if err != nil {
		t.Fatalf("stat store: %v", err)
	}
	if perm := info.Mode().Perm(); perm != filePerm {
		t.Fatalf("file perm = %04o, want %04o", perm, filePerm)
	}
}

func TestInit_Idempotent(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	// Second init should fail
	err := Init(backend)
	if err == nil {
		t.Fatal("second Init should fail")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExists(t *testing.T) {
	setupTestDir(t)

	if Exists() {
		t.Fatal("Exists() should be false before init")
	}

	initTestStore(t, "test-passphrase")

	if !Exists() {
		t.Fatal("Exists() should be true after init")
	}
}

func TestSetAndGet(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Set("MY_KEY", []byte("my-value"))
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reopen and verify
	s2, err := Open(backend)
	if err != nil {
		t.Fatalf("Open after save: %v", err)
	}
	defer s2.Close()

	val, err := s2.Get("MY_KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "my-value" {
		t.Fatalf("Get = %q, want %q", val, "my-value")
	}
}

func TestGet_NotFound(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	_, err = s.Get("NONEXISTENT")
	if err == nil {
		t.Fatal("Get nonexistent key should fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSet_Overwrite(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Set("KEY", []byte("first"))
	s.Set("KEY", []byte("second"))

	val, err := s.Get("KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "second" {
		t.Fatalf("Get = %q, want %q", val, "second")
	}
}

func TestDelete(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Set("KEY", []byte("value"))
	if err := s.Delete("KEY"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = s.Get("KEY")
	if err == nil {
		t.Fatal("Get after Delete should fail")
	}
}

func TestDelete_NotFound(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	err = s.Delete("NONEXISTENT")
	if err == nil {
		t.Fatal("Delete nonexistent should fail")
	}
}

func TestList_Empty(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	keys := s.List()
	if len(keys) != 0 {
		t.Fatalf("List empty store = %v, want empty", keys)
	}
}

func TestList_Sorted(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Set in non-alphabetical order
	s.Set("ZEBRA", []byte("z"))
	s.Set("ALPHA", []byte("a"))
	s.Set("MIKE", []byte("m"))

	keys := s.List()
	expected := []string{"ALPHA", "MIKE", "ZEBRA"}
	if len(keys) != len(expected) {
		t.Fatalf("List len = %d, want %d", len(keys), len(expected))
	}
	for i, k := range keys {
		if k != expected[i] {
			t.Fatalf("List[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

func TestOpen_NoStore(t *testing.T) {
	setupTestDir(t)
	backend := age.New("test-passphrase")

	_, err := Open(backend)
	if err == nil {
		t.Fatal("Open without init should fail")
	}
	if !strings.Contains(err.Error(), "no store found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpen_WrongPassphrase(t *testing.T) {
	setupTestDir(t)
	initTestStore(t, "correct-passphrase")

	wrong := age.New("wrong-passphrase")
	_, err := Open(wrong)
	if err == nil {
		t.Fatal("Open with wrong passphrase should fail")
	}
}

func TestAtomicWrite_OriginalSurvives(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	// Set a value and save
	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Set("ORIGINAL", []byte("data"))
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s.Close()

	// Verify original is readable
	s2, err := Open(backend)
	if err != nil {
		t.Fatalf("Open after save: %v", err)
	}
	val, err := s2.Get("ORIGINAL")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "data" {
		t.Fatalf("Get = %q, want %q", val, "data")
	}
	s2.Close()
}

func TestEmptyPassphrase(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open with empty passphrase: %v", err)
	}
	defer s.Close()

	s.Set("KEY", []byte("value"))
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reopen
	s2, err := Open(backend)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer s2.Close()

	val, err := s2.Get("KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "value" {
		t.Fatalf("Get = %q, want %q", val, "value")
	}
}

func TestSaveWithBackend(t *testing.T) {
	setupTestDir(t)
	oldBackend := initTestStore(t, "old-passphrase")

	s, err := Open(oldBackend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Set("KEY", []byte("value"))

	// Re-encrypt with new passphrase
	newBackend := age.New("new-passphrase")
	if err := s.SaveWithBackend(newBackend); err != nil {
		t.Fatalf("SaveWithBackend: %v", err)
	}
	s.Close()

	// Old passphrase should fail
	_, err = Open(oldBackend)
	if err == nil {
		t.Fatal("Open with old passphrase should fail after re-encrypt")
	}

	// New passphrase should work
	s2, err := Open(newBackend)
	if err != nil {
		t.Fatalf("Open with new passphrase: %v", err)
	}
	defer s2.Close()

	val, err := s2.Get("KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "value" {
		t.Fatalf("Get = %q, want %q", val, "value")
	}
}

func TestClose_ZerosMemory(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	s.Set("SECRET", []byte("sensitive"))

	// Get a reference to the internal slice before close
	val := s.data["SECRET"]

	s.Close()

	// After close, the internal slice should be zeroed
	for _, b := range val {
		if b != 0 {
			t.Fatal("Close did not zero secret memory")
		}
	}

	// Data map should be empty
	if len(s.data) != 0 {
		t.Fatal("Close did not clear data map")
	}
}

func TestGet_ReturnsCopy(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Set("KEY", []byte("original"))

	// Get returns a copy — mutating it should not affect the store
	val, _ := s.Get("KEY")
	val[0] = 'X'

	val2, _ := s.Get("KEY")
	if string(val2) != "original" {
		t.Fatalf("Get did not return a copy; internal data mutated to %q", val2)
	}
}

func TestSet_StoresCopy(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	input := []byte("original")
	s.Set("KEY", input)

	// Mutating the input should not affect the store
	input[0] = 'X'

	val, _ := s.Get("KEY")
	if string(val) != "original" {
		t.Fatalf("Set did not copy input; internal data is %q", val)
	}
}

func TestCheckPermissions_TooPermissive(t *testing.T) {
	dir := setupTestDir(t)
	initTestStore(t, "test-passphrase")

	// Make dir too permissive
	os.Chmod(dir, 0755)

	warnings := CheckPermissions()
	if len(warnings) == 0 {
		t.Fatal("expected permission warning")
	}
	if !strings.Contains(warnings[0], "0755") {
		t.Fatalf("warning should mention 0755, got: %s", warnings[0])
	}
	if !strings.Contains(warnings[0], "chmod") {
		t.Fatalf("warning should include chmod fix, got: %s", warnings[0])
	}
}

func TestCheckPermissions_Correct(t *testing.T) {
	dir := setupTestDir(t)
	initTestStore(t, "test-passphrase")

	os.Chmod(dir, dirPerm)
	os.Chmod(filepath.Join(dir, storeFileName), filePerm)

	warnings := CheckPermissions()
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestMultipleKeys(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Set many keys
	keys := map[string]string{
		"PRIVATE_KEY_myproject":    "0xTESTKEY1",
		"PRIVATE_KEY_otherproject": "0xTESTKEY2",
		"RPC_URL_MAINNET":         "https://rpc.example.com",
		"RPC_URL_GOERLI":          "https://goerli.example.com",
		"ETHERSCAN_API":           "abc123def456",
		"FOUNDRY_PROFILE":         "default",
	}

	for k, v := range keys {
		s.Set(k, []byte(v))
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s.Close()

	// Reopen and verify all
	s2, err := Open(backend)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer s2.Close()

	for k, want := range keys {
		got, err := s2.Get(k)
		if err != nil {
			t.Fatalf("Get(%q): %v", k, err)
		}
		if string(got) != want {
			t.Fatalf("Get(%q) = %q, want %q", k, got, want)
		}
	}

	// List should have all keys sorted
	listed := s2.List()
	if len(listed) != len(keys) {
		t.Fatalf("List len = %d, want %d", len(listed), len(keys))
	}
}

func TestSpecialValueCharacters(t *testing.T) {
	setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	testCases := map[string]string{
		"SPACES":     "value with spaces",
		"QUOTES":     `value with "double" and 'single' quotes`,
		"NEWLINES":   "line1\nline2\nline3",
		"TABS":       "col1\tcol2\tcol3",
		"BACKSLASH":  `path\to\something`,
		"DOLLAR":     "$HOME/bin",
		"BACKTICK":   "`command`",
		"UNICODE":    "\u00e9\u00e8\u00ea\u00eb \u00fc\u00f6\u00e4",
		"SPECIAL":    "!@#$%^&*(){}[]|;:<>,?/~",
		"EMPTY":      "",
		"EQUALS":     "key=value=extra",
		"URL":        "https://user:pass@host:8080/path?q=1&r=2#frag",
		"PRIVATE_HEX": "0xdeadbeef0123456789abcdef",
	}

	for k, v := range testCases {
		s.Set(k, []byte(v))
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s.Close()

	// Reopen and verify
	s2, err := Open(backend)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer s2.Close()

	for k, want := range testCases {
		got, err := s2.Get(k)
		if err != nil {
			t.Fatalf("Get(%q): %v", k, err)
		}
		if string(got) != want {
			t.Fatalf("Get(%q) = %q, want %q", k, got, want)
		}
	}
}

func TestNoTempFilesLeftBehind(t *testing.T) {
	dir := setupTestDir(t)
	backend := initTestStore(t, "test-passphrase")

	s, err := Open(backend)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Set("KEY", []byte("value"))
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s.Close()

	// Check no temp files remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestFilePath(t *testing.T) {
	t.Setenv("SECRETS_STORE_DIR", "/tmp/test-secrets")
	if fp := FilePath(); fp != "/tmp/test-secrets/store.age" {
		t.Fatalf("FilePath() = %q, want /tmp/test-secrets/store.age", fp)
	}
}
