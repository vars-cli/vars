package age

import (
	"bytes"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	passphrase := "test-passphrase"
	plaintext := []byte("hello world")

	backend := New(passphrase)

	ciphertext, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext should differ from plaintext")
	}

	decrypted, err := backend.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestEmptyPassphrase(t *testing.T) {
	backend := New("")
	plaintext := []byte("secret data with no passphrase")

	ciphertext, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt with empty passphrase: %v", err)
	}

	decrypted, err := backend.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt with empty passphrase: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestWrongPassphrase(t *testing.T) {
	correct := New("correct-passphrase")
	wrong := New("wrong-passphrase")

	plaintext := []byte("sensitive data")
	ciphertext, err := correct.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = wrong.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("Decrypt with wrong passphrase should fail")
	}
}

func TestTrialDecryptEmpty_NoPassphrase(t *testing.T) {
	backend := New("")
	plaintext := []byte("unprotected data")

	ciphertext, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, ok := TrialDecryptEmpty(ciphertext)
	if !ok {
		t.Fatal("TrialDecryptEmpty should succeed for empty-passphrase store")
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestTrialDecryptEmpty_WithPassphrase(t *testing.T) {
	backend := New("real-passphrase")
	plaintext := []byte("protected data")

	ciphertext, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, ok := TrialDecryptEmpty(ciphertext)
	if ok {
		t.Fatal("TrialDecryptEmpty should fail for passphrase-protected store")
	}
}

func TestEmptyPlaintext(t *testing.T) {
	backend := New("test-passphrase")
	plaintext := []byte{}

	ciphertext, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	decrypted, err := backend.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if len(decrypted) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(decrypted))
	}
}

func TestLargePayload(t *testing.T) {
	backend := New("test-passphrase")
	// 1MB of data
	plaintext := bytes.Repeat([]byte("A"), 1024*1024)

	ciphertext, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt large: %v", err)
	}

	decrypted, err := backend.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt large: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("large payload round-trip mismatch")
	}
}

func TestBinaryData(t *testing.T) {
	backend := New("test-passphrase")
	// All byte values 0-255
	plaintext := make([]byte, 256)
	for i := range plaintext {
		plaintext[i] = byte(i)
	}

	ciphertext, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt binary: %v", err)
	}

	decrypted, err := backend.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt binary: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("binary data round-trip mismatch")
	}
}

func TestDecryptGarbage(t *testing.T) {
	backend := New("test-passphrase")
	_, err := backend.Decrypt([]byte("not encrypted data"))
	if err == nil {
		t.Fatal("Decrypt garbage should fail")
	}
}

func TestDecryptTruncated(t *testing.T) {
	backend := New("test-passphrase")
	ciphertext, err := backend.Encrypt([]byte("hello"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Truncate to half
	truncated := ciphertext[:len(ciphertext)/2]
	_, err = backend.Decrypt(truncated)
	if err == nil {
		t.Fatal("Decrypt truncated should fail")
	}
}

func TestDifferentPassphrasesProduceDifferentCiphertext(t *testing.T) {
	plaintext := []byte("same data")
	b1 := New("passphrase-1")
	b2 := New("passphrase-2")

	c1, err := b1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}
	c2, err := b2.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}

	if bytes.Equal(c1, c2) {
		t.Fatal("different passphrases should produce different ciphertext")
	}
}

func TestEncryptIsNondeterministic(t *testing.T) {
	backend := New("test-passphrase")
	plaintext := []byte("same data")

	c1, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}
	c2, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}

	if bytes.Equal(c1, c2) {
		t.Fatal("same plaintext encrypted twice should produce different ciphertext (random salt)")
	}
}

func TestUnicodePassphrase(t *testing.T) {
	backend := New("p\u00e4ssw\u00f6rd-\u00fc\u00f1\u00ee\u00e7\u00f8d\u00e9")
	plaintext := []byte("unicode passphrase test")

	ciphertext, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := backend.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestLongPassphrase(t *testing.T) {
	backend := New(strings.Repeat("a", 10000))
	plaintext := []byte("long passphrase test")

	ciphertext, err := backend.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := backend.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}
