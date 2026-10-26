package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/AmitwalaH/doriki/internal/collect/k8s"
)

const vllmFixture = "../../packs/vllm/testdata/0.32/"

// bootingVLLM serves the real vLLM capture as a pod that is still booting: Ready is not set
// yet, and the log is a pipe the test writes to. ready() sets the Ready condition the capture
// recorded. A one-shot run on the same client reads the whole capture log.
type bootingVLLM struct {
	dir   string
	cs    *fake.Clientset
	pod   corev1.Pod
	mu    sync.Mutex
	pipe  *io.PipeWriter
	piped chan struct{}
}

func newBootingVLLM(t *testing.T) *bootingVLLM { return newBooting(t, vllmFixture) }

// newBooting serves any capture the same way.
func newBooting(t *testing.T, dir string) *bootingVLLM {
	t.Helper()
	b := &bootingVLLM{dir: dir, piped: make(chan struct{}, 4)}
	for name, v := range map[string]any{"pod.json": &b.pod} {
		raw, err := os.ReadFile(dir + name) // #nosec G304 -- a fixture path
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, v); err != nil {
			t.Fatal(err)
		}
	}
	var events corev1.EventList
	raw, err := os.ReadFile(dir + "events.json") // #nosec G304 -- a fixture path
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	booting := b.pod.DeepCopy()
	var conds []corev1.PodCondition
	for _, c := range booting.Status.Conditions {
		if c.Type != corev1.PodReady && c.Type != corev1.ContainersReady {
			conds = append(conds, c)
		}
	}
	booting.Status.Conditions = conds
	objs := []runtime.Object{booting}
	for i := range events.Items {
		objs = append(objs, &events.Items[i])
	}
	b.cs = fake.NewClientset(objs...)

	saved, savedEvery := liveClient, followEvery
	t.Cleanup(func() { liveClient, followEvery = saved, savedEvery })
	followEvery = 10 * time.Millisecond
	liveClient = func(*genericclioptions.ConfigFlags) (k8s.Client, string, error) {
		return k8s.Client{API: b.cs, Logs: b.logs}, "default", nil
	}
	return b
}

func (b *bootingVLLM) logs(_ context.Context, _, _ string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	if !opts.Follow {
		return os.Open(b.dir + "container.log") // #nosec G304 -- a fixture path
	}
	r, w := io.Pipe()
	b.mu.Lock()
	b.pipe = w
	b.mu.Unlock()
	b.piped <- struct{}{}
	return r, nil
}

// writeLog streams the whole capture log into the followed stream.
func (b *bootingVLLM) writeLog(t *testing.T) {
	t.Helper()
	select {
	case <-b.piped:
	case <-time.After(3 * time.Second):
		t.Fatal("--follow did not open the log stream")
	}
	raw, err := os.ReadFile(b.dir + "container.log") // #nosec G304 -- a fixture path
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	w := b.pipe
	b.mu.Unlock()
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
}

func (b *bootingVLLM) ready(t *testing.T) {
	t.Helper()
	if _, err := b.cs.CoreV1().Pods(b.pod.Namespace).UpdateStatus(context.Background(), &b.pod, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}

type result struct {
	out  string
	code int
	msg  string
}

func runAsync(t *testing.T, args ...string) <-chan result {
	t.Helper()
	done := make(chan result, 1)
	go func() {
		out, err := run(t, args...)
		code, msg := Exit(err)
		done <- result{out, code, msg}
	}()
	return done
}

func notYet(t *testing.T, done <-chan result, why string) {
	t.Helper()
	select {
	case r := <-done:
		t.Fatalf("--follow ended %s: exit %d %q\n%s", why, r.code, r.msg, r.out)
	case <-time.After(150 * time.Millisecond):
	}
}

func finish(t *testing.T, done <-chan result) result {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("--follow did not end")
		return result{}
	}
}

