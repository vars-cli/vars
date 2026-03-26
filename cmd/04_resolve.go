package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vars-cli/vars/internal/agent"
	"github.com/vars-cli/vars/internal/envfile"
	"github.com/vars-cli/vars/internal/format"
	"github.com/vars-cli/vars/internal/manifest"
)

var (
	resolveFish    bool
	resolveDotenv  bool
	resolveFile    string
	resolvePartial bool
	resolveProfile string
	resolveOrigins bool
)

func init() {
	resolveCmd.Flags().BoolVar(&resolveDotenv, "dotenv", false, "Output as KEY=value")
	resolveCmd.Flags().BoolVar(&resolveFish, "fish", false, "Output in fish shell format")
	resolveCmd.Flags().StringVarP(&resolveFile, "file", "f", ".vars.yaml", "Path to the manifest file")
	resolveCmd.Flags().BoolVar(&resolvePartial, "partial", false, "Skip missing keys instead of erroring")
	resolveCmd.Flags().StringVarP(&resolveProfile, "profile", "p", "", "Active profile name")
	resolveCmd.Flags().BoolVar(&resolveOrigins, "origins", false, "Append source comment to each line (vars, .env, not set)")
	rootCmd.AddCommand(resolveCmd)
}

var resolveCmd = &cobra.Command{
	Use:   "resolve",
	Short: "Resolve manifest keys and print as shell variables",
	Long: `Read .vars.yaml, resolve all variables against the store, and print as
shell-source-able lines to stdout.

  eval "$(vars resolve)"
  vars resolve --profile mainnet
  cat .env | vars resolve --partial

When stdin is a dotenv file, it is used as a fallback for missing store keys.
Non-manifest keys from stdin are passed through unchanged.
Store values always take priority over stdin values.

Resolution priority (per key):
  1. Active profile from .vars.local.yaml (personal override)
  2. Active profile from .vars.yaml
  3. mappings: from .vars.local.yaml
  4. mappings: from .vars.yaml
  5. Bare key (identity)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		formatter := format.Posix
		if resolveFish {
			formatter = format.Fish
		} else if resolveDotenv {
			formatter = format.Dotenv
		}

		localPath := filepath.Join(filepath.Dir(resolveFile), ".vars.local.yaml")
		vars, err := manifest.Resolve(resolveFile, localPath, resolveProfile)
		if err != nil {
			return UserError(err.Error())
		}

		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()

		// Parse stdin dotenv if piped
		var stdinEntries []envfile.Entry
		var stdinMap map[string]string
		if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
			stdinEntries, err = envfile.Parse(os.Stdin)
			if err != nil {
				return UserError("failed to parse stdin as dotenv: " + err.Error())
			}
			stdinMap = make(map[string]string, len(stdinEntries))
			for _, e := range stdinEntries {
				stdinMap[e.Key] = e.Value
			}
		}

		// Build set of manifest env names to exclude them from pass-through
		manifestKeys := make(map[string]bool, len(vars))
		for _, v := range vars {
			manifestKeys[v.EnvName] = true
		}

		type entry struct {
			envName string
			value   string
			source  string // "vars" | ".env" | "not set" | "" (pass-through)
		}
		var entries []entry

		// Resolve manifest keys: store first, stdin dotenv as fallback
		for _, v := range vars {
			val, lookupErr := resolveStoreKey(sockPath, v.StoreKey)
			if lookupErr != nil {
				if dotval, ok := stdinMap[v.EnvName]; ok {
					entries = append(entries, entry{v.EnvName, dotval, "stdin"})
					continue
				}
				if resolvePartial {
					if resolveOrigins {
						entries = append(entries, entry{v.EnvName, "", "not set"})
					} else {
						fmt.Fprintf(os.Stderr, "vars: %q not found (skipping)\n", v.StoreKey)
					}
					continue
				}
				if v.StoreKey == v.EnvName {
					return UserError(fmt.Sprintf("key %q not found in store", v.EnvName))
				}
				return UserError(fmt.Sprintf("key %q not found in store (mapped from %q)", v.StoreKey, v.EnvName))
			}
			entries = append(entries, entry{v.EnvName, val, "vars"})
		}

		// Pass through stdin dotenv keys not declared in the manifest
		for _, e := range stdinEntries {
			if !manifestKeys[e.Key] {
				entries = append(entries, entry{e.Key, e.Value, ""})
			}
		}

		for _, e := range entries {
			if e.source == "not set" {
				fmt.Fprintf(os.Stdout, "# %s  not set\n", e.envName)
			} else if resolveOrigins && e.source != "" {
				fmt.Fprintf(os.Stdout, "%s  # %s\n", formatter(e.envName, e.value), e.source)
			} else {
				fmt.Fprintln(os.Stdout, formatter(e.envName, e.value))
			}
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
