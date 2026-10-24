// Package packs holds the parser packs embedded in the doriki binary: one YAML file per engine
// version, each with a real fixture under testdata/. Packs are data, not Go (DESIGN.md
// section 5).
package packs

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Generic is the engine name of the fallback pack.
const Generic = "generic"

// Packs live at <engine>/<version>.yaml, or <engine>/pack.yaml when one pack covers every
// version. testdata holds no YAML, so it is not embedded.
//
//go:embed */*.yaml
var embedded embed.FS

func paths() []string {
	matches, _ := fs.Glob(embedded, "*/*.yaml") // the pattern is a constant, so no error
	return matches
}

func nameFromPath(p string) string {
	engine, file := path.Split(p)
	engine = strings.TrimSuffix(engine, "/")
	if file == "pack.yaml" {
		return engine
	}
	return engine + "/" + strings.TrimSuffix(file, ".yaml")
}

// Names returns the embedded packs as "<engine>/<version>", or "<engine>" for a pack that
// covers every version, sorted.
func Names() []string {
	var names []string
	for _, p := range paths() {
		names = append(names, nameFromPath(p))
	}
	slices.Sort(names)
	return names
}

// All parses every embedded pack. Unknown fields are an error, so a typo in a pack fails
// the tests instead of being ignored.
func All() ([]model.Pack, error) {
	var out []model.Pack
	for _, p := range paths() {
		b, err := embedded.ReadFile(p)
		if err != nil {
			return nil, err
		}
		pack, err := parse(b)
		if err != nil {
			return nil, fmt.Errorf("pack %s: %w", p, err)
		}
		out = append(out, pack)
	}
	return out, nil
}

func parse(b []byte) (model.Pack, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var pack model.Pack
	err := dec.Decode(&pack)
	return pack, err
}

// Select picks the pack for a container image (DESIGN.md section 5.3). With no match it
// falls back to the generic pack and warns, naming the image.
func Select(image string) (model.Pack, []model.Warning, error) {
	all, err := All()
	if err != nil {
		return model.Pack{}, nil, err
	}
	return selectFrom(all, image)
}

func selectFrom(all []model.Pack, image string) (model.Pack, []model.Warning, error) {
	var generic *model.Pack
	var matched []model.Pack
	for i, p := range all {
		if p.Engine == Generic {
			generic = &all[i]
			continue
		}
		if p.Detect.ImageRegex == "" {
			continue
		}
		re, err := regexp.Compile(p.Detect.ImageRegex)
		if err != nil {
			return model.Pack{}, nil, fmt.Errorf("pack %s: image_regex: %w", p.Engine, err)
		}
		if re.MatchString(image) {
			matched = append(matched, p)
		}
	}
	switch {
	case len(matched) == 1:
		return matched[0], nil, nil
	case len(matched) > 1:
		// DESIGN.md section 5.3 disambiguates with log_regex; that lands with the first
		// engine pack (M1b).
		return model.Pack{}, nil, fmt.Errorf("image %q matches %d packs; selecting by log is not implemented yet", image, len(matched))
	case generic == nil:
		return model.Pack{}, nil, fmt.Errorf("no pack matches image %q and the generic pack is missing", image)
	}
	return *generic, []model.Warning{{
		Code: "no-pack",
		Text: fmt.Sprintf("no parser pack matches image %q; only Kubernetes phases are reported", image),
	}}, nil
}
