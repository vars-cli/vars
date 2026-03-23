package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
)

func init() {
	rootCmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a secret from the store",
	Long:  `Print one value to stdout with no trailing newline. Pipes cleanly.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		if err := ensureAgent(); err != nil {
			return err
		}

		val, err := agent.Get(agentSocketPath(), key)
		if err != nil {
			return UserError(fmt.Sprintf("Key %q not found in store.", key))
		}

		fmt.Fprint(os.Stdout, val)
		return nil
	},
}
