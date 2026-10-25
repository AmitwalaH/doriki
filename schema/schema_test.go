package schema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

// The committed schemas are exactly what the Go types generate, so a model change that is not
// followed by `make schema` fails the gate, in CI and locally (M1b task item 10).
func TestSchemasUpToDate(t *testing.T) {
	gen, err := Generate("..")
	if err != nil {
		t.Fatal(err)
	}
	// The names are listed here, not read from Files, so an empty generator cannot pass.
	for _, name := range []string{"report.v1.json", "manifest.v1.json", "pack.v1.json"} {
		if gen[name] == nil {
			t.Errorf("%s was not generated", name)
		}
	}
	if len(gen) != 3 {
		t.Errorf("generated %d schemas, want 3", len(gen))
	}
	for name, want := range gen {
		got, err := os.ReadFile(name) // #nosec G304 -- a schema file name from Files
		if err != nil {
			t.Errorf("%s: %v (run make schema)", name, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from what the Go types generate; run make schema and commit it", name)
		}
	}
}

func compile(t *testing.T, ref string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.AssertFormat() // format is only an annotation in 2020-12 unless asserted; times must parse
	for name := range Files {
		f, err := os.Open(name) // #nosec G304 -- a schema file name from Files
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(f)
		f.Close() //nolint:errcheck,gosec // read-only file
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := c.AddResource(BaseURL+name, doc); err != nil {
			t.Fatal(err)
		}
	}
	s, err := c.Compile(BaseURL + ref)
	if err != nil {
		t.Fatalf("compile %s: %v", ref, err)
	}
	return s
}

// instance turns any decoded value into what the validator expects, through JSON.
func instance(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return inst
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- a committed file from a fixed glob or name
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return m
}

func glob(t *testing.T, pattern string, atLeast int) []string {
	t.Helper()
	m, err := filepath.Glob(pattern)
	if err != nil || len(m) < atLeast {
		t.Fatalf("%s: %d files (%v), want at least %d; a glob that finds nothing would pass", pattern, len(m), err, atLeast)
	}
	return m
}

// Every document doriki commits validates (DESIGN.md section 9.1, schema layer): each CLI JSON
// golden against the report schema and its manifest against the manifest schema; each pack
// fixture's expected.json, which holds only phases, gaps and totals, against those parts of
// the report schema; each pack against the pack schema.
func TestDocumentsValidate(t *testing.T) {
	report, manifest := compile(t, "report.v1.json"), compile(t, "manifest.v1.json")
	for _, path := range glob(t, "../internal/cli/testdata/*.json", 3) {
		doc := readJSON(t, path)
		if err := report.Validate(instance(t, doc)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if err := manifest.Validate(instance(t, doc["manifest"])); err != nil {
			t.Errorf("%s manifest: %v", path, err)
		}
	}

	parts := map[string]*jsonschema.Schema{
		"phases": compile(t, "report.v1.json#/$defs/Report/properties/phases"),
		"gaps":   compile(t, "report.v1.json#/$defs/Report/properties/gaps"),
		"totals": compile(t, "report.v1.json#/$defs/Report/properties/totals"),
	}
	for _, path := range glob(t, "../packs/*/testdata/*/expected.json", 3) {
		doc := readJSON(t, path)
		for key := range doc {
			if parts[key] == nil {
				t.Errorf("%s: unexpected key %q", path, key)
			}
		}
		for key, s := range parts {
			if v, ok := doc[key]; ok {
				if err := s.Validate(instance(t, v)); err != nil {
					t.Errorf("%s %s: %v", path, key, err)
				}
			}
		}
	}

	pack := compile(t, "pack.v1.json")
	for _, path := range glob(t, "../packs/*/*.yaml", 2) {
		b, err := os.ReadFile(path) // #nosec G304 -- a committed pack from a fixed glob
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := yaml.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if err := pack.Validate(instance(t, doc)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

// The schemas reject what they should, so a pass above means something: an unknown phase
// name, a missing required field, a misspelt pack key and a wrong state each fail.
func TestSchemasReject(t *testing.T) {
	report, pack := compile(t, "report.v1.json"), compile(t, "pack.v1.json")
	golden := readJSON(t, "../internal/cli/testdata/vllm-0.32.json")
	mutate := func(f func(m map[string]any)) any {
		var m map[string]any
		b, _ := json.Marshal(golden)
		_ = json.Unmarshal(b, &m)
		f(m)
		return instance(t, m)
	}
	phase0 := func(m map[string]any) map[string]any { return m["phases"].([]any)[0].(map[string]any) }
	for name, bad := range map[string]any{
		"unknown phase name":      mutate(func(m map[string]any) { phase0(m)["name"] = "warmup" }),
		"unknown phase state":     mutate(func(m map[string]any) { phase0(m)["state"] = "done" }),
		"missing report_version":  mutate(func(m map[string]any) { delete(m, "report_version") }),
		"unknown top-level field": mutate(func(m map[string]any) { m["extra"] = 1 }),
		"time that is not a time": mutate(func(m map[string]any) { phase0(m)["start"] = "yesterday" }),
	} {
		if err := report.Validate(bad); err == nil {
			t.Errorf("report schema accepted %s", name)
		}
	}
	for name, doc := range map[string]string{
		"misspelt marker key": "pack_version: 1\nengine: x\nversions: '1'\nmarkers:\n  - id: a\n    regx: b\n",
		"missing engine":      "pack_version: 1\nversions: '1'\n",
	} {
		var v any
		if err := yaml.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatal(err)
		}
		if err := pack.Validate(instance(t, v)); err == nil {
			t.Errorf("pack schema accepted a pack with a %s", name)
		}
	}
}

// start_from_duration is documented in the pack schema (owner decision on the M1b spike): the
// description comes from the Go comment on the field.
func TestPackSchemaDocumentsStartFromDuration(t *testing.T) {
	b, err := os.ReadFile("pack.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	d := s.Defs["PackMarker"].Properties["start_from_duration"].Description
	if !strings.Contains(d, "duration in seconds") {
		t.Errorf("start_from_duration description = %q", d)
	}
}
