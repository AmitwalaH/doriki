// Package packs holds the parser packs embedded in the doriki binary: one YAML file per engine
// version, each with a real fixture under testdata/. Packs are data, not Go (DESIGN.md
// section 5).
package packs

// Names returns the embedded packs as "<engine>/<version>", sorted. None are embedded until
// M1 adds the first pack, so it returns nil; an empty //go:embed pattern would not compile.
func Names() []string {
	return nil
}
