package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
	"github.com/brickpop/secrets/internal/format"
	"github.com/brickpop/secrets/internal/manifest"
)

var (
	resolveFormat  string
	resolveFile    string
	resolvePartial bool
	resolveProfile string
)

func init() {
	resolveCmd.Flags().StringVar(&resolveFormat, "format", "posix", "Output format: posix, fish, dotenv")
	resolveCmd.Flags().StringVarP(&resolveFile, "file", "f", ".secrets.yaml", "Path to manifest file")
	resolveCmd.Flags().BoolVar(&resolvePartial, "partial", false, "Export empty values for missing keys instead of erroring")
	resolveCmd.Flags().StringVarP(&resolveProfile, "profile", "p", "", "Active profile name")
	rootCmd.AddCommand(resolveCmd)
}

var resolveCmd = &cobra.Command{
	Use:   "resolve",
	Short: "Resolve manifest keys and print as shell variables",
	Long: `Read .secrets.yaml, resolve all variables against the store, and print
shell-source-able lines to stdout.

  eval "$(secrets resolve)"
  secrets resolve --format fish | source
  secrets resolve --profile mainnet

Resolution priority (per key):
  1. Active profile from .secrets.local.yaml (personal override)
  2. Active profile from .secrets.yaml
  3. mappings: from .secrets.local.yaml
  4. mappings: from .secrets.yaml
  5. Bare key (identity)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		formatter, err := format.Get(resolveFormat)
		if err != nil {
			return UserError(err.Error())
		}

		localPath := filepath.Join(filepath.Dir(resolveFile), ".secrets.local.yaml")
		vars, err := manifest.Resolve(resolveFile, localPath, resolveProfile)
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
			val, lookupErr := resolveStoreKey(sockPath, v.StoreKey)
			if lookupErr != nil {
				if resolvePartial {
					fmt.Fprintf(os.Stderr, "Warning: %q not found in store, exporting as empty.\n", v.StoreKey)
					entries = append(entries, entry{v.EnvName, ""})
					continue
				}
				if v.StoreKey == v.EnvName {
					return UserError(fmt.Sprintf("Cannot resolve: key %q (required by .secrets.yaml) is not in the store.", v.EnvName))
				}
				return UserError(fmt.Sprintf("Cannot resolve: key %q (mapped from %q) is not in the store.", v.StoreKey, v.EnvName))
			}
			entries = append(entries, entry{v.EnvName, val})
		}

		for _, e := range entries {
			fmt.Fprintln(os.Stdout, formatter(e.envName, e.value))
		}

		return nil
	},
}

// resolveStoreKey tries the given key, then falls back by stripping successive
// scope prefixes: "main/dev/RPC_URL" → "dev/RPC_URL" → "RPC_URL".
func resolveStoreKey(sockPath, key string) (string, error) {
	for {
		val, err := agent.Get(sockPath, key)
		if err == nil {
			return val, nil
		}
		i := strings.IndexByte(key, '/')
		if i < 0 {
			return "", err
		}
		key = key[i+1:]
	}
}
