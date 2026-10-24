// Command doriki profiles why an LLM inference pod was slow to start. It also runs as the
// kubectl plugin `kubectl doriki`.
package main

import (
	"fmt"
	"os"

	"github.com/AmitwalaH/doriki/internal/cli"
)

// exitUsage is DESIGN.md section 7.3's usage-error code. In M0 the only possible error is a
// bad command or flag; exit 1 is reserved for reports with open phases.
const exitUsage = 64

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "doriki:", err)
		os.Exit(exitUsage)
	}
}
