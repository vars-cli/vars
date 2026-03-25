package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vars-cli/vars/internal/agent"
)

var (
	setOverwrite bool
	setSkip      bool
)

func init() {
	setCmd.Flags().BoolVar(&setOverwrite, "overwrite", false, "Overwrite existing key without prompting")
	setCmd.Flags().BoolVar(&setSkip, "skip", false, "Skip if key already exists")
	rootCmd.AddCommand(setCmd)
}

var setCmd = &cobra.Command{
	Use:   "set <key> [value]",
	Short: "Set a secret in the store",
	Long: `Write a key-value pair to the store. If value is omitted, prompts
interactively with echo disabled (preferred — inline values appear in
shell history).`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if setOverwrite && setSkip {
			return UserError("--overwrite and --skip are mutually exclusive")
		}

		key := args[0]

		if strings.ContainsRune(key, '~') {
			return UserError("key names may not contain '~' (reserved for history entries)")
		}

		var value string
		if len(args) == 2 {
			value = args[1]
		} else {
			v, err := stdinPrompter().Value("Value: ")
			if err != nil {
				return UserError(err.Error())
			}
			value = v
		}

		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()
		isTTY := term.IsTerminal(int(os.Stdin.Fd()))

		// Conflict resolution loop (handles rename re-checks)
		for {
			existing, getErr := agent.Get(sockPath, key)

			if getErr != nil {
				// New key — no conflict
				break
			}

			if existing == value {
				fmt.Fprintln(os.Stderr, "Already set, nothing to do.")
				return nil
			}

			// Key exists with a different value
			if setSkip {
				fmt.Fprintln(os.Stderr, "Skipped.")
				return nil
			}

			if setOverwrite {
				break
			}

			if !isTTY {
				return UserError("key already exists; use --overwrite or --skip")
			}

			fmt.Fprintf(os.Stderr, "\n%s already exists. New value will replace it.\n", key)
			choice, err := stdinPrompter().Line("[o]verwrite  [r]ename  [s]kip > ")
			if err != nil {
				return UserError(err.Error())
			}

			switch c := strings.ToLower(strings.TrimSpace(choice)); {
			case strings.HasPrefix(c, "o"):
				// proceed to set below
			case strings.HasPrefix(c, "r"):
				sfx, err := stdinPrompter().Line(fmt.Sprintf("Suffix (saved as %s_<suffix>): ", key))
				if err != nil {
					return UserError(err.Error())
				}
				sfx = strings.TrimSpace(strings.TrimPrefix(sfx, "_"))
				if sfx == "" {
					fmt.Fprintln(os.Stderr, "Suffix cannot be empty, skipping.")
					return nil
				}
				key = key + "_" + sfx
				continue // re-check renamed key
			default: // "s" or unrecognised
				fmt.Fprintln(os.Stderr, "Skipped.")
				return nil
			}
			break
		}

		if err := withPassphrase(func(passphrase string) error {
			return agent.Set(sockPath, key, value, passphrase)
		}); err != nil {
			return UserError(err.Error())
		}

		printManifestHint(key)
		fmt.Fprintln(os.Stderr, "Saved.")
		return nil
	},
}
