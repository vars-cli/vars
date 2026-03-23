package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
	agebackend "github.com/brickpop/secrets/internal/crypto/age"
	"github.com/brickpop/secrets/internal/store"
)

// daemonPayload is the JSON structure written to the temp file for the daemon.
type daemonPayload struct {
	Passphrase string            `json:"passphrase"`
	Data       map[string]string `json:"data"`
}

var agentTTL string

func init() {
	agentCmd.Flags().StringVar(&agentTTL, "ttl", "8h", "Agent lifetime (e.g. 30m, 5h, 10d, 0 for unlimited)")
	agentCmd.AddCommand(agentStopCmd)
	rootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start the background agent",
	Long: `Start a background agent that holds the decrypted store in memory.
Most commands auto-start the agent transparently. Use this command
to set an explicit TTL, or to update the TTL of a running agent.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sockPath := agentSocketPath()

		if agent.IsRunning(sockPath) {
			if !cmd.Flags().Changed("ttl") {
				fmt.Fprintln(os.Stderr, "Agent already running.")
				return nil
			}
			ttl, err := parseTTLSeconds(agentTTL)
			if err != nil {
				return UserError(fmt.Sprintf("Invalid TTL: %v", err))
			}
			if err := agent.SetAgentTTL(sockPath, ttl); err != nil {
				return InternalError(fmt.Sprintf("updating agent TTL: %v", err))
			}
			fmt.Fprintln(os.Stderr, "Agent TTL updated.")
			return nil
		}

		// Internal daemon mode (re-exec'd child)
		if os.Getenv("_SECRETS_AGENT_DAEMON") == "1" {
			return runDaemon(sockPath)
		}

		ttl, err := parseTTLSeconds(agentTTL)
		if err != nil {
			return UserError(fmt.Sprintf("Invalid TTL: %v", err))
		}

		if _, err := startAgent(ttl); err != nil {
			return err
		}

		fmt.Fprintln(os.Stderr, "Agent started.")
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

// startAgent decrypts the store, spawns the daemon, and waits for the socket.
// Returns the socket path. Used by both `secrets agent` and ensureAgent().
func startAgent(ttl int64) (string, error) {
	if !store.Exists() {
		return "", UserError("No store found. Run 'secrets init' to create one.")
	}

	ciphertext, err := os.ReadFile(store.FilePath())
	if err != nil {
		return "", InternalError(fmt.Sprintf("reading store: %v", err))
	}

	// Print permission warnings
	for _, w := range store.CheckPermissions() {
		fmt.Fprintln(os.Stderr, w)
	}

	// Decrypt: trial empty passphrase, then prompt
	var plaintext []byte
	var passphrase string

	if pt, ok := agebackend.TrialDecryptEmpty(ciphertext); ok {
		plaintext = pt
		passphrase = ""
	} else {
		pass, err := stdinPrompter().Passphrase("Passphrase: ")
		if err != nil {
			return "", UserError(err.Error())
		}
		backend := agebackend.New(pass)
		pt, err := backend.Decrypt(ciphertext)
		if err != nil {
			return "", UserError("Incorrect passphrase.")
		}
		plaintext = pt
		passphrase = pass
	}

	// Verify valid JSON
	var data map[string]string
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return "", InternalError("corrupt store data")
	}
	// Zero plaintext before handing off
	for i := range plaintext {
		plaintext[i] = 0
	}

	return launchDaemon(data, passphrase, ttl)
}

// launchDaemon spawns the agent daemon with already-decrypted data.
// Used by startAgent (after decrypting from disk) and init (data already in memory).
func launchDaemon(data map[string]string, passphrase string, ttl int64) (string, error) {
	sockPath := agentSocketPath()

	// Build daemon payload with passphrase
	payload := daemonPayload{Passphrase: passphrase, Data: data}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", InternalError("serializing daemon payload")
	}

	// Write payload to temp file
	tmpFile, err := os.CreateTemp("", ".secrets-agent-*")
	if err != nil {
		return "", InternalError("creating temp file for daemon")
	}
	tmpFile.Chmod(0600)
	tmpFile.Write(payloadBytes)
	tmpFile.Close()

	// Zero payload bytes
	for i := range payloadBytes {
		payloadBytes[i] = 0
	}

	// Re-exec as daemon
	self, err := os.Executable()
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", InternalError(fmt.Sprintf("finding executable: %v", err))
	}

	ttlStr := strconv.FormatInt(ttl, 10)

	daemonCmd := exec.Command(self, "agent", "--ttl", ttlStr)
	daemonCmd.Env = append(os.Environ(),
		"_SECRETS_AGENT_DAEMON=1",
		"_SECRETS_AGENT_DATA="+tmpFile.Name(),
	)
	daemonCmd.Stdout = nil
	daemonCmd.Stderr = nil

	if err := daemonCmd.Start(); err != nil {
		os.Remove(tmpFile.Name())
		return "", InternalError(fmt.Sprintf("starting daemon: %v", err))
	}

	// Wait for socket
	for i := 0; i < 100; i++ {
		if agent.IsRunning(sockPath) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	daemonCmd.Process.Release()
	return sockPath, nil
}

// runDaemon is called by the re-exec'd child process.
func runDaemon(sockPath string) error {
	dataFile := os.Getenv("_SECRETS_AGENT_DATA")
	if dataFile == "" {
		return InternalError("daemon: missing data file")
	}

	rawPayload, err := os.ReadFile(dataFile)
	if err != nil {
		return InternalError("daemon: reading data file")
	}
	os.Remove(dataFile)

	var payload daemonPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return InternalError("daemon: corrupt payload")
	}

	// Zero raw payload
	for i := range rawPayload {
		rawPayload[i] = 0
	}

	ttl, err := parseTTLSeconds(agentTTL)
	if err != nil {
		ttl = defaultAgentTTL
	}

	backend := agebackend.New(payload.Passphrase)
	srv := agent.NewServer(payload.Data, sockPath, payload.Passphrase, backend, agebackend.NewBackend, store.Dir())
	return srv.Start(time.Duration(ttl) * time.Second)
}

// parseTTLSeconds parses a TTL string into seconds.
// Accepts: plain integer (seconds), or suffixed values: s, m, h (via time.ParseDuration), d (days).
// 0 means infinite. Negative values are not allowed (use agent stop to stop).
func parseTTLSeconds(s string) (int64, error) {
	if s == "0" {
		return 0, nil
	}
	// Days suffix (not supported by time.ParseDuration)
	if strings.HasSuffix(s, "d") {
		n, err := strconv.ParseInt(strings.TrimSuffix(s, "d"), 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid TTL %q", s)
		}
		return n * 86400, nil
	}
	// Plain integer = seconds
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("TTL must be >= 0")
		}
		return n, nil
	}
	// Standard duration (s, m, h)
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid TTL %q", s)
	}
	if d < 0 {
		return 0, fmt.Errorf("TTL must be >= 0")
	}
	return int64(d / time.Second), nil
}
