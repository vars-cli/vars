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
	importReplace bool
	importSkip    bool
)

func init() {
	importCmd.Flags().BoolVar(&importReplace, "replace", false, "Replace conflicting keys without confirmation")
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
		if importReplace && importSkip {
			return UserError("--replace and --skip are mutually exclusive")
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

		type pendingItem struct {
			key   string
			value string
		}
		var pending []pendingItem
		var imported, replaced, skipped int

	entryLoop:
		for _, e := range entries {
			key := e.Key
			value := e.Value

			for {
				existing, getErr := agent.Get(sockPath, key)

				if getErr != nil {
					// New key
					pending = append(pending, pendingItem{key, value})
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
					fmt.Fprintf(os.Stderr, "Skipped %s\n", key)
					skipped++
					continue entryLoop
				}

				if importReplace {
					pending = append(pending, pendingItem{key, value})
					replaced++
					continue entryLoop
				}

				// Interactive mode
				if !isTTY {
					return UserError("conflicting keys found; use --replace or --skip to resolve non-interactively")
				}

				fmt.Fprintf(os.Stderr, "\n%s already exists.\n  current:  %s\n  imported: %s\n", key, existing, value)
				choice, err := stdinPrompter().Line("[r]eplace  [n]ew name  [s]kip > ")
				if err != nil {
					return UserError(err.Error())
				}

				switch c := strings.ToLower(strings.TrimSpace(choice)); {
				case strings.HasPrefix(c, "r"):
					pending = append(pending, pendingItem{key, value})
					replaced++
					continue entryLoop

				case strings.HasPrefix(c, "n"):
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

		if len(pending) > 0 {
			items := make([]agent.SetItem, len(pending))
			for i, p := range pending {
				items[i] = agent.SetItem{Key: p.key, Value: p.value}
			}
			if err := agent.Set(sockPath, items); err != nil {
				return UserError(err.Error())
			}
		}

		fmt.Fprintf(os.Stderr, "Imported %d, replaced %d, skipped %d.\n", imported, replaced, skipped)
		return nil
	},
}
