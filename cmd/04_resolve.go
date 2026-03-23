package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
	"github.com/brickpop/secrets/internal/format"
	"github.com/brickpop/secrets/internal/manifest"
)

var (
	resolveFormat  string
	resolveFile    string
	resolvePartial bool
)

func init() {
	resolveCmd.Flags().StringVar(&resolveFormat, "format", "posix", "Output format: posix, fish, dotenv")
	resolveCmd.Flags().StringVarP(&resolveFile, "file", "f", ".secrets.yaml", "Path to manifest file")
	resolveCmd.Flags().BoolVar(&resolvePartial, "partial", false, "Export empty values for missing keys instead of erroring")
	rootCmd.AddCommand(resolveCmd)
}

var resolveCmd = &cobra.Command{
	Use:   "resolve",
	Short: "Resolve manifest keys and print as shell variables",
	Long: `Read .secrets.yaml, apply map file remappings, resolve all variables
against the store, and print shell-source-able lines to stdout.

  eval "$(secrets resolve)"
  secrets resolve --format fish | source`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		formatter, err := format.Get(resolveFormat)
		if err != nil {
			return UserError(err.Error())
		}

		vars, err := manifest.Resolve(resolveFile)
		if err != nil {
			return UserError(err.Error())
		}

		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()

		type entry struct {
			envName string
			value   string
		}
		var entries []entry

		for _, v := range vars {
			val, err := agent.Get(sockPath, v.StoreKey)
			if err != nil {
				if resolvePartial {
					fmt.Fprintf(os.Stderr, "Warning: %q not found in store, exporting as empty.\n", v.StoreKey)
					entries = append(entries, entry{v.EnvName, ""})
					continue
				}
				return UserError(fmt.Sprintf("Resolve failed: key %q (required by .secrets.yaml) is not in the store.", v.StoreKey))
			}
			entries = append(entries, entry{v.EnvName, val})
		}

		for _, e := range entries {
			fmt.Fprintln(os.Stdout, formatter(e.envName, e.value))
		}

		return nil
	},
}
