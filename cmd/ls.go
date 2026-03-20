package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(lsCmd)
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all keys in the store",
	Long:  `List all key names sorted lexicographically, one per line. Never prints values.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStoreReadOnly()
		if err != nil {
			return err
		}
		defer s.Close()

		for _, key := range s.List() {
			fmt.Fprintln(os.Stdout, key)
		}
		return nil
	},
}
