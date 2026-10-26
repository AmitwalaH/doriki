package k8s

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// streamLogs serves a followed log from a pipe the test writes to, one pipe per container
// start, and a fixed previous log.
type streamLogs struct {
	mu       sync.Mutex
	pipes    []*io.PipeWriter
	opened   chan struct{}
	previous string
	asked    []corev1.PodLogOptions
}

func newStreamLogs() *streamLogs { return &streamLogs{opened: make(chan struct{}, 8)} }

func (s *streamLogs) get(_ context.Context, _, _ string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, *opts)
	if opts.Previous {
		return io.NopCloser(strings.NewReader(s.previous)), nil
	}
	if !opts.Follow {
		return nil, errors.New("want a followed log")
	}
	r, w := io.Pipe()
	s.pipes = append(s.pipes, w)
	s.opened <- struct{}{}
	return r, nil
}

func (s *streamLogs) write(t *testing.T, line string) {
	t.Helper()
	s.mu.Lock()
	w := s.pipes[len(s.pipes)-1]
	s.mu.Unlock()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

// waitOpened waits for Follow to open a log stream.
func waitOpened(t *testing.T, logs *streamLogs) {
	t.Helper()
	select {
	case <-logs.opened:
	case <-time.After(3 * time.Second):
		t.Fatal("no log stream opened")
	}
}

func followPod(restarts int32) *corev1.Pod {
	p := podAt("p", t0, nil)
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "main", RestartCount: restarts,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(t0.Add(time.Second))}}}}
	return p
}

// follow runs Follow in the background and hands the test each update.
func follow(t *testing.T, c Client, target Target) (<-chan Objects, func() error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updates := make(chan Objects, 64)
	done := make(chan error, 1)
	go func() {
		done <- Follow(ctx, c, target, time.Time{}, 10*time.Millisecond, func(o Objects) bool {
			updates <- o
			return strings.Contains(string(o.Log), "STOP")
		})
	}()
	return updates, func() error { defer cancel(); return <-done }
}

// waitFor reads updates until one satisfies ok.
func waitFor(t *testing.T, updates <-chan Objects, what string, ok func(Objects) bool) Objects {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case o := <-updates:
			if ok(o) {
				return o
			}
		case <-deadline:
			t.Fatalf("no update with %s", what)
		}
	}
}

