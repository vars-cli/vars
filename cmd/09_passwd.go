package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
)

func init() {
	rootCmd.AddCommand(passwdCmd)
}

var passwdCmd = &cobra.Command{
	Use:   "passwd",
	Short: "Change the store passphrase",
	Long: `Re-encrypt the store with a new passphrase. An empty passphrase
is allowed. The agent updates its internal state — no restart needed.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()

		// Prompt for new passphrase first
		newPass, err := stdinPrompter().PassphraseConfirm(
			"New passphrase (leave empty for no passphrase): ",
			"Confirm new passphrase: ",
		)
		if err != nil {
			return UserError(err.Error())
		}

		// Send to agent — withPassphrase handles the current passphrase
		// via trial approach (empty first, prompt if required).
		err = withPassphrase(func(oldPass string) error {
			return agent.Passwd(sockPath, oldPass, newPass)
		})
		if err != nil {
			return UserError(err.Error())
		}

		fmt.Fprintln(os.Stderr, "Passphrase updated.")
		return nil
	},
}
