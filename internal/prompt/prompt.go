// Package prompt provides terminal I/O for passphrases, values, and confirmations.
//
// The Prompter type wraps an io.Reader with buffering so multiple
// sequential reads work correctly. For real terminal usage, passphrase
// prompts use golang.org/x/term to suppress echo.
package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Prompter handles interactive prompts with a shared buffered reader.
type Prompter struct {
	r     *bufio.Reader
	w     io.Writer
	isTTY bool
	fd    int
}

// New creates a Prompter from a reader and writer.
// For testing, pass strings.NewReader / bytes.Buffer.
func New(r io.Reader, w io.Writer) *Prompter {
	p := &Prompter{
		r: bufio.NewReader(r),
		w: w,
	}
	if f, ok := r.(*os.File); ok {
		p.fd = int(f.Fd())
		p.isTTY = term.IsTerminal(p.fd)
	}
	return p
}

// Passphrase prompts for a passphrase with echo disabled on TTYs.
func (p *Prompter) Passphrase(msg string) (string, error) {
	fmt.Fprint(p.w, msg)

	if p.isTTY {
		pass, err := term.ReadPassword(p.fd)
		fmt.Fprintln(p.w)
		if err != nil {
			return "", fmt.Errorf("reading passphrase: %w", err)
		}
		return string(pass), nil
	}

	return p.readLine()
}

// PassphraseConfirm prompts twice and ensures both entries match.
// An empty passphrase is allowed.
func (p *Prompter) PassphraseConfirm(msg string, confirmMsg string) (string, error) {
	pass1, err := p.Passphrase(msg)
	if err != nil {
		return "", err
	}
	pass2, err := p.Passphrase(confirmMsg)
	if err != nil {
		return "", err
	}
	if pass1 != pass2 {
		return "", fmt.Errorf("passphrases do not match")
	}
	return pass1, nil
}

// Value prompts for a secret value with echo disabled.
func (p *Prompter) Value(msg string) (string, error) {
	return p.Passphrase(msg)
}

// Confirm prompts for y/N confirmation.
// Returns true only on explicit "y" or "yes". Default is no.
func (p *Prompter) Confirm(msg string) (bool, error) {
	fmt.Fprint(p.w, msg)
	line, err := p.readLine()
	if err != nil {
		return false, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes", nil
}

func (p *Prompter) readLine() (string, error) {
	line, err := p.r.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
