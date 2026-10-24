package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AmitwalaH/doriki/packs"
)

// MinKubeVersion is the oldest Kubernetes minor doriki supports. The README states it too.
const MinKubeVersion = "1.29"

// Set at build time with -ldflags -X by the Makefile (and goreleaser from M3). A plain
// `go build` or `go run` keeps these defaults.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// packNames is a variable so tests can swap in a sentinel pack list.
var packNames = packs.Names

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, commit, build date and embedded packs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			list := "none"
			if names := packNames(); len(names) > 0 {
				list = strings.Join(names, ", ")
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "version: %s\ncommit: %s\ndate: %s\npacks: %s\n",
				version, commit, date, list)
			return err
		},
	}
}
