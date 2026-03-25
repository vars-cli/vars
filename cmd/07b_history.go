package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vars-cli/vars/internal/agent"
)

func init() {
	rootCmd.AddCommand(historyCmd)
}

var historyCmd = &cobra.Command{
	Use:   "history <key>",
	Short: "Show value history for a key (newest first)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}

		keys, values, err := agent.History(agentSocketPath(), args[0])
		if err != nil {
			return InternalError(err.Error())
		}

		for i, k := range keys {
			fmt.Printf("%s:\t%s\n", k, values[i])
		}
		return nil
	},
}
