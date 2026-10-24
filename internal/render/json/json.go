// Package json writes the report as JSON, the schema in DESIGN.md section 4.4 and nothing
// else (section 7.5). It is pure: it writes only to the writer it is given.
package json

import (
	stdjson "encoding/json"
	"io"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Write encodes r, indented by two spaces and followed by a newline. Field omission and
// "never null" are the model's JSON tags; this only fixes the layout. HTML escaping is off so
// log lines and image names read as written.
func Write(w io.Writer, r model.Report) error {
	enc := stdjson.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}
