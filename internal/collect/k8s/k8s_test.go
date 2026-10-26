package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

const restart = "../../../packs/generic/testdata/kind-restart/"

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- a fixture path
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// fixtureLogs serves the kind-restart capture's logs, and records what was asked.
type fixtureLogs struct {
	t     *testing.T
	asked []corev1.PodLogOptions
	err   error
}

func (f *fixtureLogs) get(_ context.Context, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	f.asked = append(f.asked, *opts)
	if f.err != nil {
		return nil, f.err
	}
	name := "container.log"
	if opts.Previous {
		name = "container-previous.log"
	}
	b, err := os.ReadFile(restart + name)
	if err != nil {
		f.t.Fatal(err)
	}
	return io.NopCloser(strings.NewReader(string(b))), nil
}

// restartCluster is a fake API holding the real kind-restart pod and its events.
func restartCluster(t *testing.T) (*fake.Clientset, *corev1.Pod, *fixtureLogs) {
	t.Helper()
	var pod corev1.Pod
	readJSON(t, restart+"pod.json", &pod)
	var events corev1.EventList
	readJSON(t, restart+"events.json", &events)
	objs := []runtime.Object{&pod, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: pod.Spec.NodeName}}}
	for i := range events.Items {
		objs = append(objs, &events.Items[i])
	}
	return fake.NewClientset(objs...), &pod, &fixtureLogs{t: t}
}

// Only get and list, on the resources DESIGN.md section 7.4 grants.
func assertReadOnly(t *testing.T, cs *fake.Clientset) {
	t.Helper()
	if len(cs.Actions()) == 0 {
		t.Fatal("no API calls recorded")
	}
	for _, a := range cs.Actions() {
		if v := a.GetVerb(); v != "get" && v != "list" {
			t.Errorf("verb %s on %s: only get and list are allowed", v, a.GetResource().Resource)
		}
	}
}

// A pod by name: the pod, its events, its node, and its logs with runtime timestamps; the
// previous container's log too, because it restarted.
func TestCollectPod(t *testing.T) {
	cs, pod, logs := restartCluster(t)
	o, err := Collect(context.Background(), Client{API: cs, Logs: logs.get}, Target{Kind: KindPod, Namespace: "default", Name: "restart-once"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if o.Pod.UID != pod.UID || o.Node == nil || o.Node.Name != "doriki-control-plane" {
		t.Errorf("pod %s node %+v", o.Pod.UID, o.Node)
	}
	if len(o.Events) == 0 {
		t.Errorf("no events")
	}
	for _, e := range o.Events {
		if e.InvolvedObject.UID != pod.UID {
			t.Errorf("event about %s, not the pod", e.InvolvedObject.UID)
		}
	}
	if !strings.Contains(string(o.Log), "second start") || !strings.Contains(string(o.PreviousLog), "first start") {
		t.Errorf("log %q previous %q", o.Log, o.PreviousLog)
	}
	for _, a := range logs.asked {
		if !a.Timestamps || a.Container != "main" {
			t.Errorf("log request %+v, want timestamps for container main", a)
		}
	}
	if len(logs.asked) != 2 {
		t.Errorf("log requests = %d, want current and previous", len(logs.asked))
	}
	if len(o.Warnings) != 0 {
		t.Errorf("warnings = %v", o.Warnings)
	}
	assertReadOnly(t, cs)
}

func owned(kind, name string, uid types.UID) []metav1.OwnerReference {
	return []metav1.OwnerReference{{Kind: kind, Name: name, UID: uid, Controller: new(true)}}
}

func podAt(name string, created time.Time, owner []metav1.OwnerReference) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", UID: types.UID("uid-" + name), Labels: map[string]string{"app": "llm"},
			CreationTimestamp: metav1.NewTime(created), OwnerReferences: owner},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
	}
}

var t0 = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