// The log streams: each line the container writes reaches an update, whole lines only, and
// the pod's status changes reach one too. Follow returns when the caller says stop.
func TestFollowStreams(t *testing.T) {
	cs := fake.NewClientset(followPod(0))
	logs := newStreamLogs()
	updates, wait := follow(t, Client{API: cs, Logs: logs.get}, Target{Kind: KindPod, Namespace: "ns", Name: "p"})
	waitOpened(t, logs)

	logs.write(t, "2026-10-26T12:00:02Z first line")
	waitFor(t, updates, "the first line", func(o Objects) bool { return strings.Contains(string(o.Log), "first line") })

	s := logs.pipes[0]
	if _, err := io.WriteString(s, "2026-10-26T12:00:03Z half"); err != nil {
		t.Fatal(err)
	}
	ready := followPod(0)
	ready.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	if _, err := cs.CoreV1().Pods("ns").UpdateStatus(context.Background(), ready, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	o := waitFor(t, updates, "the Ready condition", func(o Objects) bool { return len(o.Pod.Status.Conditions) == 1 })
	if strings.Contains(string(o.Log), "half") {
		t.Errorf("a partial line was passed on: %q", o.Log)
	}

	logs.write(t, " a line\n2026-10-26T12:00:04Z STOP")
	if err := wait(); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	for _, a := range logs.asked {
		if !a.Timestamps || a.Container != "main" {
			t.Errorf("log request %+v, want timestamps for container main", a)
		}
	}
	assertFollowReadOnly(t, cs)
}

// The fake records the test's own UpdateStatus; everything Follow did is get, list or watch-free.
func assertFollowReadOnly(t *testing.T, cs *fake.Clientset) {
	t.Helper()
	for _, a := range cs.Actions() {
		if v := a.GetVerb(); v != "get" && v != "list" && (v != "update" || a.GetSubresource() != "status") {
			t.Errorf("verb %s on %s", v, a.GetResource().Resource)
		}
	}
}

// A restart mid-follow: the old stream ends, the previous container's log is read, and the
// new container's log starts empty instead of being appended to the old one.
func TestFollowRestart(t *testing.T) {
	cs := fake.NewClientset(followPod(0))
	logs := newStreamLogs()
	logs.previous = "2026-10-26T12:00:02Z old container\n"
	updates, wait := follow(t, Client{API: cs, Logs: logs.get}, Target{Kind: KindPod, Namespace: "ns", Name: "p"})
	waitOpened(t, logs)
	logs.write(t, "2026-10-26T12:00:02Z old container")
	waitFor(t, updates, "the first container's line", func(o Objects) bool { return len(o.Log) > 0 })

	if _, err := cs.CoreV1().Pods("ns").UpdateStatus(context.Background(), followPod(1), metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	_ = logs.pipes[0].Close() // the first container exited
	waitOpened(t, logs)
	o := waitFor(t, updates, "the restart", func(o Objects) bool { return o.PreviousLog != nil })
	if strings.Contains(string(o.Log), "old container") || string(o.PreviousLog) != logs.previous {
		t.Errorf("after the restart: log %q, previous %q", o.Log, o.PreviousLog)
	}
	logs.write(t, "2026-10-26T12:00:09Z new container STOP")
	if err := wait(); err != nil {
		t.Fatal(err)
	}
}

// Following a Deployment that has no pod yet waits for one: --follow is run right after an
// apply. A name that does not exist at all still stops at once, so a typo does not wait out
// the timeout.
func TestFollowWaitsForPod(t *testing.T) {
	sel := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "llm"}}
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "ns", UID: "d1"}, Spec: appsv1.DeploymentSpec{Selector: sel}}
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "llm-b", Namespace: "ns", UID: "rs-b", Labels: map[string]string{"app": "llm"}, OwnerReferences: owned("Deployment", "llm", "d1")}}
	cs := fake.NewClientset(deploy, rs)
	logs := newStreamLogs()
	updates, wait := follow(t, Client{API: cs, Logs: logs.get}, Target{Kind: KindDeployment, Namespace: "ns", Name: "llm"})
	time.Sleep(50 * time.Millisecond)
	pod := followPod(0)
	pod.OwnerReferences = owned("ReplicaSet", "llm-b", "rs-b")
	if _, err := cs.CoreV1().Pods("ns").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	waitOpened(t, logs)
	logs.write(t, "2026-10-26T12:00:02Z STOP")
	waitFor(t, updates, "the pod", func(o Objects) bool { return o.Pod != nil && o.Pod.Name == "p" })
	if err := wait(); err != nil {
		t.Fatal(err)
	}

	err := Follow(context.Background(), Client{API: fake.NewClientset(), Logs: logs.get}, Target{Kind: KindPod, Namespace: "ns", Name: "typo"},
		time.Time{}, 10*time.Millisecond, func(Objects) bool { return true })
	var u *UnavailableError
	if !errors.As(err, &u) {
		t.Errorf("following a pod that does not exist: err = %v, want an UnavailableError at once", err)
	}
}

// When the context ends (--timeout), Follow returns its error after the updates it sent.
func TestFollowContextEnds(t *testing.T) {
	cs := fake.NewClientset(followPod(0))
	logs := newStreamLogs()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	n := 0
	err := Follow(ctx, Client{API: cs, Logs: logs.get}, Target{Kind: KindPod, Namespace: "ns", Name: "p"}, time.Time{}, 10*time.Millisecond,
		func(Objects) bool { n++; return false })
	if !errors.Is(err, context.DeadlineExceeded) || n == 0 {
		t.Errorf("err %v after %d updates, want the deadline after at least one", err, n)
	}
}
