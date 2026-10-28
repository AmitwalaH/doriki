package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Inspect the embedded parser packs",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the embedded packs, one per line",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, name := range packNames() {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), name); err != nil {
					return err
				}
			}
			return nil
		},
	})
	cmd.AddCommand(newPackTestCmd())
	return cmd
}
