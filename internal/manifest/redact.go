package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Redacted replaces a secret value.
const Redacted = "[redacted]"

// The secret patterns (PROJECT.md section 6 names this file as their one home). A name that
// says secret redacts its whole value; a value shaped like a known token is redacted wherever
// it appears, so a secret under an innocent name, or inside a longer value, is caught too.
var (
	secretName  = regexp.MustCompile(`(?i)token|secret|passw|key|credential|auth`)
	secretValue = regexp.MustCompile(strings.Join([]string{
		`hf_[A-Za-z0-9]{20,}`,                               // Hugging Face
		`sk-[A-Za-z0-9_-]{16,}`,                             // OpenAI-style API keys
		`gh[pousr]_[A-Za-z0-9]{20,}`,                        // GitHub tokens
		`github_pat_[A-Za-z0-9_]{20,}`,                      // GitHub fine-grained tokens
		`xox[abposr]-[A-Za-z0-9-]{10,}`,                     // Slack
		`AKIA[0-9A-Z]{16}`,                                  // AWS access key ids
		`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`, // JWTs
	}, "|"))
)

// Sanitize applies env capture rules to the collector's facts before anything else reads them
// (DESIGN.md sections 4.3 and 10.3): an env fact outside the allowlist is dropped, an
// allowlisted one has its value redacted when the name says secret and any token-shaped part
// redacted otherwise, and args are redacted the same way. Every other event passes unchanged.
func Sanitize(events []model.Event) []model.Event {
	out := make([]model.Event, 0, len(events))
	for _, e := range events {
		if e.Source != model.SourceManifest || e.Kind != model.KindFact {
			out = append(out, e)
			continue
		}
		key, value := e.Attrs[model.AttrFactKey], e.Attrs[model.AttrFactValue]
		switch {
		case strings.HasPrefix(key, model.FactEnvPrefix):
			name := strings.TrimPrefix(key, model.FactEnvPrefix)
			if !allowlisted(name) {
				continue
			}
			e = withValue(e, redactValue(name, value))
		case key == model.FactArgs:
			args, ok := redactArgs(value)
			if !ok {
				continue // unreadable args could hide anything: leave them out
			}
			e = withValue(e, args)
		}
		out = append(out, e)
	}
	return out
}

func allowlisted(name string) bool {
	return slices.ContainsFunc(envAllowlist, func(prefix string) bool { return strings.HasPrefix(name, prefix) })
}

func redactValue(name, value string) string {
	if secretName.MatchString(name) {
		return Redacted
	}
	return secretValue.ReplaceAllString(value, Redacted)
}

// redactArgs redacts the value of a secret-named flag, as --flag=value or as the next argument,
// and any token-shaped part of any argument.
func redactArgs(value string) (string, bool) {
	var args []string
	if err := json.Unmarshal([]byte(value), &args); err != nil {
		return "", false
	}
	for i, a := range args {
		if !strings.HasPrefix(a, "-") || !secretName.MatchString(a) {
			args[i] = secretValue.ReplaceAllString(a, Redacted)
			continue
		}
		if flag, _, ok := strings.Cut(a, "="); ok {
			args[i] = flag + "=" + Redacted
		} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			args[i+1] = Redacted
		}
	}
	out, _ := json.Marshal(args) // a []string always marshals
	return string(out), true
}

func withValue(e model.Event, v string) model.Event {
	attrs := map[string]string{}
	for k, val := range e.Attrs {
		attrs[k] = val
	}
	attrs[model.AttrFactValue] = v
	e.Attrs = attrs
	return e
}

// Fingerprint is "sha256:" and the hash of the manifest without the pod's name and UID and
// without any earlier fingerprint, so two boots of one deployment share it (DESIGN.md section
// 4.3). encoding/json writes struct fields in order and map keys sorted, so equal manifests
// hash equal. It does not modify m.
func Fingerprint(m *model.Manifest) string {
	c := *m
	c.Fingerprint = ""
	if m.Pod != nil {
		pod := *m.Pod
		pod.Name, pod.UID = "", ""
		c.Pod = &pod
	}
	b, _ := json.Marshal(c) // the manifest holds only strings, ints, slices and maps of strings
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
