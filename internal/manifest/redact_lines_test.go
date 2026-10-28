package manifest

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

const (
	plantedHF = "hf_abcdefghijklmnopqrstuvwxyz0123"
	plantedGH = "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345"
)

// A token an engine prints in its log is redacted in the stored line and in the whole line
// packs match, so it reaches no report, --include-events or capture
// (DK-COLLECT-ENH-20261026-redact-log-lines). The rest of the line stays, so markers still match.
func TestSanitizeLogLines(t *testing.T) {
	line := "INFO Loading weights took 0.49 seconds token=" + plantedHF
	out := Sanitize([]model.Event{{Source: model.SourceEngineLog, Kind: model.KindLogLine, Raw: line[:30], Line: line}})
	if len(out) != 1 {
		t.Fatalf("got %d events", len(out))
	}
	want := "INFO Loading weights took 0.49 seconds token=" + Redacted
	if out[0].Line != want || strings.Contains(out[0].Raw+out[0].Line, plantedHF) {
		t.Errorf("Line %q, Raw %q; want Line %q", out[0].Line, out[0].Raw, want)
	}
}

// Every string value in a JSON document has its token-shaped parts redacted, at any depth;
// keys, numbers and the document's shape are kept.
func TestRedactJSON(t *testing.T) {
	in := `{"a":"x ` + plantedGH + ` y","n":3,"list":[{"b":"` + plantedHF + `"},true,null],"keep":"plain"}`
	out, err := RedactJSON([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), plantedGH) || strings.Contains(string(out), plantedHF) {
		t.Fatalf("token left in %s", out)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if got["a"] != "x "+Redacted+" y" || got["n"] != float64(3) || got["keep"] != "plain" {
		t.Errorf("got %v", got)
	}
	if _, err := RedactJSON([]byte("{not json")); err == nil {
		t.Errorf("no error on bad JSON")
	}
}

// The capture's env rule is the manifest's: kept only when allowlisted, with a secret name's
// value redacted whole.
func TestRedactEnv(t *testing.T) {
	for _, c := range []struct {
		name, value, want string
		keep              bool
	}{
		{"HF_TOKEN", "anything", Redacted, true},
		{"HF_HOME", "/cache " + plantedHF, "/cache " + Redacted, true},
		{"VLLM_MODEL", "facebook/opt-125m", "facebook/opt-125m", true},
		{"MY_PASSWORD", "hunter2", "", false},
	} {
		got, keep := RedactEnv(c.name, c.value)
		if got != c.want || keep != c.keep {
			t.Errorf("RedactEnv(%s) = %q, %v; want %q, %v", c.name, got, keep, c.want, c.keep)
		}
	}
}

// Args and command lists use the args rule.
func TestRedactArgList(t *testing.T) {
	got := RedactArgList([]string{"--api-key", "plain-secret", "--model", "org/m", "--hf-token=x", plantedHF})
	want := []string{"--api-key", Redacted, "--model", "org/m", "--hf-token=" + Redacted, Redacted}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A plain secret (no token shape) after a secret-named key in a log line is redacted in each
// shape engines print: a Python dict (vLLM's "non-default args"), key=value, key: value,
// --flag value, and an Authorization header. The key list is the one env names use (secretName),
// and the rest of the line stays (DK-COLLECT-ENH-20261028-secret-keys-in-log-lines).
func TestRedactLineSecretKeys(t *testing.T) {
	for in, want := range map[string]string{ // #nosec G101 -- made up for the test
		// vLLM 0.32's real line (fixture line 110) with an api_key planted.
		"non-default args: {'host': '0.0.0.0', 'model': 'facebook/opt-125m', 'api_key': 'plainword', 'dtype': 'bfloat16'}": "non-default args: {'host': '0.0.0.0', 'model': 'facebook/opt-125m', 'api_key': '[redacted]', 'dtype': 'bfloat16'}",
		`{"password": "two words", "user": "bob"}`:         `{"password": "[redacted]", "user": "bob"}`,
		"starting with api_key=plainword port=8000":        "starting with api_key=[redacted] port=8000",
		"HF_TOKEN: plainword":                              "HF_TOKEN: [redacted]",
		"cmd: server --api-key plainword --port 8000":      "cmd: server --api-key [redacted] --port 8000",
		"cmd: server --hf-token=plainword":                 "cmd: server --hf-token=[redacted]",
		"header Authorization: Bearer abc.def":             "header Authorization: [redacted]",
		"model=org/m dtype=auto port: 8000":                "model=org/m dtype=auto port: 8000",
		"INFO 10-25 09:28:37 [api_utils.py:286] listening": "INFO 10-25 09:28:37 [api_utils.py:286] listening",
		"api_key=[redacted] already":                       "api_key=[redacted] already",
	} {
		if got := RedactLine(in); got != want {
			t.Errorf("RedactLine(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

// The key rule uses secretName itself: swapping it for a sentinel changes which keys are
// redacted, so no second key list is in use.
func TestRedactLineUsesSecretName(t *testing.T) {
	saved := secretName
	t.Cleanup(func() { secretName = saved })
	secretName = regexp.MustCompile(`sentinel`)
	if got := RedactLine("api_key=plainword sentinel=plainword"); got != "api_key=plainword sentinel=[redacted]" {
		t.Errorf("got %q", got)
	}
}
