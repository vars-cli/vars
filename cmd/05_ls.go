package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
)

func init() {
	rootCmd.AddCommand(lsCmd)
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all keys in the store",
	Long:  `List all key names sorted lexicographically, one per line. Never prints values.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}

		keys, err := agent.List(agentSocketPath())
		if err != nil {
			return InternalError(err.Error())
		}

		for _, key := range keys {
			fmt.Fprintln(os.Stdout, key)
		}
		return nil
	},
}