// deploy/<name> is the newest pod of the Deployment: a pod of one of its own ReplicaSets, not
// one that merely matches the selector. sts/<name> likewise through the StatefulSet.
func TestResolveWorkloads(t *testing.T) {
	sel := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "llm"}}
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "ns", UID: "d1"}, Spec: appsv1.DeploymentSpec{Selector: sel}}
	rsOld := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "llm-a", Namespace: "ns", UID: "rs-a", Labels: map[string]string{"app": "llm"}, OwnerReferences: owned("Deployment", "llm", "d1")}}
	rsNew := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "llm-b", Namespace: "ns", UID: "rs-b", Labels: map[string]string{"app": "llm"}, OwnerReferences: owned("Deployment", "llm", "d1")}}
	rsForeign := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "other-x", Namespace: "ns", UID: "rs-x", Labels: map[string]string{"app": "llm"}, OwnerReferences: owned("Deployment", "other", "d2")}}
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "llm-sts", Namespace: "ns", UID: "s1"}, Spec: appsv1.StatefulSetSpec{Selector: sel}}
	cs := fake.NewClientset(deploy, rsOld, rsNew, rsForeign, sts,
		podAt("old", t0, owned("ReplicaSet", "llm-a", "rs-a")),
		podAt("new", t0.Add(time.Minute), owned("ReplicaSet", "llm-b", "rs-b")),
		podAt("foreign", t0.Add(time.Hour), owned("ReplicaSet", "other-x", "rs-x")),
		podAt("sts-0", t0.Add(2*time.Minute), owned("StatefulSet", "llm-sts", "s1")),
		podAt("sts-1", t0.Add(3*time.Minute), owned("StatefulSet", "llm-sts", "s1")),
	)
	c := Client{API: cs, Logs: (&fixtureLogs{t: t, err: errors.New("no log")}).get}
	for kind, want := range map[Kind]string{KindDeployment: "new", KindStatefulSet: "sts-1"} {
		name := "llm"
		if kind == KindStatefulSet {
			name = "llm-sts"
		}
		o, err := Collect(context.Background(), c, Target{Kind: kind, Namespace: "ns", Name: name}, time.Time{})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if o.Pod.Name != want {
			t.Errorf("%s/%s resolved to %s, want %s", kind, name, o.Pod.Name, want)
		}
	}
	assertReadOnly(t, cs)

	empty := fake.NewClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "idle", Namespace: "ns", UID: "d9"}, Spec: appsv1.DeploymentSpec{Selector: sel}})
	_, err := Collect(context.Background(), Client{API: empty}, Target{Kind: KindDeployment, Namespace: "ns", Name: "idle"}, time.Time{})
	var u *UnavailableError
	if !errors.As(err, &u) || !strings.Contains(err.Error(), "no pods") {
		t.Errorf("deployment without pods: err = %v, want an UnavailableError saying it has no pods", err)
	}
}

// The ReplicaSet's own owner is the pod's workload; a ReplicaSet without one is the workload
// itself. A forbidden ReplicaSet read leaves the owner to the collector's inference, with a
// warning.
func TestOwnerWalk(t *testing.T) {
	pod := podAt("p", t0, owned("ReplicaSet", "llm-b", "rs-b"))
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "llm-b", Namespace: "ns", UID: "rs-b", OwnerReferences: owned("Deployment", "llm", "d1")}}
	noLog := (&fixtureLogs{t: t, err: errors.New("no log")}).get
	target := Target{Kind: KindPod, Namespace: "ns", Name: "p"}

	o, err := Collect(context.Background(), Client{API: fake.NewClientset(pod, rs), Logs: noLog}, target, time.Time{})
	if err != nil || o.Owner != "Deployment/llm" {
		t.Errorf("owner = %q (%v), want Deployment/llm", o.Owner, err)
	}

	bare := rs.DeepCopy()
	bare.OwnerReferences = nil
	o, _ = Collect(context.Background(), Client{API: fake.NewClientset(pod, bare), Logs: noLog}, target, time.Time{})
	if o.Owner != "ReplicaSet/llm-b" {
		t.Errorf("owner of an unowned ReplicaSet = %q, want ReplicaSet/llm-b", o.Owner)
	}

	cs := fake.NewClientset(pod, rs)
	cs.PrependReactor("get", "replicasets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "replicasets"}, "llm-b", errors.New("rbac"))
	})
	o, err = Collect(context.Background(), Client{API: cs, Logs: noLog}, target, time.Time{})
	if err != nil || o.Owner != "" || !hasWarning(o.Warnings, "owner-unreadable") {
		t.Errorf("forbidden ReplicaSet: owner %q, err %v, warnings %v; want no owner and an owner-unreadable warning", o.Owner, err, o.Warnings)
	}
}

