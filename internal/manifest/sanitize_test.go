package manifest

import (
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

func envFact(name, value string) model.Event {
	return fact(model.FactEnvPrefix+name, value)
}

// Secrets used below. Shaped like real tokens, made up for the test.
const (
	hfToken  = "hf_AbCdEfGhIjKlMnOpQrStUvWxYz012345"                       // #nosec G101 -- made up for the test
	apiKey   = "sk-proj-AbCdEfGhIjKlMnOpQrStUvWx"                          // #nosec G101 -- made up for the test
	ghToken  = "ghp_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"                  // #nosec G101 -- made up for the test
	jwtToken = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJlLXZhbHVl" // #nosec G101 -- made up for the test
)

func sanitizedFacts(t *testing.T, in []model.Event) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, e := range Sanitize(in) {
		if e.Kind == model.KindFact {
			out[e.Attrs[model.AttrFactKey]] = e.Attrs[model.AttrFactValue]
		}
		for _, s := range []string{hfToken, apiKey, ghToken, jwtToken} {
			if strings.Contains(e.Raw, s) || strings.Contains(e.Attrs[model.AttrFactValue], s) {
				t.Errorf("secret %s… survives in %+v", s[:6], e)
			}
		}
	}
	return out
}

// Env capture is allowlist-only (DESIGN.md section 4.3): a variable outside the allowlisted
// prefixes is dropped, value and all. Inside it, a value is redacted when the name says it is a
// secret, or when the value looks like one; anything else is kept as written.
func TestSanitizeEnv(t *testing.T) {
	got := sanitizedFacts(t, []model.Event{
		envFact("VLLM_MODEL", "facebook/opt-125m"),
		envFact("LLAMA_ARG_CTX_SIZE", "2048"),
		envFact("HF_TOKEN", hfToken),
		envFact("HF_HUB_OFFLINE", "0"),
		envFact("VLLM_API_KEY", "plain-looking"),
		envFact("NCCL_SOCKET_PASSWORD", "x"),
		envFact("VLLM_EXTRA_ARGS", "--api-key "+apiKey),
		envFact("SGLANG_CONFIG", ghToken),
		envFact("PATH", "/usr/bin"),
		envFact("AWS_SECRET_ACCESS_KEY", "abc"),
		envFact("MY_TOKEN", jwtToken),
	})
	want := map[string]string{
		model.FactEnvPrefix + "VLLM_MODEL":           "facebook/opt-125m",
		model.FactEnvPrefix + "LLAMA_ARG_CTX_SIZE":   "2048",
		model.FactEnvPrefix + "HF_TOKEN":             Redacted,
		model.FactEnvPrefix + "HF_HUB_OFFLINE":       "0",
		model.FactEnvPrefix + "VLLM_API_KEY":         Redacted,
		model.FactEnvPrefix + "NCCL_SOCKET_PASSWORD": Redacted,
		model.FactEnvPrefix + "VLLM_EXTRA_ARGS":      "--api-key " + Redacted,
		model.FactEnvPrefix + "SGLANG_CONFIG":        Redacted,
	}
	if len(got) != len(want) {
		t.Errorf("kept %d env facts, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// Args are redacted the same way: the value after a secret-named flag, in either form, and
// any value that looks like a token. Other args are kept in order.
func TestSanitizeArgs(t *testing.T) {
	got := sanitizedFacts(t, []model.Event{fact(model.FactArgs,
		`["--model","facebook/opt-125m","--api-key","`+apiKey+`","--hf-token=`+hfToken+`","--port","8000","--x","`+jwtToken+`"]`)})
	want := `["--model","facebook/opt-125m","--api-key","` + Redacted + `","--hf-token=` + Redacted + `","--port","8000","--x","` + Redacted + `"]`
	if got[model.FactArgs] != want {
		t.Errorf("args = %s\nwant %s", got[model.FactArgs], want)
	}
}

// Events that are not env or args facts pass through untouched, pack facts included.
func TestSanitizeLeavesOtherEvents(t *testing.T) {
	in := []model.Event{
		fact("model.ref", "facebook/opt-125m"),
		fact(model.FactPodUID, "u-1"),
		{Source: model.SourceEngineLog, Kind: model.KindLogLine, Raw: "INFO started"},
	}
	out := Sanitize(in)
	if len(out) != len(in) {
		t.Fatalf("Sanitize kept %d of %d events", len(out), len(in))
	}
	for i := range in {
		if model.CompareEvents(in[i], out[i]) != 0 {
			t.Errorf("event %d changed: %+v -> %+v", i, in[i], out[i])
		}
	}
}

// Only the allowlist decides what is captured: swapping it for a sentinel changes the result,
// so no other copy of the prefixes is in use (PROJECT.md section 6).
func TestSanitizeUsesAllowlist(t *testing.T) {
	saved := envAllowlist
	t.Cleanup(func() { envAllowlist = saved })
	envAllowlist = []string{"SENTINEL_"}
	got := sanitizedFacts(t, []model.Event{envFact("VLLM_MODEL", "m"), envFact("SENTINEL_X", "1")})
	if _, ok := got[model.FactEnvPrefix+"VLLM_MODEL"]; ok || got[model.FactEnvPrefix+"SENTINEL_X"] != "1" {
		t.Errorf("with a sentinel allowlist got %v", got)
	}
}