// --follow streams the log and re-reads the pod: the whole log is not enough, because
// server-ready ends at the Ready condition; once Ready is set it stops with exit 0. Off a
// terminal it prints the final report once, and that report is the one-shot report of the
// same pod (M2 task acceptance 6).
func TestFollowUntilReady(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	b := newBootingVLLM(t)
	done := runAsync(t, "--follow", "pod/vllm-785fb8997d-bvsz4")

	notYet(t, done, "before any log")
	b.writeLog(t)
	notYet(t, done, "before the pod was Ready")
	b.ready(t)
	r := finish(t, done)
	if r.code != 0 {
		t.Fatalf("exit %d %q\n%s", r.code, r.msg, r.out)
	}

	oneShot, err := run(t, "pod/vllm-785fb8997d-bvsz4")
	if err != nil {
		t.Fatal(err)
	}
	if r.out != oneShot {
		t.Errorf("--follow's final report differs from a one-shot run:\nfollow:\n%s\none-shot:\n%s", r.out, oneShot)
	}
	if strings.Contains(r.out, clearScreen) {
		t.Errorf("redraw codes written off a terminal")
	}
}

// --timeout ends --follow with the report as it stands and the one-shot exit code: 1 while
// server-ready is still open.
func TestFollowTimeout(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	b := newBootingVLLM(t)
	done := runAsync(t, "--follow", "--timeout", "400ms", "pod/vllm-785fb8997d-bvsz4")
	b.writeLog(t)
	r := finish(t, done)
	if r.code != ExitIncomplete || strings.Count(r.out, "Pod default/vllm-785fb8997d-bvsz4") != 1 || !strings.Contains(r.out, "weight-load") {
		t.Errorf("exit %d, want %d with one report showing the log's phases:\n%s", r.code, ExitIncomplete, r.out)
	}
}

// On a terminal each change redraws the whole waterfall in place, and an unchanged state is
// not drawn again; the last frame is the final report.
func TestFollowRedraws(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	saved := isTerminal
	t.Cleanup(func() { isTerminal = saved })
	isTerminal = func(io.Writer) bool { return true }
	b := newBootingVLLM(t)
	done := runAsync(t, "--follow", "pod/vllm-785fb8997d-bvsz4")
	notYet(t, done, "before any log")
	b.writeLog(t)
	notYet(t, done, "before the pod was Ready")
	b.ready(t)
	r := finish(t, done)

	frames := strings.Split(strings.TrimPrefix(r.out, clearScreen), clearScreen)
	if !strings.HasPrefix(r.out, clearScreen) || len(frames) < 3 {
		t.Fatalf("want at least 3 frames (booting, log read, Ready), got %d:\n%q", len(frames), r.out)
	}
	for i := 1; i < len(frames); i++ {
		if frames[i] == frames[i-1] {
			t.Errorf("frame %d repeats frame %d", i, i-1)
		}
	}
	oneShot, _ := run(t, "pod/vllm-785fb8997d-bvsz4")
	if frames[len(frames)-1] != oneShot {
		t.Errorf("last frame is not the final report:\n%s", frames[len(frames)-1])
	}
}

// --follow watches a live pod: not files, and only with a positive timeout.
func TestFollowUsage(t *testing.T) {
	for name, args := range map[string][]string{
		"with files":   {"--follow", "--from-files", fixtureArgs[0], fixtureArgs[1], fixtureArgs[2]},
		"zero timeout": {"--follow", "--timeout", "0s", "pod/x"},
	} {
		_, err := run(t, args...)
		if code, msg := Exit(err); code != ExitUsage || !strings.Contains(msg, "--follow") && !strings.Contains(msg, "--timeout") {
			t.Errorf("%s: exit %d %q, want %d", name, code, msg, ExitUsage)
		}
	}
}

// Under the generic pack every expected phase is complete at the first log line, before the
// pod is Ready; --follow still waits for Ready, so the report has its total.
func TestFollowGenericWaitsForReady(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	b := newBooting(t, fixture)
	done := runAsync(t, "--follow", "pod/nginx-89d46f4b4-ffrkt")
	b.writeLog(t)
	notYet(t, done, "before the pod was Ready, with every expected phase complete")
	b.ready(t)
	r := finish(t, done)
	if r.code != 0 || !strings.Contains(r.out, "total 0:21") {
		t.Errorf("exit %d, want 0 and the report's total:\n%s", r.code, r.out)
	}
}
