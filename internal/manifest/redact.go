package manifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/AmitwalaH/doriki/internal/model"
)

// secretWords are the words that make a name secret: an env variable, a flag, or a key in a
// log line. A name says secret when one of its whole words is one of these, so HF_TOKEN,
// VLLM_API_KEY, --hf-token and accessToken do, and tokenizer, max_tokens and keyboard do not.
// A variable so a test can swap in a sentinel and prove nothing else decides.
var secretWords = []string{"token", "secret", "password", "passwd", "key", "apikey", "credential", "credentials", "auth", "authorization"}

// nameWord is a word of a name: a run of letters and digits, split again where a lower-case
// letter meets an upper-case one (camelCase).
var nameWord = regexp.MustCompile(`[A-Z]?[a-z0-9]+|[A-Z0-9]+`)

// secretName reports whether one of name's words is a secret word.
func secretName(name string) bool {
	for _, w := range nameWord.FindAllString(name, -1) {
		if slices.Contains(secretWords, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

// Redacted replaces a secret value.
const Redacted = "[redacted]"

// The secret patterns (PROJECT.md section 6 names this file as their one home). A name that
// says secret redacts its whole value; a value shaped like a known token is redacted wherever
// it appears, so a secret under an innocent name, or inside a longer value, is caught too.
var (
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
// redacted otherwise, and args are redacted the same way. An engine log line has its
// token-shaped parts redacted, so a token the engine prints reaches no report or capture; the
// rest of the line stays for packs to match. Every other event passes unchanged.
func Sanitize(events []model.Event) []model.Event {
	out := make([]model.Event, 0, len(events))
	for _, e := range events {
		if e.Source == model.SourceEngineLog && e.Kind == model.KindLogLine {
			// Raw is Line cut to MaxRawBytes, so it is cut again from the redacted line: a
			// token across the cut would leave a stub too short for the patterns.
			if e.Line != "" {
				e.Line = RedactLine(e.Line)
				e.Raw = model.TruncateRaw(e.Line)
			} else {
				e.Raw = RedactLine(e.Raw)
			}
		}
		if e.Source != model.SourceManifest || e.Kind != model.KindFact {
			out = append(out, e)
			continue
		}
		key, value := e.Attrs[model.AttrFactKey], e.Attrs[model.AttrFactValue]
		switch {
		case strings.HasPrefix(key, model.FactEnvPrefix):
			name := strings.TrimPrefix(key, model.FactEnvPrefix)
			v, keep := RedactEnv(name, value)
			if !keep {
				continue
			}
			e = withValue(e, v)
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

// RedactEnv is the env capture rule for one variable: keep is false outside the allowlist;
// inside it, a secret name's value is redacted whole, and any other value's token-shaped parts.
func RedactEnv(name, value string) (v string, keep bool) {
	if !allowlisted(name) {
		return "", false
	}
	if secretName(name) {
		return Redacted, true
	}
	return RedactLine(value), true
}

// The shapes in which a log line pairs a key with its value, for RedactLine: a quoted value
// (a Python dict such as vLLM's "non-default args", or JSON), key=value or key: value with an
// optional Bearer or Basic scheme, and a --flag followed by its value. Group 1 is the key; the
// other groups are the value. A value already [redacted] does not match again.
var keyValues = []*regexp.Regexp{
	regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_.-]*)['"]?\s*[:=]\s*(?:'([^']*)'|"([^"]*)")`),
	regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_.-]*)\s*[:=]\s*((?:(?:Bearer|Basic)\s+)?[^\s'",;{}\[\]()]+)`),
	regexp.MustCompile(`--([A-Za-z0-9_-]+)\s+([^\s\[-]\S*)`),
}

// RedactLine redacts every token-shaped part of s, and the value after any secret-named key
// (secretName, the rule env names use), so an engine that echoes a plain API key or password
// does not leak it. Only whole words count, so a key such as "tokenizer" keeps its value.
func RedactLine(s string) string {
	s = secretValue.ReplaceAllString(s, Redacted)
	for _, re := range keyValues {
		s = redactValues(re, s)
	}
	return s
}

// redactValues replaces the value groups of re's matches whose key (group 1) says secret.
func redactValues(re *regexp.Regexp, s string) string {
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		if !secretName(s[m[2]:m[3]]) {
			continue
		}
		for g := 2; 2*g+1 < len(m); g++ {
			if start, end := m[2*g], m[2*g+1]; start >= 0 && s[start:end] != Redacted {
				b.WriteString(s[last:start])
				b.WriteString(Redacted)
				last = end
			}
		}
	}
	b.WriteString(s[last:])
	return b.String()
}

// RedactJSON redacts the token-shaped parts of every string value in a JSON document, at any
// depth. The document is decoded with numbers kept as written, so only strings change.
func RedactJSON(b []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case string:
			return RedactLine(x)
		case []any:
			for i := range x {
				x[i] = walk(x[i])
			}
		case map[string]any:
			for k := range x {
				x[k] = walk(x[k])
			}
		}
		return v
	}
	return json.MarshalIndent(walk(v), "", "  ")
}

// redactArgs is RedactArgList on a JSON array of arguments; false when it is not one.
func redactArgs(value string) (string, bool) {
	var args []string
	if err := json.Unmarshal([]byte(value), &args); err != nil {
		return "", false
	}
	out, _ := json.Marshal(RedactArgList(args)) // a []string always marshals
	return string(out), true
}

// RedactArgList redacts the value of a secret-named flag, as --flag=value or as the next
// argument, and any token-shaped part of any argument. It returns a new slice.
func RedactArgList(in []string) []string {
	args := slices.Clone(in)
	for i, a := range args {
		if !strings.HasPrefix(a, "-") || !secretName(a) {
			args[i] = RedactLine(a)
			continue
		}
		if flag, _, ok := strings.Cut(a, "="); ok {
			args[i] = flag + "=" + Redacted
		} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			args[i+1] = Redacted
		}
	}
	return args
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
