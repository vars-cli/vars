// Package cmd implements the CLI commands for secrets.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "secrets",
	Short: "A central vault for environment variable secrets",
	Long: `secrets is a single encrypted store for environment variable secrets,
shared across multiple projects. It replaces scattered .env files with
a single age-encrypted store and per-project manifests.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command. Called from main.
func Execute() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("secrets {{.Version}}\n")
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
