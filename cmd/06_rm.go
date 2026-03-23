package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
)

func init() {
	rmCmd.Flags().BoolVarP(&rmForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(rmCmd)
}

var rmForce bool

var rmCmd = &cobra.Command{
	Use:   "rm <key>",
	Short: "Remove a secret from the store",
	Long:  `Delete a key from the store. Prompts for confirmation unless --force is used.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()

		// Verify key exists via agent before prompting
		if _, err := agent.Get(sockPath, key); err != nil {
			return UserError(fmt.Sprintf("Key %q not found in store.", key))
		}

		if !rmForce {
			ok, err := stdinPrompter().Confirm(fmt.Sprintf("Remove %s? [y/N] ", key))
			if err != nil {
				return UserError(err.Error())
			}
			if !ok {
				return nil
			}
		}

		err := withPassphrase(func(passphrase string) error {
			return agent.Delete(sockPath, key, passphrase)
		})
		if err != nil {
			return UserError(err.Error())
		}

		fmt.Fprintln(os.Stderr, "Removed.")
		return nil
	},
}
