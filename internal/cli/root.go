// Package cli defines the doriki commands, parses flags and selects output. It is the only
// package that imports everything else (DESIGN.md section 8.3).
package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the doriki command tree.
func NewRootCmd() *cobra.Command {
	var f profileFlags
	root := &cobra.Command{
		Use:   "doriki [pod/]<name> | deploy/<name> | sts/<name> [-n namespace] [-o text|json]",
		Short: "Show which startup phase made an LLM inference pod slow, and what to do about it",
		// Positional arguments are the pod (or workload) to profile, or with --from-files the files.
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runProfile(cmd, args, &f) },
		// main prints the error once; cobra's own usage dump on every error is noise.
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	addProfileFlags(root, &f)
	root.AddCommand(newVersionCmd(), newPackCmd())
	return root
}

// Execute runs the root command with the process arguments.
func Execute() error {
	return NewRootCmd().Execute()
}
