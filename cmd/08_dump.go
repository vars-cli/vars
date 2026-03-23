package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
	"github.com/brickpop/secrets/internal/format"
)

var dumpFormat string

func init() {
	dumpCmd.Flags().StringVar(&dumpFormat, "format", "posix", "Output format: posix, fish, dotenv")
	rootCmd.AddCommand(dumpCmd)
}

var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Dump all secrets from the store",
	Long: `Print all key/value pairs from the store. No manifest involved.
Intended for debugging and migration only.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		formatter, err := format.Get(dumpFormat)
		if err != nil {
			return UserError(err.Error())
		}

		fmt.Fprintln(os.Stderr, "Warning: dumping all secrets from store.")

		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()
		keys, err := agent.List(sockPath)
		if err != nil {
			return InternalError(err.Error())
		}

		for _, key := range keys {
			val, _ := agent.Get(sockPath, key)
			fmt.Fprintln(os.Stdout, formatter(key, val))
		}

		return nil
	},
}
