package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/AmitwalaH/doriki/internal/collect/k8s"
)

const llamacppFixture = "../../packs/llamacpp/testdata/11889/"

// Secrets planted in the kind-nginx pod for the capture tests: a token-shaped value under a
// secret-named allowlisted variable, a plain password outside the allowlist, a plain value
// after a secret-named flag, and a token the engine prints in its log.
const (
	plantedEnvToken = "hf_plantedEnvToken0000000000000000" // #nosec G101 -- made up for the test
	plantedPassword = "plain-password-123"
	plantedArg      = "plain-arg-secret-456"
	plantedLogToken = "ghp_plantedLogToken00000000000000000" // #nosec G101 -- made up for the test
)

// fakeCluster serves a real fixture as a live API: its pod (changed by mutate), events, a node
// and its log, with extra appended to the log. It returns the pod.
func fakeCluster(t *testing.T, dir string, mutate func(*corev1.Pod), extra string) *corev1.Pod {
	t.Helper()
	read := func(name string, v any) {
		b, err := os.ReadFile(dir + name) // #nosec G304 -- a fixture path
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatal(err)
		}
	}
	var pod corev1.Pod
	read("pod.json", &pod)
	if mutate != nil {
		mutate(&pod)
	}
	var events corev1.EventList
	read("events.json", &events)
	node := &corev1.Node{}
	node.Name = pod.Spec.NodeName
	node.Labels = map[string]string{"kubernetes.io/hostname": pod.Spec.NodeName}
	node.Status.NodeInfo.KubeletVersion = "v1.34.1"
	node.Status.Images = []corev1.ContainerImage{{Names: []string{"registry.internal/secret-team/other:1"}}}
	objs := []runtime.Object{&pod, node}
	for i := range events.Items {
		objs = append(objs, &events.Items[i])
	}
	logs := func(_ context.Context, _, _ string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
		name := "container.log"
		if opts.Previous {
			name = "container-previous.log"
		}
		b, err := os.ReadFile(dir + name) // #nosec G304 -- a fixture path
		if err != nil {
			return nil, err
		}
		return io.NopCloser(strings.NewReader(string(b) + extra)), nil
	}
	saved := liveClient
	t.Cleanup(func() { liveClient = saved })
	liveClient = func(*genericclioptions.ConfigFlags) (k8s.Client, string, error) {
		return k8s.Client{API: fake.NewClientset(objs...), Logs: logs}, pod.Namespace, nil
	}
	return &pod
}

