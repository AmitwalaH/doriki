// Command gen writes the JSON Schemas in schema/ from the Go types (`make schema`, run from
// the repository root).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AmitwalaH/doriki/schema"
)

func main() {
	files, err := schema.Generate(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join("schema", name), b, 0o644); err != nil { // #nosec G306 -- committed, world-readable schema files
			fmt.Fprintln(os.Stderr, "schema:", err)
			os.Exit(1)
		}
	}
}
