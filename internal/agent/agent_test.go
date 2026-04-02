package agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vars-cli/vars/internal/crypto"
	"github.com/vars-cli/vars/internal/store"
)

// fastBackend is a trivial no-op crypto backend for tests.
// It avoids scrypt overhead so agent tests run quickly under the race detector.
// Passphrase logic in the server uses string comparison, not the backend,
// so this does not reduce test coverage.
type fastBackend struct{}

func (fastBackend) Encrypt(plaintext []byte) ([]byte, error) {
	out := make([]byte, len(plaintext)+1)
	out[0] = 0x01
	copy(out[1:], plaintext)
	return out, nil
}

func (fastBackend) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 || ciphertext[0] != 0x01 {
		return nil, fmt.Errorf("fastBackend: decrypt failed")
	}
	return ciphertext[1:], nil
}

func newFastBackend(_ string) crypto.Backend { return fastBackend{} }

func startTestServer(t *testing.T, data map[string]string, passphrase string, ttl time.Duration) (string, *Server) {
	t.Helper()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "agent.sock")

	// Create the store dir and init a store so SaveData works
	storeDir := t.TempDir()
	t.Setenv("VARS_STORE_DIR", storeDir)
	backend := fastBackend{}

	// Init the store so the directory/file exist
	if err := store.Init(backend); err != nil {
		t.Fatalf("store init: %v", err)
	}

	srv := NewServer(data, sockPath, passphrase, backend, newFastBackend, storeDir)

	go func() {
		srv.Start(ttl)
	}()

	select {
	case <-srv.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start in time")
	}

	return sockPath, srv
}

// set is a test helper for single-key Set.
func set(sockPath, key, value string) error {
	return Set(sockPath, []SetItem{{Key: key, Value: value}})
}

// del is a test helper for single-key Delete.
func del(sockPath, key string) error {
	return Delete(sockPath, []string{key})
}

// --- Read tests ---

func TestGetExisting(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY_A": "value_a",
		"KEY_B": "value_b",
	}, "", 0)
	defer srv.Stop()

	val, err := Get(sockPath, "KEY_A")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "value_a" {
		t.Fatalf("Get = %q, want %q", val, "value_a")
	}
}

func TestGetMissing(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY_A": "value_a",
	}, "", 0)
	defer srv.Stop()

	_, err := Get(sockPath, "NONEXISTENT")
	if err == nil {
		t.Fatal("Get missing key should fail")
	}
}

func TestList(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"ZEBRA": "z",
		"ALPHA": "a",
		"MIKE":  "m",
	}, "", 0)
	defer srv.Stop()

	keys, err := List(sockPath)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
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

func TestListEmpty(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, "", 0)
	defer srv.Stop()

	keys, err := List(sockPath)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("List empty = %v, want empty", keys)
	}
}

// --- Lifecycle tests ---

func TestStop(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "", 0)
	_ = srv

	err := Stop(sockPath)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if IsRunning(sockPath) {
		t.Fatal("agent should not be running after Stop")
	}
}

func TestTTLExpiry(t *testing.T) {
	sockPath, _ := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "", 200*time.Millisecond)

	if !IsRunning(sockPath) {
		t.Fatal("agent should be running")
	}

	time.Sleep(400 * time.Millisecond)

	if IsRunning(sockPath) {
		t.Fatal("agent should have stopped after TTL")
	}
}

func TestTTLZeroNoExpiry(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "", 0)
	defer srv.Stop()

	time.Sleep(200 * time.Millisecond)
	if !IsRunning(sockPath) {
		t.Fatal("agent with TTL=0 should still be running")
	}
}

func TestConcurrentClients(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "", 0)
	defer srv.Stop()

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := Get(sockPath, "KEY")
			if err != nil {
				errors <- err
				return
			}
			if val != "value" {
				errors <- fmt.Errorf("got %q, want %q", val, "value")
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Fatalf("concurrent client error: %v", err)
	}
}

func TestIsRunning_NotRunning(t *testing.T) {
	if IsRunning("/nonexistent/socket.sock") {
		t.Fatal("IsRunning should be false for nonexistent socket")
	}
}

func TestRename(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"OLD_KEY": "value",
	}, "", 0)
	defer srv.Stop()

	if err := Rename(sockPath, "OLD_KEY", "NEW_KEY"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if _, err := Get(sockPath, "OLD_KEY"); err == nil {
		t.Fatal("OLD_KEY should be gone after rename")
	}

	val, err := Get(sockPath, "NEW_KEY")
	if err != nil {
		t.Fatalf("Get NEW_KEY: %v", err)
	}
	if val != "value" {
		t.Fatalf("NEW_KEY = %q, want %q", val, "value")
	}
}

func TestRename_MissingKey(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, "", 0)
	defer srv.Stop()

	if err := Rename(sockPath, "NONEXISTENT", "NEW_KEY"); err == nil {
		t.Fatal("rename of missing key should fail")
	}
}

