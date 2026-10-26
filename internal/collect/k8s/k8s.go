package k8s

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Kind is what a Target names.
type Kind string

const (
	KindPod         Kind = "pod"
	KindDeployment  Kind = "deployment"  // its newest pod
	KindStatefulSet Kind = "statefulset" // its newest pod
)

// Target is the pod to profile, or the workload whose newest pod to profile.
type Target struct {
	Kind            Kind
	Namespace, Name string
}

// LogFunc returns a pod's container log, as the API's pods/log does.
type LogFunc func(ctx context.Context, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error)

// Client is the cluster access the collector needs. Logs defaults to the API's pods/log.
type Client struct {
	API  kubernetes.Interface
	Logs LogFunc
}

// Objects are what the collector read about one pod. Node, Owner and PreviousLog are
// optional: what could not be read is left empty and named in Warnings.
type Objects struct {
	Pod    *corev1.Pod
	Events []corev1.Event
	Node   *corev1.Node
	// Owner is the workload, Kind/name, from walking the pod's ReplicaSet to its Deployment;
	// empty when the pod has no ReplicaSet or it could not be read.
	Owner       string
	Log         []byte
	PreviousLog []byte
	Warnings    []model.Warning
}

// UnavailableError is a pod that cannot be profiled: not found, forbidden, or an API that
// cannot be reached (DESIGN.md section 7.3, exit 2). It carries the API's own message.
type UnavailableError struct{ Err error }

func (e *UnavailableError) Error() string { return e.Err.Error() }
func (e *UnavailableError) Unwrap() error { return e.Err }

func unavailable(format string, args ...any) error {
	return &UnavailableError{fmt.Errorf(format, args...)}
}

// Collect reads one pod's objects and logs from the API. It only gets and lists (DESIGN.md
// sections 7.4 and 10.3). notBefore, when set, drops events last seen before it (--since).
func Collect(ctx context.Context, c Client, t Target, notBefore time.Time) (Objects, error) {
	if c.Logs == nil {
		c.Logs = func(ctx context.Context, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
			return c.API.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
		}
	}
	pod, err := resolve(ctx, c.API, t)
	if err != nil {
		return Objects{}, err
	}
	o := Objects{Pod: pod}

	selector := fields.OneTermEqualSelector("involvedObject.uid", string(pod.UID)).String()
	list, err := c.API.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{FieldSelector: selector})
	if err != nil {
		return Objects{}, unavailable("events of pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	for _, e := range list.Items {
		if e.InvolvedObject.UID == pod.UID && !lastSeen(e).Before(notBefore) {
			o.Events = append(o.Events, e)
		}
	}

	o.Node = node(ctx, c.API, pod, &o.Warnings)
	o.Owner = owner(ctx, c.API, pod, &o.Warnings)
	o.Log, o.PreviousLog = logs(ctx, c.Logs, pod, &o.Warnings)
	return o, nil
}

// lastSeen is an event's latest time: what --since compares.
func lastSeen(e corev1.Event) time.Time {
	return slices.MaxFunc([]time.Time{e.EventTime.Time, e.FirstTimestamp.Time, e.LastTimestamp.Time}, time.Time.Compare)
}

// resolve finds the pod: by name, or the newest pod owned by the Deployment (through one of
// its ReplicaSets) or the StatefulSet. A pod that only matches the selector is not owned.
func resolve(ctx context.Context, api kubernetes.Interface, t Target) (*corev1.Pod, error) {
	switch t.Kind {
	case KindPod:
		pod, err := api.CoreV1().Pods(t.Namespace).Get(ctx, t.Name, metav1.GetOptions{})
		if err != nil {
			return nil, unavailable("%w", err)
		}
		return pod, nil
	case KindDeployment:
		d, err := api.AppsV1().Deployments(t.Namespace).Get(ctx, t.Name, metav1.GetOptions{})
		if err != nil {
			return nil, unavailable("%w", err)
		}
		sel, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
		if err != nil {
			return nil, unavailable("deployment %s/%s: %w", t.Namespace, t.Name, err)
		}
		sets, err := api.AppsV1().ReplicaSets(t.Namespace).List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
		if err != nil {
			return nil, unavailable("replicasets of deployment %s/%s: %w", t.Namespace, t.Name, err)
		}
		owners := map[types.UID]bool{}
		for _, rs := range sets.Items {
			if controller(rs.OwnerReferences) == d.UID {
				owners[rs.UID] = true
			}
		}
		return newestPod(ctx, api, t, sel, owners)
	case KindStatefulSet:
		s, err := api.AppsV1().StatefulSets(t.Namespace).Get(ctx, t.Name, metav1.GetOptions{})
		if err != nil {
			return nil, unavailable("%w", err)
		}
		sel, err := metav1.LabelSelectorAsSelector(s.Spec.Selector)
		if err != nil {
			return nil, unavailable("statefulset %s/%s: %w", t.Namespace, t.Name, err)
		}
		return newestPod(ctx, api, t, sel, map[types.UID]bool{s.UID: true})
	}
	return nil, fmt.Errorf("unknown target kind %q", t.Kind)
}

