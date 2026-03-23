package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
)

func init() {
	rootCmd.AddCommand(mvCmd)
}

var mvCmd = &cobra.Command{
	Use:   "mv <from> <to>",
	Short: "Rename a key in the store",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}
		sockPath := agentSocketPath()

		if err := withPassphrase(func(pass string) error {
			return agent.Rename(sockPath, args[0], args[1], pass)
		}); err != nil {
			return UserError(err.Error())
		}

		fmt.Fprintf(os.Stderr, "Renamed %s → %s\n", args[0], args[1])
		return nil
	},
}
