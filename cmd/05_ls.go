package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
)

var lsAll bool

func init() {
	lsCmd.Flags().BoolVarP(&lsAll, "all", "a", false, "List all keys with full names")
	rootCmd.AddCommand(lsCmd)
}

var lsCmd = &cobra.Command{
	Use:   "ls [scope]",
	Short: "List keys in the store",
	Long: `List key names, one per line.

Without arguments, lists unscoped keys (no "/" in name).
With a scope name, lists keys under that scope (prefix stripped from output).
With --all, lists every key with its full name.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}

		keys, err := agent.List(agentSocketPath())
		if err != nil {
			return InternalError(err.Error())
		}

		switch {
		case lsAll:
			for _, key := range keys {
				fmt.Fprintln(os.Stdout, key)
			}
		case len(args) == 1:
			prefix := args[0] + "/"
			for _, key := range keys {
				if strings.HasPrefix(key, prefix) {
					fmt.Fprintln(os.Stdout, strings.TrimPrefix(key, prefix))
				}
			}
		default:
			for _, key := range keys {
				if !strings.Contains(key, "/") {
					fmt.Fprintln(os.Stdout, key)
				}
			}
		}

		return nil
	},
}

