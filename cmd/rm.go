package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/prompt"
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

		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		// Check key exists before prompting
		if _, err := s.Get(key); err != nil {
			return UserError(fmt.Sprintf("Key %q not found in store.", key))
		}

		if !rmForce {
			p := prompt.New(os.Stdin, os.Stderr)
			ok, err := p.Confirm(fmt.Sprintf("Remove %s? [y/N] ", key))
			if err != nil {
				return UserError(err.Error())
			}
			if !ok {
				return nil
			}
		}

		if err := s.Delete(key); err != nil {
			return UserError(fmt.Sprintf("Key %q not found in store.", key))
		}
		if err := s.Save(); err != nil {
			return InternalError(err.Error())
		}

		fmt.Fprintln(os.Stderr, "Removed.")
		return nil
	},
}
