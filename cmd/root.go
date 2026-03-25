// Package cmd implements the CLI commands for vars.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	cobra.EnableCommandSorting = false
}

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "vars",
	Short: "A central vault for environment variables",
	Long: `vars is a single encrypted store for environment variables,
shared across multiple projects. It replaces scattered .env files with
a single age-encrypted store.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command. Called from main.
func Execute() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("vars {{.Version}}\n")
	if err := rootCmd.Execute(); err != nil {
		// Determine exit code: ExitError for user errors (1), default 2
		if exitErr, ok := err.(*ExitError); ok {
			fmt.Fprintln(os.Stderr, exitErr.Error())
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
}

// ExitError is an error with a specific exit code.
type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string {
	return e.Message
}

// UserError returns an ExitError with exit code 1 (user error).
func UserError(msg string) *ExitError {
	return &ExitError{Code: 1, Message: msg}
}

// InternalError returns an ExitError with exit code 2 (internal error).
func InternalError(msg string) *ExitError {
	return &ExitError{Code: 2, Message: msg}
}
