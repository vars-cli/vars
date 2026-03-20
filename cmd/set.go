package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/prompt"
)

func init() {
	rootCmd.AddCommand(setCmd)
}

var setCmd = &cobra.Command{
	Use:   "set <key> [value]",
	Short: "Set a secret in the store",
	Long: `Write a key-value pair to the store. If value is omitted, prompts
interactively with echo disabled (preferred — inline values appear in
shell history).`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		var value string
		if len(args) == 2 {
			value = args[1]
		} else {
			p := prompt.New(os.Stdin, os.Stderr)
			v, err := p.Value("Value: ")
			if err != nil {
				return UserError(err.Error())
			}
			value = v
		}

		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		s.Set(key, []byte(value))
		if err := s.Save(); err != nil {
			return InternalError(err.Error())
		}

		printManifestHint(key)

		fmt.Fprintln(os.Stderr, "Saved.")
		return nil
	},
}
