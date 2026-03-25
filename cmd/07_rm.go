package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vars-cli/vars/internal/agent"
)

func init() {
	rmCmd.Flags().BoolVarP(&rmForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(rmCmd)
}

var rmForce bool

var rmCmd = &cobra.Command{
	Use:   "rm <key> [key...]",
	Short: "Remove one or more entries from the store",
	Long:  `Delete keys from the store. Prompts for confirmation unless --force is used.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()

		// Verify all keys exist before prompting
		for _, key := range args {
			if _, err := agent.Get(sockPath, key); err != nil {
				return UserError(fmt.Sprintf("Key %q not found in store.", key))
			}
		}

		if !rmForce {
			var prompt string
			if len(args) == 1 {
				prompt = fmt.Sprintf("Remove %s? [y/N] ", args[0])
			} else {
				prompt = fmt.Sprintf("Remove %d keys (%s)? [y/N] ", len(args), joinKeys(args))
			}
			ok, err := stdinPrompter().Confirm(prompt)
			if err != nil {
				return UserError(err.Error())
			}
			if !ok {
				return nil
			}
		}

		err := withPassphrase(func(passphrase string) error {
			for _, key := range args {
				if err := agent.Delete(sockPath, key, passphrase); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return UserError(err.Error())
		}

		if len(args) == 1 {
			fmt.Fprintln(os.Stderr, "Removed.")
		} else {
			fmt.Fprintf(os.Stderr, "Removed %d keys.\n", len(args))
		}
		return nil
	},
}

func joinKeys(keys []string) string {
	if len(keys) <= 3 {
		result := keys[0]
		for _, k := range keys[1:] {
			result += ", " + k
		}
		return result
	}
	return fmt.Sprintf("%s, %s, ... +%d more", keys[0], keys[1], len(keys)-2)
}
