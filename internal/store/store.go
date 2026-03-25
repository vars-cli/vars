// Package store provides CRUD operations on an encrypted key-value store.
//
// The store is a flat JSON object persisted as an encrypted file. It uses
// a crypto.Backend for encryption/decryption and knows nothing about the
// specific encryption scheme.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/vars-cli/vars/internal/crypto"
)

const metaFileName = "meta.json"

// Meta holds store-level metadata written at init time.
// It is unencrypted and used to determine how to open the store.
type Meta struct {
	Backend string `json:"backend"`
}

const (
	appDirName    = "vars"
	storeFileName = "store.age"

	dirPerm  = 0700
	filePerm = 0600
)

// Store holds decrypted key-value data in memory.
// Secret values are stored as []byte for memory safety (can be zeroed).
type Store struct {
	data    map[string][]byte
	backend crypto.Backend
	dir     string
}

// Dir returns the store directory path.
// Priority: VARS_STORE_DIR > XDG_DATA_HOME/vars > ~/.local/share/vars
func Dir() string {
	if d := os.Getenv("VARS_STORE_DIR"); d != "" {
		return d
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, appDirName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".local", "share", appDirName)
	}
	return filepath.Join(home, ".local", "share", appDirName)
}

// FilePath returns the full path to the store file.
func FilePath() string {
	return filepath.Join(Dir(), storeFileName)
}

// Exists returns true if the store file exists.
func Exists() bool {
	_, err := os.Stat(FilePath())
	return err == nil
}

// Init creates a new empty store encrypted with the given backend.
// Creates the directory if needed with 0700 permissions.
// Returns an error if the store already exists.
func Init(backend crypto.Backend) error {
	dir := Dir()

	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("creating store directory: %w", err)
	}
	// MkdirAll respects umask, so enforce exact permissions
	if err := os.Chmod(dir, dirPerm); err != nil {
		return fmt.Errorf("setting directory permissions: %w", err)
	}

	storePath := filepath.Join(dir, storeFileName)
	if _, err := os.Stat(storePath); err == nil {
		return fmt.Errorf("store already exists at %s", storePath)
	}

	// Encrypt an empty JSON object
	emptyStore := []byte("{}")
	ciphertext, err := backend.Encrypt(emptyStore)
	if err != nil {
		return fmt.Errorf("encrypting initial store: %w", err)
	}

	if err := atomicWrite(storePath, ciphertext); err != nil {
		return fmt.Errorf("writing store: %w", err)
	}

	metaBytes, err := json.Marshal(Meta{Backend: "scrypt"})
	if err != nil {
		return fmt.Errorf("serializing meta: %w", err)
	}
	metaPath := filepath.Join(dir, metaFileName)
	if err := atomicWrite(metaPath, metaBytes); err != nil {
		return fmt.Errorf("writing meta: %w", err)
	}

	return nil
}

// Open decrypts the store file and returns a Store for in-memory operations.
// The caller must call Close() when done to zero secret memory.
func Open(backend crypto.Backend) (*Store, error) {
	dir := Dir()
	storePath := filepath.Join(dir, storeFileName)

	ciphertext, err := os.ReadFile(storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no store found. Store not found — run 'vars' to get started")
		}
		return nil, fmt.Errorf("reading store: %w", err)
	}

	plaintext, err := backend.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypting store: %w", err)
	}

	data := make(map[string][]byte)
	// Parse JSON into string→string first, then convert values to []byte
	var raw map[string]string
	if err := json.Unmarshal(plaintext, &raw); err != nil {
		zeroBytes(plaintext)
		return nil, fmt.Errorf("parsing store data: %w", err)
	}
	zeroBytes(plaintext)

	for k, v := range raw {
		data[k] = []byte(v)
	}

	return &Store{data: data, backend: backend, dir: dir}, nil
}

// Get returns the value for a key, or an error if not found.
func (s *Store) Get(key string) ([]byte, error) {
	val, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in store", key)
	}
	// Return a copy to prevent caller from modifying internal state
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

