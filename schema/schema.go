// Package schema generates the JSON Schemas in this directory from the Go types in
// internal/model, which are the single encoding of the report, manifest and pack formats
// (PROJECT.md section 6). `make schema` writes them; a test fails when they are stale.
package schema

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"reflect"

	"github.com/invopop/jsonschema"

	"github.com/AmitwalaH/doriki/internal/model"
)

const module = "github.com/AmitwalaH/doriki"

// BaseURL is where the committed schemas are published; each schema's $id is BaseURL plus its
// file name.
const BaseURL = "https://raw.githubusercontent.com/AmitwalaH/doriki/main/schema/"

type source struct {
	value any
	tag   string // the struct tag that names fields: json for reports, yaml for packs
}

// Files maps each schema file to the Go type it describes.
var Files = map[string]source{
	"report.v1.json":   {model.Report{}, "json"},
	"manifest.v1.json": {model.Manifest{}, "json"},
	"pack.v1.json":     {model.Pack{}, "yaml"},
}

// enums gives the named string types whose values model lists their closed set.
func enums(t reflect.Type) *jsonschema.Schema {
	var values []any
	switch t {
	case reflect.TypeFor[model.PhaseName]():
		for _, p := range model.Phases() {
			values = append(values, string(p))
		}
	case reflect.TypeFor[model.PhaseState]():
		for _, s := range model.PhaseStates() {
			values = append(values, string(s))
		}
	default:
		return nil
	}
	return &jsonschema.Schema{Type: "string", Enum: values}
}

// Generate returns each schema file's contents. root is the repository root relative to the
// working directory; Go doc comments in internal/model become descriptions.
func Generate(root string) (map[string][]byte, error) {
	base, err := commentBase(root)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for name, src := range Files {
		r := &jsonschema.Reflector{FieldNameTag: src.tag, Mapper: enums}
		if err := r.AddGoComments(base, filepath.Join(root, "internal", "model")); err != nil {
			return nil, err
		}
		s := r.Reflect(src.value)
		s.ID = jsonschema.ID(BaseURL + name)
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return nil, err
		}
		out[name] = append(b, '\n')
	}
	return out, nil
}

// commentBase is the import path of the working directory. The comment extractor keys each
// type by this base joined with the directory it walks, so the keys come out the same whether
// Generate runs from the repository root (make schema) or from schema/ (the test).
func commentBase(root string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, wd)
	if err != nil {
		return "", err
	}
	return path.Join(module, filepath.ToSlash(rel)), nil
}
