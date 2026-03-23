package agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brickpop/secrets/internal/crypto"
	"github.com/brickpop/secrets/internal/store"
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
	t.Setenv("SECRETS_STORE_DIR", storeDir)
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
	}, "secret", 0)
	defer srv.Stop()

	// Basic rename
	if err := Rename(sockPath, "OLD_KEY", "NEW_KEY", "secret"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	// Old key gone
	if _, err := Get(sockPath, "OLD_KEY"); err == nil {
		t.Fatal("OLD_KEY should be gone after rename")
	}

	// New key has the value
	val, err := Get(sockPath, "NEW_KEY")
	if err != nil {
		t.Fatalf("Get NEW_KEY: %v", err)
	}
	if val != "value" {
		t.Fatalf("NEW_KEY = %q, want %q", val, "value")
	}
}

func TestRename_WrongPassphrase(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "secret", 0)
	defer srv.Stop()

	if err := Rename(sockPath, "KEY", "KEY2", "wrong"); err == nil {
		t.Fatal("rename with wrong passphrase should fail")
	}

	// Original key untouched
	val, _ := Get(sockPath, "KEY")
	if val != "value" {
		t.Fatalf("KEY changed despite failed rename: %q", val)
	}
}

func TestRename_MissingKey(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, "secret", 0)
	defer srv.Stop()

	if err := Rename(sockPath, "NONEXISTENT", "NEW_KEY", "secret"); err == nil {
		t.Fatal("rename of missing key should fail")
	}
}

func TestRename_DestinationExists(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY_A": "a",
		"KEY_B": "b",
	}, "secret", 0)
	defer srv.Stop()

	if err := Rename(sockPath, "KEY_A", "KEY_B", "secret"); err == nil {
		t.Fatal("rename to existing key should fail")
	}

	// Both keys unchanged
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

func TestSetNewKey_NoPassphrase(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, "secret", 0)
	defer srv.Stop()

	// New key should work without passphrase
	err := Set(sockPath, "NEW_KEY", "new_value", "")
	if err != nil {
		t.Fatalf("Set new key: %v", err)
	}

	val, err := Get(sockPath, "NEW_KEY")
	if err != nil {
		t.Fatalf("Get after set: %v", err)
	}
	if val != "new_value" {
		t.Fatalf("Get = %q, want %q", val, "new_value")
	}
}

func TestSetOverwrite_RequiresPassphrase(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "old_value",
	}, "secret", 0)
	defer srv.Stop()

	// Overwrite with wrong passphrase should fail
	err := Set(sockPath, "KEY", "new_value", "wrong")
	if err == nil {
		t.Fatal("overwrite with wrong passphrase should fail")
	}
	if !strings.Contains(err.Error(), ErrPassphraseRequired) {
		t.Fatalf("unexpected error: %v", err)
	}

	// Value should be unchanged
	val, _ := Get(sockPath, "KEY")
	if val != "old_value" {
		t.Fatalf("value changed despite failed overwrite: %q", val)
	}

	// Overwrite with correct passphrase should succeed
	err = Set(sockPath, "KEY", "new_value", "secret")
	if err != nil {
		t.Fatalf("overwrite with correct passphrase: %v", err)
	}

	val, _ = Get(sockPath, "KEY")
	if val != "new_value" {
		t.Fatalf("Get after overwrite = %q, want %q", val, "new_value")
	}
}

func TestSetOverwrite_EmptyPassphrase(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "old_value",
	}, "", 0)
	defer srv.Stop()

	// Overwrite with empty passphrase (store has no passphrase) should work
	err := Set(sockPath, "KEY", "new_value", "")
	if err != nil {
		t.Fatalf("overwrite with empty passphrase: %v", err)
	}

	val, _ := Get(sockPath, "KEY")
	if val != "new_value" {
		t.Fatalf("Get = %q, want %q", val, "new_value")
	}
}

func TestDelete_RequiresPassphrase(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "secret", 0)
	defer srv.Stop()

	// Delete with wrong passphrase
	err := Delete(sockPath, "KEY", "wrong")
	if err == nil {
		t.Fatal("delete with wrong passphrase should fail")
	}
	if !strings.Contains(err.Error(), ErrPassphraseRequired) {
		t.Fatalf("unexpected error: %v", err)
	}

	// Key should still exist
	val, _ := Get(sockPath, "KEY")
	if val != "value" {
		t.Fatalf("key changed despite failed delete: %q", val)
	}

	// Delete with correct passphrase
	err = Delete(sockPath, "KEY", "secret")
	if err != nil {
		t.Fatalf("delete with correct passphrase: %v", err)
	}

	// Key should be gone
	_, err = Get(sockPath, "KEY")
	if err == nil {
		t.Fatal("key should be gone after delete")
	}
}

func TestDelete_NonexistentKey(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, "secret", 0)
	defer srv.Stop()

	err := Delete(sockPath, "NONEXISTENT", "secret")
	if err == nil {
		t.Fatal("delete nonexistent should fail")
	}
}

func TestPasswd(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "oldpass", 0)
	defer srv.Stop()

	// Wrong old passphrase
	err := Passwd(sockPath, "wrong", "newpass")
	if err == nil {
		t.Fatal("passwd with wrong old passphrase should fail")
	}

	// Correct old passphrase
	err = Passwd(sockPath, "oldpass", "newpass")
	if err != nil {
		t.Fatalf("passwd: %v", err)
	}

	// Data should still be readable
	val, err := Get(sockPath, "KEY")
	if err != nil {
		t.Fatalf("get after passwd: %v", err)
	}
	if val != "value" {
		t.Fatalf("value changed after passwd: %q", val)
	}

	// Old passphrase should no longer work for writes
	err = Set(sockPath, "KEY", "updated", "oldpass")
	if err == nil {
		t.Fatal("old passphrase should no longer work")
	}

	// New passphrase should work for writes
	err = Set(sockPath, "KEY", "updated", "newpass")
	if err != nil {
		t.Fatalf("set with new passphrase: %v", err)
	}
}

func TestPasswd_EmptyToSet(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, "", 0)
	defer srv.Stop()

	err := Passwd(sockPath, "", "newpass")
	if err != nil {
		t.Fatalf("passwd empty to set: %v", err)
	}

	// Now overwrites require newpass
	err = Set(sockPath, "KEY", "updated", "")
	if err == nil {
		t.Fatal("empty passphrase should no longer work")
	}

	err = Set(sockPath, "KEY", "updated", "newpass")
	if err != nil {
		t.Fatalf("set with new passphrase: %v", err)
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
			key := fmt.Sprintf("KEY_%d", i)
			err := Set(sockPath, key, fmt.Sprintf("value_%d", i), "")
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

func TestSetPersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	storeDir := t.TempDir()
	t.Setenv("SECRETS_STORE_DIR", storeDir)
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
	if err := Set(sockPath, "PERSIST", "disk_value", ""); err != nil {
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

