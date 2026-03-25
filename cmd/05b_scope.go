package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vars-cli/vars/internal/agent"
)

func init() {
	scopeCmd.AddCommand(scopeLsCmd)
	rootCmd.AddCommand(scopeCmd)
}

var scopeCmd = &cobra.Command{
	Use:   "scope",
	Short: "Manage key scopes",
}

var scopeLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all scope prefixes in the store",
	Long: `List all unique scope prefixes found across all store keys, at all levels.

"main/dev/RPC_URL" contributes both "main" and "main/dev".`,
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
