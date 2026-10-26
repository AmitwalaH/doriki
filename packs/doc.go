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

	"github.com/Masterminds/semver/v3"
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
		pack.Name = nameFromPath(p)
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

// detectLines is how far into the log log_regex looks for the engine version (DESIGN.md
// section 5.3).
const detectLines = 200

// Select picks the pack for a container image and its log lines (DESIGN.md section 5.3). The
// image narrows the packs; log_regex on the first 200 lines reads the engine version, and the
// pack whose versions range holds it wins. With no match it falls back to the generic pack
// and warns, naming the image or the version.
func Select(image string, lines ...string) (model.Pack, []model.Warning, error) {
	all, err := All()
	if err != nil {
		return model.Pack{}, nil, err
	}
	return selectFrom(all, image, lines...)
}

func selectFrom(all []model.Pack, image string, lines ...string) (model.Pack, []model.Warning, error) {
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
	if len(matched) > 0 {
		return selectByVersion(matched, generic, image, lines)
	}
	if generic == nil {
		return model.Pack{}, nil, fmt.Errorf("no pack matches image %q and the generic pack is missing", image)
	}
	return *generic, []model.Warning{{
		Code: "no-pack",
		Text: fmt.Sprintf("no parser pack matches image %q; only Kubernetes phases are reported", image),
	}}, nil
}

// selectByVersion chooses among the packs matching the image by the version their log_regex
// reads from the first lines.
func selectByVersion(matched []model.Pack, generic *model.Pack, image string, lines []string) (model.Pack, []model.Warning, error) {
	lines = lines[:min(len(lines), detectLines)]
	var covering, names, ranges []string
	var found string
	byName := map[string]model.Pack{}
	for _, p := range matched {
		names, ranges = append(names, p.Name), append(ranges, p.Versions)
		byName[p.Name] = p
		v, err := logVersion(p, lines)
		if err != nil {
			return model.Pack{}, nil, err
		}
		if v == "" {
			continue
		}
		found = v
		ok, err := covers(p, v)
		if err != nil {
			return model.Pack{}, nil, err
		}
		if ok {
			covering = append(covering, p.Name)
		}
	}
	switch {
	case len(covering) == 1:
		return byName[covering[0]], nil, nil
	case len(covering) > 1:
		return model.Pack{}, nil, fmt.Errorf("packs %s all cover version %s", strings.Join(covering, ", "), found)
	case found != "":
		if generic == nil {
			return model.Pack{}, nil, fmt.Errorf("no pack covers version %s and the generic pack is missing", found)
		}
		return *generic, []model.Warning{{
			Code: "no-pack",
			Text: fmt.Sprintf("image %q matches packs %s, but the log says version %s, which none of them covers (%s); only Kubernetes phases are reported",
				image, strings.Join(names, ", "), found, strings.Join(ranges, "; ")),
		}}, nil
	case len(matched) == 1:
		p := matched[0]
		if p.Detect.LogRegex == "" {
			return p, nil, nil
		}
		text := fmt.Sprintf("the log does not say the engine version in its first %d lines; using pack %s, tested with %s", detectLines, p.Name, p.Versions)
		if p.Detect.VersionNote != "" {
			text += "; " + p.Detect.VersionNote
		}
		return p, []model.Warning{{Code: "pack-version-unknown", Text: text}}, nil
	}
	return model.Pack{}, nil, fmt.Errorf("image %q matches packs %s and the log does not say which version it is", image, strings.Join(names, ", "))
}

// logVersion is the named group "version" of the pack's log_regex on the first matching line.
func logVersion(p model.Pack, lines []string) (string, error) {
	if p.Detect.LogRegex == "" {
		return "", nil
	}
	re, err := regexp.Compile(p.Detect.LogRegex)
	if err != nil {
		return "", fmt.Errorf("pack %s: log_regex: %w", p.Name, err)
	}
	i := re.SubexpIndex("version")
	if i < 0 {
		return "", fmt.Errorf("pack %s: log_regex has no named group version", p.Name)
	}
	for _, l := range lines {
		if m := re.FindStringSubmatch(l); m != nil {
			return m[i], nil
		}
	}
	return "", nil
}

// covers reports whether the pack's versions range holds v.
func covers(p model.Pack, v string) (bool, error) {
	c, err := semver.NewConstraint(p.Versions)
	if err != nil {
		return false, fmt.Errorf("pack %s: versions: %w", p.Name, err)
	}
	sv, err := semver.NewVersion(v)
	if err != nil {
		return false, nil // the log said something that is not a version; no pack covers it
	}
	return c.Check(sv), nil
}
