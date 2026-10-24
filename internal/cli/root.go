// Package cli defines the doriki commands, parses flags and selects output. It is the only
// package that imports everything else (DESIGN.md section 8.3).
package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the doriki command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "doriki",
		Short: "Show which startup phase made an LLM inference pod slow, and what to do about it",
		// main prints the error once; cobra's own usage dump on every error is noise.
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCmd())
	return root
}

// Execute runs the root command with the process arguments.
func Execute() error {
	return NewRootCmd().Execute()
}
