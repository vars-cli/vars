// Package age implements the crypto.Backend interface using filippo.io/age
// with scrypt passphrase-based encryption.
package age

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"

	"github.com/vars-cli/vars/internal/crypto"
)

// sentinelPassphrase is used internally when the user chooses "no passphrase".
// age's scrypt rejects empty strings, so we use a fixed sentinel instead.
// This is not a security measure — an empty passphrase is a deliberate user
// choice. The sentinel just satisfies the library constraint.
const sentinelPassphrase = "vars-no-passphrase"

// Ensure ScryptBackend implements crypto.Backend at compile time.
var _ crypto.Backend = (*ScryptBackend)(nil)

// ScryptBackend encrypts/decrypts using age's scrypt passphrase KDF.
type ScryptBackend struct {
	passphrase string
}

// New returns a ScryptBackend using the given passphrase.
// An empty passphrase is valid — it is mapped to an internal sentinel
// so the store is still encrypted (satisfying age's API) but can be
// decrypted without prompting the user.
func New(passphrase string) *ScryptBackend {
	p := passphrase
	if p == "" {
		p = sentinelPassphrase
	}
	return &ScryptBackend{passphrase: p}
}

// Encrypt encrypts plaintext using age scrypt with the configured passphrase.
func (b *ScryptBackend) Encrypt(plaintext []byte) ([]byte, error) {
	recipient, err := age.NewScryptRecipient(b.passphrase)
	if err != nil {
		return nil, fmt.Errorf("creating scrypt recipient: %w", err)
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("initializing encryption: %w", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("writing encrypted data: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("finalizing encryption: %w", err)
	}
	return buf.Bytes(), nil
}

// Decrypt decrypts ciphertext using age scrypt with the configured passphrase.
func (b *ScryptBackend) Decrypt(ciphertext []byte) ([]byte, error) {
	identity, err := age.NewScryptIdentity(b.passphrase)
	if err != nil {
		return nil, fmt.Errorf("creating scrypt identity: %w", err)
	}

	r, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading decrypted data: %w", err)
	}
	return plaintext, nil
}

// NewBackend creates a ScryptBackend and returns it as a crypto.Backend.
// Use this as a backend factory where func(string) crypto.Backend is required.
func NewBackend(passphrase string) crypto.Backend {
	return New(passphrase)
}

// TrialDecryptEmpty attempts to decrypt with the sentinel (empty) passphrase.
// Returns the plaintext and true if successful, or nil and false if
// the decryption fails (meaning a real passphrase is required).
func TrialDecryptEmpty(ciphertext []byte) ([]byte, bool) {
	backend := New("") // maps to sentinel
	plaintext, err := backend.Decrypt(ciphertext)
	if err != nil {
		return nil, false
	}
	return plaintext, true
}
