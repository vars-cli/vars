// Package crypto defines the interface for encryption backends.
//
// The Backend interface abstracts encryption and decryption so that
// different implementations (age/scrypt, Yubikey, SSH agent) can be
// swapped without changing the store or CLI layers.
package crypto

// Backend encrypts and decrypts arbitrary byte slices.
// Implementations are responsible for key management (passphrases,
// hardware tokens, etc.) — callers only see opaque bytes in and out.
type Backend interface {
	Encrypt(plaintext []byte) (ciphertext []byte, err error)
	Decrypt(ciphertext []byte) (plaintext []byte, err error)
}