func hasWarning(ws []model.Warning, code string) bool {
	for _, w := range ws {
		if w.Code == code {
			return true
		}
	}
	return false
}

// The node is optional (DESIGN.md section 7.4): forbidden is a warning and no node. A log
// that cannot be read yet (the container is still starting) is a warning and an empty log.
func TestOptionalReads(t *testing.T) {
	cs, _, _ := restartCluster(t)
	cs.PrependReactor("get", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "doriki-control-plane", errors.New("rbac"))
	})
	logs := &fixtureLogs{t: t, err: apierrors.NewBadRequest(`container "main" in pod "restart-once" is waiting to start: ContainerCreating`)}
	o, err := Collect(context.Background(), Client{API: cs, Logs: logs.get}, Target{Kind: KindPod, Namespace: "default", Name: "restart-once"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if o.Node != nil || !hasWarning(o.Warnings, "node-unreadable") {
		t.Errorf("node %+v, warnings %v; want no node and node-unreadable", o.Node, o.Warnings)
	}
	if len(o.Log) != 0 || !hasWarning(o.Warnings, "log-unavailable") || !hasWarning(o.Warnings, "previous-log-unavailable") {
		t.Errorf("log %q, warnings %v; want empty and both log warnings", o.Log, o.Warnings)
	}
}

// Pod not found, events forbidden, or an unreachable API stop the run with an
// UnavailableError carrying the API's message (DESIGN.md section 7.3, exit 2).
func TestUnavailable(t *testing.T) {
	_, err := Collect(context.Background(), Client{API: fake.NewClientset()}, Target{Kind: KindPod, Namespace: "ns", Name: "nope"}, time.Time{})
	var u *UnavailableError
	if !errors.As(err, &u) || !strings.Contains(err.Error(), `"nope" not found`) {
		t.Errorf("missing pod: err = %v", err)
	}

	cs, _, logs := restartCluster(t)
	cs.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "events"}, "", errors.New("rbac"))
	})
	_, err = Collect(context.Background(), Client{API: cs, Logs: logs.get}, Target{Kind: KindPod, Namespace: "default", Name: "restart-once"}, time.Time{})
	if !errors.As(err, &u) || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("events forbidden: err = %v", err)
	}
}

// --since keeps the events last seen at or after it; the pod's own state is not filtered.
func TestSince(t *testing.T) {
	cs, _, logs := restartCluster(t)
	all, _ := Collect(context.Background(), Client{API: cs, Logs: logs.get}, Target{Kind: KindPod, Namespace: "default", Name: "restart-once"}, time.Time{})
	cut := time.Date(2026, 10, 26, 11, 50, 44, 0, time.UTC)
	some, _ := Collect(context.Background(), Client{API: cs, Logs: logs.get}, Target{Kind: KindPod, Namespace: "default", Name: "restart-once"}, cut)
	if len(some.Events) == 0 || len(some.Events) >= len(all.Events) {
		t.Fatalf("events since %v: %d of %d", cut, len(some.Events), len(all.Events))
	}
	for _, e := range some.Events {
		if lastSeen(e).Before(cut) {
			t.Errorf("event %s last seen %v kept", e.Reason, lastSeen(e))
		}
	}
}

// The k8s collector never names a mutating client-go call (M2 task acceptance 3). The fake
// clientset's recorded verbs prove what the tests exercise; this guards the code they do not.
func TestNoMutatingCalls(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files (%v)", err)
	}
	mutating := regexp.MustCompile(`\.(Create|Update|UpdateStatus|Patch|Apply|ApplyStatus|Delete|DeleteCollection|Evict|Bind)\(`)
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f) // #nosec G304 -- this package's own source files
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for i, line := range strings.Split(string(b), "\n") {
			if mutating.MatchString(line) {
				t.Errorf("%s:%d: mutating call: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test source files checked")
	}
}
