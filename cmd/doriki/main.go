// Command doriki profiles why an LLM inference pod was slow to start. It also runs as the
// kubectl plugin `kubectl doriki`.
package main

import (
	"fmt"
	"os"

	"github.com/AmitwalaH/doriki/internal/cli"
)

// main only maps the result to an exit code (DESIGN.md section 7.3); cli.Exit decides it.
func main() {
	code, msg := cli.Exit(cli.Execute())
	if msg != "" {
		fmt.Fprintln(os.Stderr, "doriki:", msg)
	}
	os.Exit(code)
}
