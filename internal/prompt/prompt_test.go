package prompt

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func newTestPrompter(input string) (*Prompter, *bytes.Buffer) {
	r := strings.NewReader(input)
	var w bytes.Buffer
	return New(r, &w), &w
}

func TestPassphrase_ReadsLine(t *testing.T) {
	p, w := newTestPrompter("my-secret\n")

	got, err := p.Passphrase("Enter: ")
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	if got != "my-secret" {
		t.Fatalf("got %q, want %q", got, "my-secret")
	}
	if !strings.Contains(w.String(), "Enter: ") {
		t.Fatalf("prompt not written: %q", w.String())
	}
}

func TestPassphrase_EmptyInput(t *testing.T) {
	p, _ := newTestPrompter("\n")

	got, err := p.Passphrase("Enter: ")
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestPassphrase_EOF(t *testing.T) {
	p, _ := newTestPrompter("")

	_, err := p.Passphrase("Enter: ")
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestPassphraseConfirm_Match(t *testing.T) {
	p, _ := newTestPrompter("secret\nsecret\n")

	got, err := p.PassphraseConfirm("Pass: ", "Confirm: ")
	if err != nil {
		t.Fatalf("PassphraseConfirm: %v", err)
	}
	if got != "secret" {
		t.Fatalf("got %q, want %q", got, "secret")
	}
}

func TestPassphraseConfirm_Mismatch(t *testing.T) {
	p, _ := newTestPrompter("first\nsecond\n")

	_, err := p.PassphraseConfirm("Pass: ", "Confirm: ")
	if err == nil {
		t.Fatal("mismatched passphrases should fail")
	}
	if !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPassphraseConfirm_EmptyBoth(t *testing.T) {
	p, _ := newTestPrompter("\n\n")

	got, err := p.PassphraseConfirm("Pass: ", "Confirm: ")
	if err != nil {
		t.Fatalf("PassphraseConfirm: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestPassphraseConfirm_FirstEmptySecondNot(t *testing.T) {
	p, _ := newTestPrompter("\nsomething\n")

	_, err := p.PassphraseConfirm("Pass: ", "Confirm: ")
	if err == nil {
		t.Fatal("empty + non-empty should fail")
	}
}

func TestValue_ReadsLine(t *testing.T) {
	p, _ := newTestPrompter("0xdeadbeef\n")

	got, err := p.Value("Value: ")
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got != "0xdeadbeef" {
		t.Fatalf("got %q, want %q", got, "0xdeadbeef")
	}
}

func TestConfirm_Yes(t *testing.T) {
	tests := []string{"y\n", "Y\n", "yes\n", "YES\n", "Yes\n"}
	for _, input := range tests {
		p, _ := newTestPrompter(input)

		got, err := p.Confirm("Delete? [y/N] ")
		if err != nil {
			t.Fatalf("Confirm(%q): %v", input, err)
		}
		if !got {
			t.Fatalf("Confirm(%q) = false, want true", strings.TrimSpace(input))
		}
	}
}

func TestConfirm_No(t *testing.T) {
	tests := []string{"n\n", "N\n", "no\n", "NO\n", "\n", "maybe\n", "yep\n"}
	for _, input := range tests {
		p, _ := newTestPrompter(input)

		got, err := p.Confirm("Delete? [y/N] ")
		if err != nil {
			t.Fatalf("Confirm(%q): %v", input, err)
		}
		if got {
			t.Fatalf("Confirm(%q) = true, want false", strings.TrimSpace(input))
		}
	}
}

func TestConfirm_WithWhitespace(t *testing.T) {
	p, _ := newTestPrompter("  y  \n")

	got, err := p.Confirm("Delete? [y/N] ")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !got {
		t.Fatal("Confirm with whitespace-padded 'y' should be true")
	}
}

func TestConfirm_EOF(t *testing.T) {
	p, _ := newTestPrompter("")

	_, err := p.Confirm("Delete? [y/N] ")
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestMultipleReads(t *testing.T) {
	p, _ := newTestPrompter("first\nsecond\nthird\n")

	v1, err := p.Passphrase("1: ")
	if err != nil {
		t.Fatalf("read 1: %v", err)
	}
	v2, err := p.Value("2: ")
	if err != nil {
		t.Fatalf("read 2: %v", err)
	}
	v3, err := p.Passphrase("3: ")
	if err != nil {
		t.Fatalf("read 3: %v", err)
	}

	if v1 != "first" || v2 != "second" || v3 != "third" {
		t.Fatalf("got %q/%q/%q, want first/second/third", v1, v2, v3)
	}
}

func TestPassphrase_WithCarriageReturn(t *testing.T) {
	p, _ := newTestPrompter("secret\r\n")

	got, err := p.Passphrase("Enter: ")
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	if got != "secret" {
		t.Fatalf("got %q, want %q", got, "secret")
	}
}

func TestPrompterOutputContainsAllPrompts(t *testing.T) {
	p, w := newTestPrompter("pass\npass\n")

	_, err := p.PassphraseConfirm("Enter passphrase: ", "Confirm: ")
	if err != nil {
		t.Fatalf("PassphraseConfirm: %v", err)
	}

	output := w.String()
	if !strings.Contains(output, "Enter passphrase: ") {
		t.Fatalf("missing first prompt in output: %q", output)
	}
	if !strings.Contains(output, "Confirm: ") {
		t.Fatalf("missing confirm prompt in output: %q", output)
	}
}
