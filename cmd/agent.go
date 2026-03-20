package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
	agebackend "github.com/brickpop/secrets/internal/crypto/age"
	"github.com/brickpop/secrets/internal/prompt"
	"github.com/brickpop/secrets/internal/store"
)

var agentTTL string

func init() {
	agentCmd.Flags().StringVar(&agentTTL, "ttl", "8h", "Agent lifetime (e.g. 8h, 30m, 0 for unlimited)")
	agentCmd.AddCommand(agentStopCmd)
	rootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start the background agent",
	Long: `Start a background agent that holds the decrypted store in memory.
The agent is read-only and serves get/list requests over a Unix socket.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sockPath := agentSocketPath()

		// Check if already running
		if agent.IsRunning(sockPath) {
			fmt.Fprintf(os.Stdout, "export SECRETS_AGENT_SOCK=%s\n", sockPath)
			return nil
		}

		// Check for --daemon flag (internal, used by re-exec)
		if os.Getenv("_SECRETS_AGENT_DAEMON") == "1" {
			return runDaemon(sockPath)
		}

		// Parse TTL
		ttl, err := parseTTL(agentTTL)
		if err != nil {
			return UserError(fmt.Sprintf("Invalid TTL: %v", err))
		}

		// Decrypt the store to verify passphrase before forking
		if !store.Exists() {
			return UserError("No store found. Run 'secrets init' to create one.")
		}

		ciphertext, err := os.ReadFile(store.FilePath())
		if err != nil {
			return InternalError(fmt.Sprintf("reading store: %v", err))
		}

		var plaintext []byte
		if pt, ok := agebackend.TrialDecryptEmpty(ciphertext); ok {
			plaintext = pt
		} else {
			p := prompt.New(os.Stdin, os.Stderr)
			passphrase, err := p.Passphrase("Passphrase: ")
			if err != nil {
				return UserError(err.Error())
			}
			backend := agebackend.New(passphrase)
			pt, err := backend.Decrypt(ciphertext)
			if err != nil {
				return UserError("Incorrect passphrase.")
			}
			plaintext = pt
		}

		// Verify it's valid JSON
		var data map[string]string
		if err := json.Unmarshal(plaintext, &data); err != nil {
			return InternalError("corrupt store data")
		}

		// Re-exec as daemon
		self, err := os.Executable()
		if err != nil {
			return InternalError(fmt.Sprintf("finding executable: %v", err))
		}

		// Write plaintext to a temp file that only the daemon reads, then deletes
		tmpFile, err := os.CreateTemp("", ".secrets-agent-*")
		if err != nil {
			return InternalError("creating temp file for daemon")
		}
		tmpFile.Chmod(0600)
		tmpFile.Write(plaintext)
		tmpFile.Close()

		// Zero plaintext in this process
		for i := range plaintext {
			plaintext[i] = 0
		}

		daemonCmd := exec.Command(self, "agent", "--ttl", agentTTL)
		daemonCmd.Env = append(os.Environ(),
			"_SECRETS_AGENT_DAEMON=1",
			"_SECRETS_AGENT_DATA="+tmpFile.Name(),
			"_SECRETS_AGENT_TTL="+agentTTL,
		)
		daemonCmd.Stdout = nil
		daemonCmd.Stderr = nil

		if err := daemonCmd.Start(); err != nil {
			os.Remove(tmpFile.Name())
			return InternalError(fmt.Sprintf("starting daemon: %v", err))
		}

		// Wait for socket to be ready
		for i := 0; i < 100; i++ {
			if agent.IsRunning(sockPath) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}

		// Detach
		daemonCmd.Process.Release()

		fmt.Fprintf(os.Stdout, "export SECRETS_AGENT_SOCK=%s\n", sockPath)
		fmt.Fprintln(os.Stderr, "Agent started.")

		_ = ttl // TTL is passed to daemon via env
		return nil
	},
}

var agentStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running agent",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sockPath := agentSocketPath()
		if !agent.IsRunning(sockPath) {
			fmt.Fprintln(os.Stderr, "No agent running.")
			return nil
		}

		if err := agent.Stop(sockPath); err != nil {
			return InternalError(fmt.Sprintf("stopping agent: %v", err))
		}

		fmt.Fprintln(os.Stderr, "Agent stopped.")
		return nil
	},
}

// runDaemon is called by the re-exec'd child process.
func runDaemon(sockPath string) error {
	dataFile := os.Getenv("_SECRETS_AGENT_DATA")
	if dataFile == "" {
		return InternalError("daemon: missing data file")
	}

	plaintext, err := os.ReadFile(dataFile)
	if err != nil {
		return InternalError("daemon: reading data file")
	}
	os.Remove(dataFile) // delete immediately

	var data map[string]string
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return InternalError("daemon: corrupt data")
	}

	// Zero plaintext
	for i := range plaintext {
		plaintext[i] = 0
	}

	ttl, err := parseTTL(os.Getenv("_SECRETS_AGENT_TTL"))
	if err != nil {
		ttl = 8 * time.Hour
	}

	srv := agent.NewServer(data, sockPath)
	return srv.Start(ttl)
}

func parseTTL(s string) (time.Duration, error) {
	if s == "0" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

