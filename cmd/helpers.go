package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/vars-cli/vars/internal/agent"
	agebackend "github.com/vars-cli/vars/internal/crypto/age"
	"github.com/vars-cli/vars/internal/prompt"
	"github.com/vars-cli/vars/internal/store"
)
// defaultTTL returns the default agent lifetime in seconds.
// Reads VARS_AGENT_TTL if set (e.g. "4h", "30m", "1d", "0" for unlimited),
// falls back to 8 hours.
func defaultTTL() int64 {
	if s := os.Getenv("VARS_AGENT_TTL"); s != "" {
		if ttl, err := parseTTLSeconds(s); err == nil {
			return ttl
		}
	}
	return 8 * 60 * 60
}

// stdinPrompt is a lazily-initialized Prompter backed by os.Stdin.
// All code must use this instead of prompt.New(os.Stdin, ...) to avoid
// creating multiple bufio.Readers over the same stdin.
var stdinPrompt *prompt.Prompter

func stdinPrompter() *prompt.Prompter {
	if stdinPrompt == nil {
		stdinPrompt = prompt.New(os.Stdin, os.Stderr)
	}
	return stdinPrompt
}

// ensureAgent ensures a running agent, auto-starting one if needed.
// If no agent is running, it prompts for passphrase if required and starts the daemon.
func ensureAgent() error {
	if agent.IsRunning(agentSocketPath()) {
		return nil
	}
	_, err := startAgent(defaultTTL())
	return err
}

// createStore walks the user through creating the store for the first time.
// Called by startAgent when no store exists yet.
// Returns the chosen passphrase so the caller can launch the daemon.
func createStore() (string, error) {
	fmt.Fprintf(os.Stderr, "No store found — let's create one.\n\n")
	fmt.Fprintf(os.Stderr, "Your environment variables will be kept in an encrypted file at:\n")
	fmt.Fprintf(os.Stderr, "  %s\n\n", store.FilePath())
	fmt.Fprintf(os.Stderr, "A passphrase adds an extra layer of protection (optional).\n")
	fmt.Fprintf(os.Stderr, "You can add or change it at any time with `vars passwd`.\n\n")

	passphrase, err := stdinPrompter().PassphraseConfirm(
		"Passphrase (leave empty for none): ",
		"Confirm passphrase: ",
	)
	if err != nil {
		return "", UserError(err.Error())
	}

	if err := store.Init(agebackend.New(passphrase)); err != nil {
		return "", InternalError(err.Error())
	}

	fmt.Fprintf(os.Stderr, "\nStore created. Starting agent...\n")
	return passphrase, nil
}

// agentSocketPath returns the agent socket path.
func agentSocketPath() string {
	if sock := os.Getenv("VARS_AGENT_SOCK"); sock != "" {
		return sock
	}
	return store.Dir() + "/agent.sock"
}

// printManifestHint prints a hint if .vars.yaml exists in cwd
// and the key is not listed in it. Strips scope prefix before checking
// so that "prod/RPC_URL" correctly matches "- RPC_URL" in the manifest.
func printManifestHint(key string) {
	data, err := os.ReadFile(".vars.yaml")
	if err != nil {
		return
	}
	bareKey := key
	if i := strings.IndexByte(key, '/'); i >= 0 {
		bareKey = key[i+1:]
	}
	if !containsKey(string(data), bareKey) {
		fmt.Fprintf(os.Stderr, "Hint: %q is not listed in .vars.yaml. Consider adding it.\n", key)
	}
}

// containsKey checks if a key appears as a YAML list item (- KEY).
func containsKey(yamlContent string, key string) bool {
	needle := "- " + key
	idx := strings.Index(yamlContent, needle)
	if idx < 0 {
		return false
	}
	// Ensure it's at end-of-string or followed by a newline (not a prefix of another key).
	end := idx + len(needle)
	return end == len(yamlContent) || yamlContent[end] == '\n' || yamlContent[end] == '\r'
}