func TestRename_DestinationExists(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY_A": "a",
		"KEY_B": "b",
	}, "", 0)
	defer srv.Stop()

	if err := Rename(sockPath, "KEY_A", "KEY_B"); err == nil {
		t.Fatal("rename to existing key should fail")
	}

	valA, _ := Get(sockPath, "KEY_A")
	valB, _ := Get(sockPath, "KEY_B")
	if valA != "a" || valB != "b" {
		t.Fatalf("keys changed despite failed rename: A=%q B=%q", valA, valB)
	}
}

func TestSetAgentTTL(t *testing.T) {
	// Start with a short TTL, reset to infinite before it fires, then stop via -1.
	sockPath, _ := startTestServer(t, map[string]string{}, "", 200*time.Millisecond)

	// Reset to infinite before the 200ms TTL fires
	if err := SetAgentTTL(sockPath, 0); err != nil {
		t.Fatalf("SetAgentTTL(0): %v", err)
	}

	time.Sleep(400 * time.Millisecond)
	if !IsRunning(sockPath) {
		t.Fatal("agent should still be running after TTL was reset to infinite")
	}

	// Stop via -1
	if err := SetAgentTTL(sockPath, -1); err != nil {
		t.Fatalf("SetAgentTTL(-1): %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if IsRunning(sockPath) {
		t.Fatal("agent should have stopped after SetAgentTTL(-1)")
	}
}

// --- Write tests ---

func TestSet(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, "", 0)
	defer srv.Stop()

	if err := set(sockPath, "KEY", "value"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	val, err := Get(sockPath, "KEY")
	if err != nil {
		t.Fatalf("Get after set: %v", err)
	}
	if val != "value" {
		t.Fatalf("Get = %q, want %q", val, "value")
	}
}

func TestSetReplace(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "old_value",
	}, "", 0)
	defer srv.Stop()

	if err := set(sockPath, "KEY", "new_value"); err != nil {
		t.Fatalf("replace: %v", err)
	}

	val, _ := Get(sockPath, "KEY")
	if val != "new_value" {
		t.Fatalf("Get after replace = %q, want %q", val, "new_value")
	}
}

func TestSetBatch(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"EXISTING": "old",
	}, "", 0)
	defer srv.Stop()

	items := []SetItem{
		{Key: "NEW_A", Value: "a"},
		{Key: "NEW_B", Value: "b"},
		{Key: "EXISTING", Value: "new"},
	}
	if err := Set(sockPath, items); err != nil {
		t.Fatalf("SetBatch: %v", err)
	}

	for key, want := range map[string]string{"NEW_A": "a", "NEW_B": "b", "EXISTING": "new"} {
		val, err := Get(sockPath, key)
		if err != nil {
			t.Fatalf("Get %s: %v", key, err)
		}
		if val != want {
			t.Fatalf("Get %s = %q, want %q", key, val, want)
		}
	}
}

func TestDeleteBatch(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY_A": "a",
		"KEY_B": "b",
		"KEY_C": "c",
	}, "", 0)
	defer srv.Stop()

	if err := Delete(sockPath, []string{"KEY_A", "KEY_B"}); err != nil {
		t.Fatalf("DeleteBatch: %v", err)
	}

	if _, err := Get(sockPath, "KEY_A"); err == nil {
		t.Fatal("KEY_A should be gone")
	}
	if _, err := Get(sockPath, "KEY_B"); err == nil {
		t.Fatal("KEY_B should be gone")
	}
	// KEY_C untouched
	val, err := Get(sockPath, "KEY_C")
	if err != nil || val != "c" {
		t.Fatalf("KEY_C = %q, want %q", val, "c")
	}
}

func TestDelete_NonexistentKey(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, "", 0)
	defer srv.Stop()

	if err := del(sockPath, "NONEXISTENT"); err == nil {
		t.Fatal("delete nonexistent should fail")
	}
}

func TestPasswd(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "oldpass", 0)
	defer srv.Stop()

	// Wrong old passphrase
	if err := Passwd(sockPath, "wrong", "newpass"); err == nil {
		t.Fatal("passwd with wrong old passphrase should fail")
	}

	// Correct old passphrase
	if err := Passwd(sockPath, "oldpass", "newpass"); err != nil {
		t.Fatalf("passwd: %v", err)
	}

	// Data should still be readable after re-encryption
	val, err := Get(sockPath, "KEY")
	if err != nil {
		t.Fatalf("get after passwd: %v", err)
	}
	if val != "value" {
		t.Fatalf("value changed after passwd: %q", val)
	}

	// Writes still work after passphrase change
	if err := set(sockPath, "KEY", "updated"); err != nil {
		t.Fatalf("set after passwd: %v", err)
	}
}

