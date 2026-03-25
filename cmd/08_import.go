package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vars-cli/vars/internal/agent"
	"github.com/vars-cli/vars/internal/envfile"
)

var (
	importOverwrite bool
	importSkip      bool
)

func init() {
	importCmd.Flags().BoolVar(&importOverwrite, "overwrite", false, "Overwrite conflicting keys without prompting")
	importCmd.Flags().BoolVar(&importSkip, "skip", false, "Skip conflicting keys without prompting")
	rootCmd.AddCommand(importCmd)
}

var importCmd = &cobra.Command{
	Use:   "import [scope] <file>",
	Short: "Import keys from a .env file",
	Long: `Import key-value pairs from a .env file into the store.

Without a scope, keys are imported into the default scope.
With a scope, keys are prefixed: vars import prod .env → prod/KEY.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if importOverwrite && importSkip {
			return UserError("--overwrite and --skip are mutually exclusive")
		}

		var scope, filePath string
		if len(args) == 2 {
			scope = args[0]
			filePath = args[1]
		} else {
			filePath = args[0]
		}

		f, err := os.Open(filePath)
		if err != nil {
			return UserError(fmt.Sprintf("opening file: %v", err))
		}
		defer f.Close()

		entries, err := envfile.Parse(f)
		if err != nil {
			return UserError(fmt.Sprintf("parsing file: %v", err))
		}
		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "No entries found.")
			return nil
		}

		// Apply scope prefix
		if scope != "" {
			for i := range entries {
				entries[i].Key = scope + "/" + entries[i].Key
			}
		}

		if err := ensureAgent(); err != nil {
			return err
		}
		sockPath := agentSocketPath()

		isTTY := term.IsTerminal(int(os.Stdin.Fd()))

		// doSet tries with the cached passphrase (initially ""), prompting once on
		// ErrPassphraseRequired and caching the result for subsequent overwrites.
		var passphrase string
		passphraseObtained := false
		doSet := func(key, value string) error {
			err := agent.Set(sockPath, key, value, passphrase)
			if err == nil {
				return nil
			}
			if !strings.Contains(err.Error(), agent.ErrPassphraseRequired) {
				return err
			}
			if !passphraseObtained {
				p, promptErr := stdinPrompter().Passphrase("Passphrase: ")
				if promptErr != nil {
					return promptErr
				}
				passphrase = p
				passphraseObtained = true
			}
			return agent.Set(sockPath, key, value, passphrase)
		}

		var imported, overwritten, skipped int

	entryLoop:
		for _, e := range entries {
			key := e.Key
			value := e.Value

			for {
				existing, getErr := agent.Get(sockPath, key)

				if getErr != nil {
					// Key does not exist — import freely
					if setErr := agent.Set(sockPath, key, value, ""); setErr != nil {
						return InternalError(fmt.Sprintf("setting %s: %v", key, setErr))
					}
					imported++
					continue entryLoop
				}

				if existing == value {
					// Same value — idempotent, skip silently
					skipped++
					continue entryLoop
				}

				// Conflict: key exists with a different value
				if importSkip {
					fmt.Fprintf(os.Stderr, "Skipped %s (already exists)\n", key)
					skipped++
					continue entryLoop
				}

				if importOverwrite {
					if err := doSet(key, value); err != nil {
						return InternalError(fmt.Sprintf("overwriting %s: %v", key, err))
					}
					overwritten++
					continue entryLoop
				}

				// Interactive mode
				if !isTTY {
					return UserError("conflicting keys found; use --overwrite or --skip to resolve non-interactively")
				}

				fmt.Fprintf(os.Stderr, "\n%s already exists\n  current:  %s\n  imported: %s\n", key, existing, value)
				choice, err := stdinPrompter().Line("[o]verwrite  [r]ename  [s]kip > ")
				if err != nil {
					return UserError(err.Error())
				}

				switch c := strings.ToLower(strings.TrimSpace(choice)); {
				case strings.HasPrefix(c, "o"):
					if err := doSet(key, value); err != nil {
						return InternalError(fmt.Sprintf("overwriting %s: %v", key, err))
					}
					overwritten++
					continue entryLoop

				case strings.HasPrefix(c, "r"):
					sfx, err := stdinPrompter().Line(fmt.Sprintf("Suffix (saved as %s_<suffix>): ", key))
					if err != nil {
						return UserError(err.Error())
					}
					sfx = strings.TrimSpace(strings.TrimPrefix(sfx, "_"))
					if sfx == "" {
						fmt.Fprintln(os.Stderr, "Suffix cannot be empty, skipping.")
						skipped++
						continue entryLoop
					}
					key = key + "_" + sfx
					// Re-check the renamed key for conflicts

				default: // includes "s" and anything unrecognised
					fmt.Fprintf(os.Stderr, "Skipped %s\n", key)
					skipped++
					continue entryLoop
				}
			}
		}

		fmt.Fprintf(os.Stderr, "Imported: %d  Overwritten: %d  Skipped: %d\n", imported, overwritten, skipped)
		return nil
	},
}
