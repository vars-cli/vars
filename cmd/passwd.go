package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/crypto/age"
	"github.com/brickpop/secrets/internal/prompt"
)

func init() {
	rootCmd.AddCommand(passwdCmd)
}

var passwdCmd = &cobra.Command{
	Use:   "passwd",
	Short: "Change the store passphrase",
	Long: `Decrypt the store with the current passphrase, then re-encrypt with
a new one. An empty passphrase is allowed.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		p := prompt.New(os.Stdin, os.Stderr)
		newPass, err := p.PassphraseConfirm(
			"New passphrase (leave empty for no passphrase): ",
			"Confirm new passphrase: ",
		)
		if err != nil {
			return UserError(err.Error())
		}

		newBackend := age.New(newPass)
		if err := s.SaveWithBackend(newBackend); err != nil {
			return InternalError(err.Error())
		}

		fmt.Fprintln(os.Stderr, "Passphrase updated.")

		if stopped := tryStopAgent(); stopped {
			fmt.Fprintln(os.Stderr, "Agent stopped. Restart it to use the new passphrase.")
		}

		return nil
	},
}