func TestPasswd_EmptyToSet(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "", 0)
	defer srv.Stop()

	if err := Passwd(sockPath, "", "newpass"); err != nil {
		t.Fatalf("passwd empty to set: %v", err)
	}

	// Data readable after re-encryption
	val, err := Get(sockPath, "KEY")
	if err != nil {
		t.Fatalf("get after passwd: %v", err)
	}
	if val != "value" {
		t.Fatalf("value changed after passwd: %q", val)
	}
}

func TestConcurrentSets(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, "", 0)
	defer srv.Stop()

	// Use 5 concurrent writes (not 20) because each write does scrypt
	// encryption which is ~500ms, and the write lock serializes them.
	const n = 5
	var wg sync.WaitGroup
	errors := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := set(sockPath, fmt.Sprintf("KEY_%d", i), fmt.Sprintf("value_%d", i))
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Fatalf("concurrent set error: %v", err)
	}

	keys, err := List(sockPath)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != n {
		t.Fatalf("expected %d keys, got %d", n, len(keys))
	}
}

// --- History tests ---

func TestHistory_RecordedOnReplace(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "v1",
	}, "", 0)
	defer srv.Stop()

	if err := set(sockPath, "KEY", "v2"); err != nil {
		t.Fatalf("Set v2: %v", err)
	}
	if err := set(sockPath, "KEY", "v3"); err != nil {
		t.Fatalf("Set v3: %v", err)
	}

	hkeys, hvals, err := History(sockPath, "KEY")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	// Expected newest first: v2 (pushed by v3), v1 (pushed by v2)
	if len(hvals) != 2 {
		t.Fatalf("History len = %d, want 2", len(hvals))
	}
	if hvals[0] != "v2" || hvals[1] != "v1" {
		t.Fatalf("History values = %v, want [v2 v1]", hvals)
	}
	// Keys should be actual store key names
	if hkeys[0] != "KEY~2" || hkeys[1] != "KEY~1" {
		t.Fatalf("History keys = %v, want [KEY~2 KEY~1]", hkeys)
	}
}

func TestHistory_NotInList(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "v1",
	}, "", 0)
	defer srv.Stop()

	if err := set(sockPath, "KEY", "v2"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	keys, err := List(sockPath)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, k := range keys {
		if strings.ContainsRune(k, '~') {
			t.Fatalf("List returned history key %q", k)
		}
	}
}

func TestHistory_DeleteCascades(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "v1",
	}, "", 0)
	defer srv.Stop()

	if err := set(sockPath, "KEY", "v2"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := del(sockPath, "KEY"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, hvals, err := History(sockPath, "KEY")
	if err != nil {
		t.Fatalf("History after delete: %v", err)
	}
	if len(hvals) != 0 {
		t.Fatalf("History should be empty after cascade delete, got %v", hvals)
	}
}

func TestHistory_RenameCarriesHistory(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"OLD": "v1",
	}, "", 0)
	defer srv.Stop()

	if err := set(sockPath, "OLD", "v2"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := Rename(sockPath, "OLD", "NEW"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	hkeys, hvals, err := History(sockPath, "NEW")
	if err != nil {
		t.Fatalf("History NEW: %v", err)
	}
	if len(hvals) != 1 || hvals[0] != "v1" {
		t.Fatalf("NEW history values = %v, want [v1]", hvals)
	}
	if hkeys[0] != "NEW~1" {
		t.Fatalf("NEW history key = %q, want NEW~1", hkeys[0])
	}

	// OLD history should be gone
	_, oldVals, _ := History(sockPath, "OLD")
	if len(oldVals) != 0 {
		t.Fatalf("OLD history should be empty after rename, got %v", oldVals)
	}
}

func TestHistory_EmptyForNewKey(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, "", 0)
	defer srv.Stop()

	if err := set(sockPath, "KEY", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	_, hvals, err := History(sockPath, "KEY")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hvals) != 0 {
		t.Fatalf("New key should have no history, got %v", hvals)
	}
}

func TestSetPersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	storeDir := t.TempDir()
	t.Setenv("VARS_STORE_DIR", storeDir)
	sockPath := filepath.Join(dir, "agent.sock")

	backend := fastBackend{}
	if err := store.Init(backend); err != nil {
		t.Fatalf("store init: %v", err)
	}

	srv := NewServer(map[string]string{}, sockPath, "", backend, newFastBackend, storeDir)
	go func() { srv.Start(0) }()
	<-srv.Ready()
	defer srv.Stop()

	// Set a key via agent
	if err := set(sockPath, "PERSIST", "disk_value"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Read back from disk (bypass agent) to verify persistence
	s, err := store.Open(fastBackend{})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	val, err := s.Get("PERSIST")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if string(val) != "disk_value" {
		t.Fatalf("disk value = %q, want %q", val, "disk_value")
	}
}
