package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/crypto/age"
	"github.com/brickpop/secrets/internal/store"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the secret store",
	Long:  `Creates a new encrypted store at ~/.secrets/store.age (or SECRETS_STORE_DIR).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if store.Exists() {
			fmt.Fprintf(os.Stderr, "Store already exists at %s\n", store.FilePath())
			return nil
		}

		passphrase, err := stdinPrompter().PassphraseConfirm(
			"Passphrase (leave empty for no passphrase): ",
			"Confirm passphrase: ",
		)
		if err != nil {
			return UserError(err.Error())
		}

		backend := age.New(passphrase)
		if err := store.Init(backend); err != nil {
			return InternalError(err.Error())
		}

		if passphrase == "" {
			fmt.Fprintln(os.Stderr, "Store created with no passphrase.")
		}
		fmt.Fprintf(os.Stderr, "Store created at %s\n", store.FilePath())

		if _, err := launchDaemon(map[string]string{}, passphrase, defaultAgentTTL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not start agent: %v\n", err)
		}

		return nil
	},
}
