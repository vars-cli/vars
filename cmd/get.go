package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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

		s, err := openStoreReadOnly()
		if err != nil {
			return err
		}
		defer s.Close()

		val, err := s.Get(key)
		if err != nil {
			return UserError(fmt.Sprintf("Key %q not found in store.", key))
		}

		fmt.Fprint(os.Stdout, string(val))
		return nil
	},
}