func newestPod(ctx context.Context, api kubernetes.Interface, t Target, sel labels.Selector, owners map[types.UID]bool) (*corev1.Pod, error) {
	pods, err := api.CoreV1().Pods(t.Namespace).List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
	if err != nil {
		return nil, unavailable("pods of %s %s/%s: %w", t.Kind, t.Namespace, t.Name, err)
	}
	var mine []corev1.Pod
	for _, p := range pods.Items {
		if owners[controller(p.OwnerReferences)] {
			mine = append(mine, p)
		}
	}
	if len(mine) == 0 {
		return nil, unavailable("%s %s/%s has no pods", t.Kind, t.Namespace, t.Name)
	}
	newest := slices.MaxFunc(mine, func(a, b corev1.Pod) int {
		return cmp.Or(a.CreationTimestamp.Compare(b.CreationTimestamp.Time), cmp.Compare(a.Name, b.Name))
	})
	return &newest, nil
}

// controller is the UID of the controlling owner, or "".
func controller(refs []metav1.OwnerReference) types.UID {
	for _, r := range refs {
		if r.Controller != nil && *r.Controller {
			return r.UID
		}
	}
	return ""
}

// node reads the pod's node. It is optional (DESIGN.md section 7.4): without it the manifest
// has no node labels, and a warning says why.
func node(ctx context.Context, api kubernetes.Interface, pod *corev1.Pod, warnings *[]model.Warning) *corev1.Node {
	if pod.Spec.NodeName == "" {
		return nil
	}
	n, err := api.CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		*warnings = append(*warnings, model.Warning{Code: "node-unreadable",
			Text: fmt.Sprintf("node %s could not be read, so the manifest has no instance type, zone or GPU product: %v", pod.Spec.NodeName, err)})
		return nil
	}
	return n
}

// owner walks a ReplicaSet to the Deployment that controls it. Other owner kinds are not
// looked up: the collector records them from ownerReferences.
func owner(ctx context.Context, api kubernetes.Interface, pod *corev1.Pod, warnings *[]model.Warning) string {
	var rsName string
	for _, r := range pod.OwnerReferences {
		if r.Controller != nil && *r.Controller && r.Kind == "ReplicaSet" {
			rsName = r.Name
		}
	}
	if rsName == "" {
		return ""
	}
	rs, err := api.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, rsName, metav1.GetOptions{})
	if err != nil {
		*warnings = append(*warnings, model.Warning{Code: "owner-unreadable",
			Text: fmt.Sprintf("replicaset %s could not be read, so the owner is inferred from its name: %v", rsName, err)})
		return ""
	}
	for _, r := range rs.OwnerReferences {
		if r.Controller != nil && *r.Controller {
			return r.Kind + "/" + r.Name
		}
	}
	return "ReplicaSet/" + rs.Name
}

// logs reads the profiled container's log with runtime timestamps, and the previous
// container's when it restarted. A log that cannot be read (the container has not started,
// or the previous one is gone) is a warning and no log, not a failed run.
func logs(ctx context.Context, get LogFunc, pod *corev1.Pod, warnings *[]model.Warning) (current, previous []byte) {
	if len(pod.Spec.Containers) == 0 {
		return nil, nil
	}
	container := pod.Spec.Containers[0].Name
	read := func(prev bool) ([]byte, error) {
		r, err := get(ctx, pod.Namespace, pod.Name, &corev1.PodLogOptions{Container: container, Timestamps: true, Previous: prev})
		if err != nil {
			return nil, err
		}
		defer r.Close() //nolint:errcheck // read-only stream
		return io.ReadAll(r)
	}
	current, err := read(false)
	if err != nil {
		*warnings = append(*warnings, model.Warning{Code: "log-unavailable",
			Text: fmt.Sprintf("the log of container %s could not be read yet, so engine phases are not shown: %v", container, apiMessage(err))})
		current = nil
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name != container || cs.RestartCount == 0 {
			continue
		}
		previous, err = read(true)
		if err != nil {
			*warnings = append(*warnings, model.Warning{Code: "previous-log-unavailable",
				Text: fmt.Sprintf("the previous container's log could not be read, so earlier attempts have no engine lines: %v", apiMessage(err))})
			previous = nil
		}
	}
	return current, previous
}

func apiMessage(err error) string {
	if s, ok := err.(apierrors.APIStatus); ok {
		return s.Status().Message
	}
	return err.Error()
}