// Set stores a value for a key. Overwrites if the key already exists.
func (s *Store) Set(key string, value []byte) {
	// Zero old value if overwriting
	if old, ok := s.data[key]; ok {
		zeroBytes(old)
	}
	val := make([]byte, len(value))
	copy(val, value)
	s.data[key] = val
}

// Delete removes a key from the store. Returns an error if not found.
func (s *Store) Delete(key string) error {
	val, ok := s.data[key]
	if !ok {
		return fmt.Errorf("key %q not found in store", key)
	}
	zeroBytes(val)
	delete(s.data, key)
	return nil
}

// List returns all key names sorted lexicographically.
func (s *Store) List() []string {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Save encrypts the current state and writes it atomically to disk.
func (s *Store) Save() error {
	// Serialize to JSON (string values)
	raw := make(map[string]string, len(s.data))
	for k, v := range s.data {
		raw[k] = string(v)
	}

	plaintext, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("serializing store: %w", err)
	}

	ciphertext, err := s.backend.Encrypt(plaintext)
	zeroBytes(plaintext)
	if err != nil {
		return fmt.Errorf("encrypting store: %w", err)
	}

	storePath := filepath.Join(s.dir, storeFileName)
	return atomicWrite(storePath, ciphertext)
}

// SaveWithBackend encrypts with a different backend and writes to disk.
// Used by passwd to re-encrypt with a new passphrase.
func (s *Store) SaveWithBackend(backend crypto.Backend) error {
	raw := make(map[string]string, len(s.data))
	for k, v := range s.data {
		raw[k] = string(v)
	}

	plaintext, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("serializing store: %w", err)
	}

	ciphertext, err := backend.Encrypt(plaintext)
	zeroBytes(plaintext)
	if err != nil {
		return fmt.Errorf("encrypting store: %w", err)
	}

	storePath := filepath.Join(s.dir, storeFileName)
	return atomicWrite(storePath, ciphertext)
}

// Close zeros all secret data in memory.
func (s *Store) Close() {
	for k, v := range s.data {
		zeroBytes(v)
		delete(s.data, k)
	}
}

// CheckPermissions verifies directory and file permissions.
// Returns a list of warning messages for anything too permissive.
func CheckPermissions() []string {
	var warnings []string
	dir := Dir()

	if info, err := os.Stat(dir); err == nil {
		perm := info.Mode().Perm()
		if perm != dirPerm {
			warnings = append(warnings, fmt.Sprintf(
				"Warning: %s has permissions %04o, expected %04o. Fix with: chmod %04o %s",
				dir, perm, dirPerm, dirPerm, dir,
			))
		}
	}

	storePath := filepath.Join(dir, storeFileName)
	if info, err := os.Stat(storePath); err == nil {
		perm := info.Mode().Perm()
		if perm != filePerm {
			warnings = append(warnings, fmt.Sprintf(
				"Warning: %s has permissions %04o, expected %04o. Fix with: chmod %04o %s",
				storePath, perm, filePerm, filePerm, storePath,
			))
		}
	}

	return warnings
}

// SaveData encrypts a map[string]string and writes it atomically to disk.
// Used by the agent server which holds data as strings, not *Store.
func SaveData(data map[string]string, backend crypto.Backend, dir string) error {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("serializing store: %w", err)
	}

	ciphertext, err := backend.Encrypt(plaintext)
	zeroBytes(plaintext)
	if err != nil {
		return fmt.Errorf("encrypting store: %w", err)
	}

	storePath := filepath.Join(dir, storeFileName)
	return atomicWrite(storePath, ciphertext)
}

// atomicWrite writes data to a temp file then renames it to path.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".store-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Clean up on any error
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(filePerm); err != nil {
		tmp.Close()
		return fmt.Errorf("setting temp file permissions: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}

	success = true
	return nil
}

// zeroBytes overwrites a byte slice with zeros.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
