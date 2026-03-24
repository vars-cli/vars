package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/brickpop/secrets/internal/agent"
)

func init() {
	rootCmd.AddCommand(scopesCmd)
}

var scopesCmd = &cobra.Command{
	Use:   "scopes",
	Short: "List all scopes (key prefixes) present in the store",
	Long: `List all unique scope prefixes found across all store keys.

A scope is any slash-delimited prefix in a key name. All levels are listed:
"main/dev/RPC_URL" contributes both "main" and "main/dev".
Bare keys without a "/" do not contribute to this list.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}

		keys, err := agent.List(agentSocketPath())
		if err != nil {
			return InternalError(err.Error())
		}

		seen := make(map[string]struct{})
		for _, k := range keys {
			parts := strings.Split(k, "/")
			// Each prefix up to (but not including) the last component is a scope.
			for i := 1; i < len(parts); i++ {
				seen[strings.Join(parts[:i], "/")] = struct{}{}
			}
		}

		scopes := make([]string, 0, len(seen))
		for s := range seen {
			scopes = append(scopes, s)
		}
		sort.Strings(scopes)

		for _, s := range scopes {
			fmt.Println(s)
		}
		return nil
	},
}
