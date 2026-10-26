package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No secret from the pod spec reaches any output, even with every input event included:
// manifest.Sanitize runs right after collection (DESIGN.md sections 4.3 and 10.3). The pod is
// the real kind-nginx capture with env and args added; the secrets are made up.
func TestNoSecretInOutput(t *testing.T) {
	const (
		hfToken = "hf_AbCdEfGhIjKlMnOpQrStUvWxYz012345" // #nosec G101 -- made up for the test
		apiKey  = "sk-proj-AbCdEfGhIjKlMnOpQrStUvWx"    // #nosec G101 -- made up for the test
		other   = "not-allowlisted-value-123"
	)
	pod, err := os.ReadFile(fixture + "pod.json")
	if err != nil {
		t.Fatal(err)
	}
	withSecrets := strings.Replace(string(pod), `"image": "nginx:1.29.3-alpine",`,
		`"image": "nginx:1.29.3-alpine",
		"args": ["--model", "m", "--api-key", "`+apiKey+`"],
		"env": [{"name": "HF_TOKEN", "value": "`+hfToken+`"}, {"name": "VLLM_MODEL", "value": "m"}, {"name": "OTHER", "value": "`+other+`"}],`, 1)
	if withSecrets == string(pod) {
		t.Fatal("could not add env to the fixture pod")
	}
	path := filepath.Join(t.TempDir(), "pod.json")
	if err := os.WriteFile(path, []byte(withSecrets), 0o600); err != nil { // #nosec G703 -- a file in t.TempDir()
		t.Fatal(err)
	}

	for _, format := range []string{"json", "text"} {
		out, _ := run(t, "--from-files", "-o", format, "--include-events", path, fixtureArgs[1], fixtureArgs[2])
		for _, s := range []string{hfToken, apiKey, other} {
			if strings.Contains(out, s) {
				t.Errorf("-o %s: output contains %q", format, s)
			}
		}
		if format == "json" {
			c, _ := decodeReport(t, out)["manifest"].(map[string]any)["container"].(map[string]any)
			env, _ := c["env_allowlist"].(map[string]any)
			if env["HF_TOKEN"] != "[redacted]" || env["VLLM_MODEL"] != "m" || env["OTHER"] != nil {
				t.Errorf("env_allowlist = %v, want HF_TOKEN redacted, VLLM_MODEL kept, OTHER dropped", env)
			}
		}
	}
}

// Two boots of one Deployment share a fingerprint (DESIGN.md section 4.3): the cold and warm
// nginx captures differ in pod name, UID and timing, not in what was deployed.
func TestFingerprintAcrossBoots(t *testing.T) {
	prints := map[string]string{}
	for _, dir := range []string{"kind-nginx", "kind-nginx-warm"} {
		d := "../../packs/generic/testdata/" + dir + "/"
		r, err := ReportFromFiles(d+"pod.json", d+"events.json", d+"container.log", "", false, now())
		if err != nil {
			t.Fatal(err)
		}
		if r.Manifest == nil || r.Manifest.Pod == nil || r.Manifest.Pod.Owner != "Deployment/nginx" {
			t.Fatalf("%s: manifest pod = %+v, want owner Deployment/nginx", dir, r.Manifest.Pod)
		}
		prints[dir] = r.Manifest.Fingerprint
	}
	if prints["kind-nginx"] == "" || prints["kind-nginx"] != prints["kind-nginx-warm"] {
		t.Errorf("fingerprints %v, want one shared value", prints)
	}

	d := "../../packs/vllm/testdata/0.32/"
	r, err := ReportFromFiles(d+"pod.json", d+"events.json", d+"container.log", "", false, now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Manifest.Fingerprint == prints["kind-nginx"] {
		t.Errorf("vllm shares nginx's fingerprint")
	}
	if c := r.Manifest.Container; c == nil || c.EnvAllowlist["VLLM_MODEL"] != "facebook/opt-125m" || !strings.HasPrefix(c.ImageDigest, "sha256:") {
		t.Errorf("vllm container = %+v", c)
	}
}
