package cmd

import (
	"fmt"
	"net"
	"os"

	"github.com/brickpop/secrets/internal/agent"
	agebackend "github.com/brickpop/secrets/internal/crypto/age"
	"github.com/brickpop/secrets/internal/prompt"
	"github.com/brickpop/secrets/internal/store"
)

// ReadStore is the interface used by read-only commands (get, ls, export, dump).
// Both *store.Store and *agentStore satisfy it.
type ReadStore interface {
	Get(key string) ([]byte, error)
	List() []string
	Close()
}

// agentStore wraps the agent client behind the ReadStore interface.
type agentStore struct {
	sockPath string
}

func (a *agentStore) Get(key string) ([]byte, error) {
	val, err := agent.Get(a.sockPath, key)
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

func (a *agentStore) List() []string {
	keys, err := agent.List(a.sockPath)
	if err != nil {
		return nil
	}
	return keys
}

func (a *agentStore) Close() {}

// openStore handles trial-decrypt and passphrase prompting.
// Returns a read-write *store.Store. Used by set, rm, passwd.
func openStore() (*store.Store, error) {
	if !store.Exists() {
		return nil, UserError("No store found. Run 'secrets init' to create one.")
	}

	for _, w := range store.CheckPermissions() {
		fmt.Fprintln(os.Stderr, w)
	}

	ciphertext, err := os.ReadFile(store.FilePath())
	if err != nil {
		return nil, InternalError(fmt.Sprintf("reading store: %v", err))
	}

	// Trial-decrypt with empty passphrase
	if plaintext, ok := agebackend.TrialDecryptEmpty(ciphertext); ok {
		backend := agebackend.New("")
		s, err := store.OpenFromBytes(plaintext, backend, store.Dir())
		if err != nil {
			return nil, InternalError(err.Error())
		}
		return s, nil
	}

	p := prompt.New(os.Stdin, os.Stderr)
	passphrase, err := p.Passphrase("Passphrase: ")
	if err != nil {
		return nil, UserError(err.Error())
	}

	backend := agebackend.New(passphrase)
	s, err := store.Open(backend)
	if err != nil {
		return nil, UserError("Incorrect passphrase.")
	}
	return s, nil
}

// openStoreReadOnly returns a ReadStore. It tries the agent first
// (if SECRETS_AGENT_SOCK is set and reachable), falling back to file.
func openStoreReadOnly() (ReadStore, error) {
	sockPath := agentSocketPath()
	if agent.IsRunning(sockPath) {
		return &agentStore{sockPath: sockPath}, nil
	}
	return openStore()
}

// agentSocketPath returns the agent socket path.
func agentSocketPath() string {
	if sock := os.Getenv("SECRETS_AGENT_SOCK"); sock != "" {
		return sock
	}
	return store.Dir() + "/agent.sock"
}

// tryStopAgent attempts to stop a running agent. Returns true if stopped.
func tryStopAgent() bool {
	sockPath := agentSocketPath()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return false
	}
	defer conn.Close()
	_, err = conn.Write([]byte(`{"op":"stop"}` + "\n"))
	return err == nil
}

// printManifestHint prints a hint if .secrets.yaml exists in cwd
// and the key is not listed in it.
func printManifestHint(key string) {
	data, err := os.ReadFile(".secrets.yaml")
	if err != nil {
		return
	}
	content := string(data)
	if !containsKey(content, key) {
		fmt.Fprintf(os.Stderr, "Hint: %q is not listed in .secrets.yaml. Consider adding it.\n", key)
	}
}

// containsKey checks if a key appears as a YAML list item (- KEY).
func containsKey(yamlContent string, key string) bool {
	patterns := []string{
		"- " + key + "\n",
		"- " + key + "\r",
		"- " + key,
	}
	for _, p := range patterns {
		if len(yamlContent) >= len(p) {
			for i := 0; i <= len(yamlContent)-len(p); i++ {
				if yamlContent[i:i+len(p)] == p {
					return true
				}
			}
		}
	}
	return false
}
