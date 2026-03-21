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
	exportFormat  string
	exportFile    string
	exportPartial bool
)

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "posix", "Output format: posix, fish, dotenv")
	exportCmd.Flags().StringVarP(&exportFile, "file", "f", ".secrets.yaml", "Path to manifest file")
	exportCmd.Flags().BoolVar(&exportPartial, "partial", false, "Export empty values for missing keys instead of erroring")
	rootCmd.AddCommand(exportCmd)
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export secrets as shell variables",
	Long: `Read .secrets.yaml, apply map file remappings, resolve all variables
against the store, and print shell-source-able lines to stdout.

  eval "$(secrets export)"
  secrets export --format fish | source`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		formatter, err := format.Get(exportFormat)
		if err != nil {
			return UserError(err.Error())
		}

		vars, err := manifest.Resolve(exportFile)
		if err != nil {
			return UserError(err.Error())
		}

		sockPath, err := ensureAgent()
		if err != nil {
			return err
		}

		type exportEntry struct {
			envName string
			value   string
		}
		var entries []exportEntry

		for _, v := range vars {
			val, err := agent.Get(sockPath, v.StoreKey)
			if err != nil {
				if exportPartial {
					fmt.Fprintf(os.Stderr, "Warning: %q not found in store, exporting as empty.\n", v.StoreKey)
					entries = append(entries, exportEntry{v.EnvName, ""})
					continue
				}
				return UserError(fmt.Sprintf("Export failed: key %q (required by .secrets.yaml) is not in the store.", v.StoreKey))
			}
			entries = append(entries, exportEntry{v.EnvName, val})
		}

		for _, e := range entries {
			fmt.Fprintln(os.Stdout, formatter(e.envName, e.value))
		}

		return nil
	},
}
