package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/AmitwalaH/doriki/internal/collect/k8s"
)

// No test reaches a real cluster: the default connection fails unless a test swaps in a fake.
func TestMain(m *testing.M) {
	liveClient = func(*genericclioptions.ConfigFlags) (k8s.Client, string, error) {
		return k8s.Client{}, "", errors.New("no cluster in tests")
	}
	os.Exit(m.Run())
}

func readFixture(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(restartFixture + name) // #nosec G304 -- a fixture path
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// fakeRestartCluster serves the real kind-restart capture as a live API: its pod, events,
// node, and logs.
func fakeRestartCluster(t *testing.T) {
	t.Helper()
	var pod corev1.Pod
	readFixture(t, "pod.json", &pod)
	var events corev1.EventList
	readFixture(t, "events.json", &events)
	objs := []runtime.Object{&pod, &corev1.Node{}}
	objs[1].(*corev1.Node).Name = pod.Spec.NodeName
	for i := range events.Items {
		objs = append(objs, &events.Items[i])
	}
	logs := func(_ context.Context, _, _ string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
		name := "container.log"
		if opts.Previous {
			name = "container-previous.log"
		}
		f, err := os.Open(restartFixture + name) // #nosec G304 -- a fixture path
		return f, err
	}
	saved := liveClient
	t.Cleanup(func() { liveClient = saved })
	liveClient = func(f *genericclioptions.ConfigFlags) (k8s.Client, string, error) {
		return k8s.Client{API: fake.NewClientset(objs...), Logs: logs}, "default", nil
	}
}

// Live and offline share every line after collection (DESIGN.md section 3): profiling the pod
// from the API gives the very report --from-files gives for the same capture, in both formats.
func TestLiveMatchesFiles(t *testing.T) {
	fakeRestartCluster(t)
	files := []string{restartFixture + "pod.json", restartFixture + "events.json", restartFixture + "container.log", restartFixture + "container-previous.log"}
	for _, format := range []string{"json", "text"} {
		offline, err := run(t, append([]string{"--from-files", "-o", format}, files...)...)
		if err != nil {
			t.Fatalf("offline: %v", err)
		}
		for _, target := range []string{"pod/restart-once", "restart-once", "po/restart-once"} {
			live, err := run(t, "-o", format, target)
			if err != nil {
				t.Fatalf("%s: %v", target, err)
			}
			if live != offline {
				t.Errorf("-o %s %s differs from --from-files:\nlive:\n%s\noffline:\n%s", format, target, live, offline)
			}
		}
	}
}

// Exit 2 for a pod that cannot be profiled, with the API's message (DESIGN.md section 7.3);
// exit 64 for a name doriki cannot parse.
func TestLiveExitCodes(t *testing.T) {
	fakeRestartCluster(t)
	cases := map[string]struct {
		args []string
		code int
		msg  string
	}{
		"pod not found":      {[]string{"pod/nope"}, ExitUnavailable, `pods "nope" not found`},
		"deployment missing": {[]string{"deploy/nope"}, ExitUnavailable, `deployments.apps "nope" not found`},
		"unsupported kind":   {[]string{"svc/x"}, ExitUsage, "svc"},
		"two targets":        {[]string{"pod/a", "pod/b"}, ExitUsage, "one pod"},
		"no target":          {nil, ExitUsage, "pod/<name>"},
		"empty name":         {[]string{"pod/"}, ExitUsage, "pod/<name>"},
		"unknown attempt":    {[]string{"--attempt", "7", "pod/restart-once"}, ExitUsage, "attempt 7 was not observed"},
		"negative since":     {[]string{"--since", "-1h", "pod/restart-once"}, ExitUsage, "--since"},
		"bad attempt value":  {[]string{"--from-files", "--attempt", "x"}, ExitUsage, "attempt"},
	}
	for name, c := range cases {
		_, err := run(t, c.args...)
		code, msg := Exit(err)
		if code != c.code || !strings.Contains(msg, c.msg) {
			t.Errorf("%s: exit %d %q, want %d mentioning %q", name, code, msg, c.code, c.msg)
		}
	}

	saved := liveClient
	t.Cleanup(func() { liveClient = saved })
	liveClient = func(*genericclioptions.ConfigFlags) (k8s.Client, string, error) {
		return k8s.Client{}, "", errors.New(`dial tcp 127.0.0.1:1: connect: connection refused`)
	}
	if code, msg := Exit(func() error { _, err := run(t, "pod/x"); return err }()); code != ExitUnavailable || !strings.Contains(msg, "connection refused") {
		t.Errorf("unreachable cluster: exit %d %q, want %d with the error", code, msg, ExitUnavailable)
	}
}

// Resource names, as kubectl spells them; a bare name is a pod.
func TestParseTarget(t *testing.T) {
	for in, want := range map[string]k8s.Target{
		"x":               {Kind: k8s.KindPod, Name: "x"},
		"pod/x":           {Kind: k8s.KindPod, Name: "x"},
		"pods/x":          {Kind: k8s.KindPod, Name: "x"},
		"po/x":            {Kind: k8s.KindPod, Name: "x"},
		"deploy/x":        {Kind: k8s.KindDeployment, Name: "x"},
		"deployment/x":    {Kind: k8s.KindDeployment, Name: "x"},
		"deployments/x":   {Kind: k8s.KindDeployment, Name: "x"},
		"sts/x":           {Kind: k8s.KindStatefulSet, Name: "x"},
		"statefulset/x":   {Kind: k8s.KindStatefulSet, Name: "x"},
		"statefulsets/x":  {Kind: k8s.KindStatefulSet, Name: "x"},
		"Deployment/vllm": {Kind: k8s.KindDeployment, Name: "vllm"},
	} {
		got, err := parseTarget(in)
		if err != nil || got != want {
			t.Errorf("parseTarget(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
}

// --since reaches the collector as now minus the duration; without it, no cut.
func TestSinceWiring(t *testing.T) {
	fakeRestartCluster(t)
	var got []time.Time
	saved := collect
	t.Cleanup(func() { collect = saved })
	collect = func(ctx context.Context, c k8s.Client, tg k8s.Target, notBefore time.Time) (k8s.Objects, error) {
		got = append(got, notBefore)
		return saved(ctx, c, tg, notBefore)
	}
	_, _ = run(t, "pod/restart-once")
	_, _ = run(t, "--since", "90m", "pod/restart-once")
	want := time.Date(2026, 10, 24, 13, 30, 0, 0, time.UTC) // run pins now at 15:00 UTC
	if len(got) != 2 || !got[0].IsZero() || !got[1].Equal(want) {
		t.Errorf("notBefore = %v, want zero then %v", got, want)
	}
}

// --attempt and --all-attempts select boot attempts, on either path. Attempt 0 of the real
// restart capture ends its container-init at the first container's own log line.
func TestAttemptFlags(t *testing.T) {
	files := []string{"--from-files", restartFixture + "pod.json", restartFixture + "events.json", restartFixture + "container.log", restartFixture + "container-previous.log"}

	out, err := run(t, append([]string{"-o", "json", "--attempt", "0"}, files...)...)
	if code, _ := Exit(err); code != 0 {
		t.Fatalf("--attempt 0: exit %d\n%s", code, out)
	}
	r := decodeReport(t, out)
	var init map[string]any
	for _, p := range r["phases"].([]any) {
		if p := p.(map[string]any); p["name"] == "container-init" {
			init = p
		}
	}
	if r["attempt"] != float64(0) || init["state"] != "complete" || init["start"] != "2026-10-26T11:50:43Z" || init["end"] != "2026-10-26T11:50:43.101003349Z" {
		t.Errorf("--attempt 0: attempt %v, container-init %v", r["attempt"], init)
	}

	out, _ = run(t, append([]string{"-o", "json", "--all-attempts"}, files...)...)
	if a, _ := decodeReport(t, out)["attempts"].([]any); len(a) != 2 {
		t.Errorf("--all-attempts: %d attempts, want 2", len(a))
	}

	t.Setenv("NO_COLOR", "1")
	out, _ = run(t, append([]string{"--all-attempts"}, files...)...)
	first, second := strings.Index(out, "attempt 0 of 2"), strings.Index(out, "attempt 1 of 2")
	if first < 0 || second < first || strings.Count(out, "Pod default/restart-once") != 2 {
		t.Errorf("--all-attempts text: want attempt 0 then attempt 1, each with its waterfall:\n%s", out)
	}
	out, _ = run(t, append([]string{"--attempt", "0"}, files...)...)
	if !strings.HasPrefix(out, "attempt 0 of 2\n") {
		t.Errorf("--attempt 0 text does not say which attempt:\n%s", out)
	}
}