// plant puts the secrets in the pod's first container and in a last-applied annotation, as
// kubectl apply writes one.
func plant(p *corev1.Pod) {
	c := &p.Spec.Containers[0]
	c.Env = append(c.Env, corev1.EnvVar{Name: "HF_TOKEN", Value: plantedEnvToken}, corev1.EnvVar{Name: "DB_PASSWORD", Value: plantedPassword})
	c.Args = append(c.Args, "--api-key", plantedArg)
	if p.Annotations == nil {
		p.Annotations = map[string]string{}
	}
	p.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"env":[{"name":"DB_PASSWORD","value":"` + plantedPassword + `"}]}`
}

// logLine is one more log line in the fixture's format, at the time of its last line.
func logLine(t *testing.T, dir, text string) string {
	t.Helper()
	b, err := os.ReadFile(dir + "container.log") // #nosec G304 -- a fixture path
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	ts, _, _ := strings.Cut(lines[len(lines)-1], " ")
	return ts + " " + text + "\n"
}

// bundle reads every file of a capture directory.
func bundle(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- the test's own bundle
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(b)
	}
	return out
}

// timing is the part of a report a replay must reproduce byte for byte.
func timing(t *testing.T, report string) string {
	t.Helper()
	var r struct {
		Phases json.RawMessage `json:"phases"`
		Gaps   json.RawMessage `json:"gaps"`
		Totals json.RawMessage `json:"totals"`
	}
	if err := json.Unmarshal([]byte(report), &r); err != nil {
		t.Fatalf("not a report: %v\n%s", err, report)
	}
	return string(r.Phases) + string(r.Gaps) + string(r.Totals)
}

// The default capture writes the bundle's files, every one redacted: none of the planted
// secrets (env, args, the last-applied annotation, a log line) is in any of them
// (DESIGN.md sections 7.1 and 10.3).
func TestCaptureDefault(t *testing.T) {
	pod := fakeCluster(t, fixture, plant, logLine(t, fixture, "auth token="+plantedLogToken))
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := run(t, "capture", "pod/"+pod.Name, "--out", out); err != nil {
		t.Fatalf("capture: %v", err)
	}
	files := bundle(t, out)
	for _, name := range []string{"pod.json", "events.json", "container.log", "node.json", "manifest.json", "report.json", "SOURCE.md"} {
		if _, ok := files[name]; !ok {
			t.Errorf("bundle has no %s; has %v", name, keys(files))
		}
	}
	for name, body := range files {
		for _, secret := range []string{plantedEnvToken, plantedPassword, plantedArg, plantedLogToken} {
			if strings.Contains(body, secret) {
				t.Errorf("%s contains planted secret %q", name, secret)
			}
		}
	}
	if !strings.Contains(files["container.log"], "auth token=[redacted]") {
		t.Errorf("the log line is gone rather than redacted")
	}
	if strings.Contains(files["node.json"], "secret-team") {
		t.Errorf("node.json lists the node's other images")
	}
	if !strings.Contains(files["SOURCE.md"], "Redaction: default") {
		t.Errorf("SOURCE.md does not name the redaction level:\n%s", files["SOURCE.md"])
	}
}

// doriki --from-files on the bundle reproduces the capture's report.json and the live report:
// byte-identical phases, gaps and totals (M3 task acceptance 4).
func TestCaptureReplays(t *testing.T) {
	pod := fakeCluster(t, restartFixture, plant, logLine(t, restartFixture, "token="+plantedLogToken))
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := run(t, "capture", "pod/"+pod.Name, "--out", out); err != nil {
		t.Fatalf("capture: %v", err)
	}
	files := bundle(t, out)
	if _, ok := files["container-previous.log"]; !ok {
		t.Fatalf("a restarted pod's bundle has no container-previous.log; has %v", keys(files))
	}
	replay, err := run(t, "--from-files", out, "-o", "json")
	if code, msg := Exit(err); code > 1 { // 1 is a report with open or absent phases
		t.Fatalf("replay: exit %d %s", code, msg)
	}
	live, _ := run(t, "pod/"+pod.Name, "-o", "json")
	if timing(t, replay) != timing(t, files["report.json"]) {
		t.Errorf("replay differs from report.json:\n%s\n%s", timing(t, replay), timing(t, files["report.json"]))
	}
	if timing(t, replay) != timing(t, live) {
		t.Errorf("replay differs from the live report:\n%s\n%s", timing(t, replay), timing(t, live))
	}
}

// numbers is timing without the phases' attributes.
func numbers(t *testing.T, report string) string {
	t.Helper()
	var r struct {
		Phases []map[string]any `json:"phases"`
		Gaps   json.RawMessage  `json:"gaps"`
		Totals json.RawMessage  `json:"totals"`
	}
	if err := json.Unmarshal([]byte(report), &r); err != nil {
		t.Fatalf("not a report: %v", err)
	}
	for _, p := range r.Phases {
		delete(p, "attrs")
	}
	b, _ := json.Marshal(r.Phases)
	return string(b) + string(r.Gaps) + string(r.Totals)
}

var testWord = regexp.MustCompile(`[A-Za-z0-9._-]+`)

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range testWord.FindAllString(s, -1) {
		out[tok] = true
	}
	return out
}

// Strict also replaces the pod, namespace, owner and node names and the image's registry and
// org with salted hashes, the same hash for the same name in every file. The image name stays
// because the llama.cpp pack's image_regex matches it, so the pack is still selected; the
// bundle's report is built from the redacted files, and the replay reproduces it.
func TestCaptureStrict(t *testing.T) {
	pod := fakeCluster(t, llamacppFixture, nil, "")
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := run(t, "capture", "pod/"+pod.Name, "--out", out, "--redact-level", "strict"); err != nil {
		t.Fatalf("capture: %v", err)
	}
	files := bundle(t, out)
	names := []string{pod.Name, pod.Namespace, pod.Spec.NodeName, "llamacpp-f6ff5f6c9", "llamacpp-f6ff5f6c9-", "llamacpp", "ghcr.io", "ggml-org"}
	for _, f := range []string{"pod.json", "events.json", "container.log", "node.json"} {
		toks := tokens(files[f])
		for _, n := range names {
			if toks[n] {
				t.Errorf("%s still has %q", f, n)
			}
		}
	}
	if !tokens(files["pod.json"])["llama.cpp"] {
		t.Errorf("the image name llama.cpp was hashed, though a pack's image_regex matches it")
	}
	for _, n := range []string{pod.Name, pod.Spec.NodeName, "ggml-org"} {
		if tokens(files["SOURCE.md"])[n] {
			t.Errorf("SOURCE.md has %q", n)
		}
	}

	var report struct {
		Manifest struct {
			Pod    struct{ Name, Namespace string }
			Node   struct{ Name string }
			Engine struct {
				ParserPack string `json:"parser_pack"`
			}
		}
	}
	if err := json.Unmarshal([]byte(files["report.json"]), &report); err != nil {
		t.Fatal(err)
	}
	m := report.Manifest
	if m.Engine.ParserPack != "llamacpp/11889" {
		t.Errorf("strict bundle selected pack %q, want llamacpp/11889", m.Engine.ParserPack)
	}
	hashed := regexp.MustCompile(`^h-[0-9a-f]{8}$`)
	if !hashed.MatchString(m.Pod.Name) || !hashed.MatchString(m.Pod.Namespace) || !hashed.MatchString(m.Node.Name) {
		t.Errorf("manifest names not hashed: pod %q, namespace %q, node %q", m.Pod.Name, m.Pod.Namespace, m.Node.Name)
	}
	if !strings.Contains(files["events.json"], m.Pod.Name) || !strings.Contains(files["pod.json"], m.Node.Name) {
		t.Errorf("a name's hash differs between files")
	}
	if !strings.Contains(files["SOURCE.md"], "fingerprint") {
		t.Errorf("SOURCE.md does not say the fingerprint differs from the live report's")
	}

	replay, err := run(t, "--from-files", out, "-o", "json")
	if code, msg := Exit(err); code > 1 { // 1 is a report with open or absent phases
		t.Fatalf("replay: exit %d %s", code, msg)
	}
	live, _ := run(t, "pod/"+pod.Name, "-o", "json")
	if timing(t, replay) != timing(t, files["report.json"]) {
		t.Errorf("replay differs from the bundle's report.json:\n%s\n%s", timing(t, replay), timing(t, files["report.json"]))
	}
	// Against the live report only the numbers can match: a phase attribute may hold a hashed
	// name (llama.cpp's model ggml-org/models shares the image's org, ggml-org).
	if numbers(t, replay) != numbers(t, live) {
		t.Errorf("strict replay's times differ from the live report:\n%s\n%s", numbers(t, replay), numbers(t, live))
	}
}

// An image whose name no pack's image_regex matches has its name hashed too.
func TestCaptureStrictUnknownImage(t *testing.T) {
	pod := fakeCluster(t, fixture, nil, "")
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := run(t, "capture", "pod/"+pod.Name, "--out", out, "--redact-level", "strict"); err != nil {
		t.Fatalf("capture: %v", err)
	}
	files := bundle(t, out)
	for _, n := range []string{"nginx", "docker.io", "library"} {
		if tokens(files["pod.json"])[n] {
			t.Errorf("pod.json still has %q", n)
		}
	}
	if !strings.Contains(files["SOURCE.md"], "name hashed") {
		t.Errorf("SOURCE.md does not say which image rule applied:\n%s", files["SOURCE.md"])
	}
}

// capture refuses --redact-level none, and an --out directory that already holds files.
func TestCaptureRefuses(t *testing.T) {
	pod := fakeCluster(t, fixture, nil, "")
	if _, err := run(t, "capture", "pod/"+pod.Name, "--out", t.TempDir()+"/b", "--redact-level", "none"); err == nil {
		t.Errorf("--redact-level none accepted")
	}
	full := t.TempDir()
	if err := os.WriteFile(filepath.Join(full, "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "capture", "pod/"+pod.Name, "--out", full); err == nil {
		t.Errorf("capture wrote into a directory that already holds files")
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A token on a marker's line is redacted from the report, --include-events included, and the
// marker still matches: vLLM's weight-load keeps its 0.49 s (DK-COLLECT-ENH-20261026-redact-log-lines).
func TestLogTokenRedactedMarkerKept(t *testing.T) {
	const vllm = "../../packs/vllm/testdata/0.32/"
	dir := t.TempDir()
	for _, name := range []string{"pod.json", "events.json", "container.log"} {
		b, err := os.ReadFile(vllm + name) // #nosec G304 -- a fixture path
		if err != nil {
			t.Fatal(err)
		}
		if name == "container.log" {
			b = []byte(strings.Replace(string(b), "Loading weights took 0.49 seconds", "Loading weights took 0.49 seconds token="+plantedLogToken, 1))
			if !strings.Contains(string(b), plantedLogToken) {
				t.Fatal("token not planted")
			}
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil { // #nosec G703 -- a file in t.TempDir()
			t.Fatal(err)
		}
	}
	out, err := run(t, "--from-files", dir, "-o", "json", "--include-events")
	if code, msg := Exit(err); code > 1 {
		t.Fatalf("exit %d %s", code, msg)
	}
	if strings.Contains(out, plantedLogToken) {
		t.Errorf("the planted token is in the report")
	}
	var r struct {
		Phases []struct {
			Name    string
			Seconds *float64
		}
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	for _, p := range r.Phases {
		if p.Name == "weight-load" && (p.Seconds == nil || *p.Seconds != 0.49) {
			t.Errorf("weight-load = %v, want 0.49 s: the marker no longer matches", p.Seconds)
		}
	}
}

// imageParts on the shapes a pod carries. A bare image id (containerd's status.image after a
// pull by digest, seen on the kind cluster) has no registry, org or name: hashing "sha256" would
// break every digest in the bundle.
func TestImageParts(t *testing.T) {
	for ref, want := range map[string]string{
		"ghcr.io/ggml-org/llama.cpp:server@sha256:5741ab30": "ghcr.io|ggml-org|llama.cpp",
		"ghcr.io/ggml-org/llama.cpp@sha256:5741ab30":        "ghcr.io|ggml-org|llama.cpp",
		"sha256:3671daf4816b3e0de4df48ac983d8732c4f6bb3b":   "||",
		"nginx:1.29.3-alpine":                               "||nginx",
		"docker.io/library/nginx@sha256:b3c6":               "docker.io|library|nginx",
		"vllm/vllm-openai:v0.32.0":                          "|vllm|vllm-openai",
		"localhost:5000/team/a/img:1":                       "localhost|team/a|img",
	} {
		host, orgs, name := imageParts(ref)
		if got := host + "|" + strings.Join(orgs, "/") + "|" + name; got != want {
			t.Errorf("imageParts(%q) = %q, want %q", ref, got, want)
		}
	}
}

// A plain API key vLLM echoes in its "non-default args" line is redacted from the report and a
// capture bundle, and the model ref the pack reads from that same line survives
// (DK-COLLECT-ENH-20261028-secret-keys-in-log-lines).
func TestEchoedAPIKeyRedacted(t *testing.T) {
	const vllm, plain = "../../packs/vllm/testdata/0.32/", "plainword-7f3a"
	dir := t.TempDir()
	for _, name := range []string{"pod.json", "events.json", "container.log"} {
		b, err := os.ReadFile(vllm + name) // #nosec G304 -- a fixture path
		if err != nil {
			t.Fatal(err)
		}
		if name == "container.log" {
			b = []byte(strings.Replace(string(b), "'model': 'facebook/opt-125m',", "'model': 'facebook/opt-125m', 'api_key': '"+plain+"',", 1))
			if !strings.Contains(string(b), plain) {
				t.Fatal("key not planted")
			}
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil { // #nosec G703 -- a file in t.TempDir()
			t.Fatal(err)
		}
	}
	out, err := run(t, "--from-files", dir, "-o", "json", "--include-events")
	if code, msg := Exit(err); code > 1 {
		t.Fatalf("exit %d %s", code, msg)
	}
	if strings.Contains(out, plain) {
		t.Errorf("the echoed API key is in the report")
	}
	var r struct {
		Manifest struct{ Model struct{ Ref string } }
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if r.Manifest.Model.Ref != "facebook/opt-125m" {
		t.Errorf("model.ref = %q, want facebook/opt-125m from the same line", r.Manifest.Model.Ref)
	}
}

// At the default level SOURCE.md's Commands block is a command the recipient can paste: the
// target as given and the pod's namespace, which pod.json holds anyway. The namespace is
// written even when -n was not typed, since one taken from the kubeconfig context is the
// capturing machine's and would name the wrong namespace elsewhere
// (DK-CLI-BUG-20261029-capture-source-command).
func TestCaptureSourceCommand(t *testing.T) {
	pod := fakeCluster(t, fixture, nil, "")
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := run(t, "capture", "pod/"+pod.Name, "--out", out); err != nil {
		t.Fatalf("capture: %v", err)
	}
	source := bundle(t, out)["SOURCE.md"]
	want := "doriki capture pod/" + pod.Name + " -n " + pod.Namespace + " --out <dir> --redact-level default\n"
	if !strings.Contains(source, want) || strings.Contains(source, "named on the command line") {
		t.Errorf("SOURCE.md does not have %q:\n%s", want, source)
	}
}
