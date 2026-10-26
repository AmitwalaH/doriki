package files

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AmitwalaH/doriki/internal/model"
)

// maxLogLine is the longest log line read. Engine startup lines can be long (a full config
// dump), so this is well above bufio's 64 KiB default.
const maxLogLine = 1 << 20

// Load opens the three saved files and reads them with Read. Errors name the file.
func Load(podPath, eventsPath, logPath string) ([]model.Event, []model.Warning, error) {
	var readers [3]io.Reader
	for i, path := range []string{podPath, eventsPath, logPath} {
		f, err := os.Open(path) // #nosec G304 -- the user names these files on the command line
		if err != nil {
			return nil, nil, err
		}
		defer f.Close() //nolint:errcheck // read-only file
		readers[i] = namedReader{f, path}
	}
	return Read(readers[0], readers[1], readers[2])
}

// namedReader lets Read name the file in its errors.
type namedReader struct {
	io.Reader
	name string
}

func nameOf(r io.Reader, fallback string) string {
	if n, ok := r.(namedReader); ok {
		return n.name
	}
	return fallback
}

// Read turns a pod object (`kubectl get pod -o json`), its events (`kubectl get events -o
// json`, an EventList or a List) and the container's log (`kubectl logs`, timestamps
// optional) into events. Only the pod's first spec container is profiled.
func Read(pod, events, log io.Reader) ([]model.Event, []model.Warning, error) {
	var p corev1.Pod
	if err := json.NewDecoder(pod).Decode(&p); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", nameOf(pod, "pod"), err)
	}
	if len(p.Spec.Containers) == 0 {
		return nil, nil, fmt.Errorf("%s: pod %s has no containers", nameOf(pod, "pod"), p.Name)
	}
	container := p.Spec.Containers[0].Name

	var list corev1.EventList
	if err := json.NewDecoder(events).Decode(&list); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", nameOf(events, "events"), err)
	}

	out := podEvents(&p, container)
	out = append(out, k8sEvents(&p, container, list.Items)...)
	if !hasScheduled(out) {
		out = append(out, scheduledFallback(&p)...)
	}
	logEvents, err := logLines(log)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", nameOf(log, "log"), err)
	}
	return append(out, logEvents...), nil, nil
}

func event(ts time.Time, src model.Source, kind, raw string) model.Event {
	return model.Event{TS: ts.UTC(), Source: src, Kind: kind, Raw: model.TruncateRaw(raw), Confidence: 1}
}

// podEvents reads creation, the profiled container's starts and the Ready condition from
// the pod object.
func podEvents(p *corev1.Pod, container string) []model.Event {
	out := []model.Event{event(p.CreationTimestamp.Time, model.SourceContainerStatus, model.KindPodCreated, "pod created")}

	// The image is a fact of the spec, which is fixed when the pod is created.
	for _, c := range p.Spec.Containers {
		if c.Name == container {
			img := event(p.CreationTimestamp.Time, model.SourceManifest, model.KindContainerImage, "container "+container+" image "+c.Image)
			img.Attrs = map[string]string{model.AttrImage: c.Image}
			for k, v := range map[string]string{model.AttrNamespace: p.Namespace, model.AttrPod: p.Name, model.AttrNode: p.Spec.NodeName} {
				if v != "" {
					img.Attrs[k] = v
				}
			}
			out = append(out, img)
		}
	}

	for _, cs := range p.Status.ContainerStatuses {
		if cs.Name != container {
			continue
		}
		start := func(at metav1.Time, restartCount int32) {
			if at.IsZero() {
				return
			}
			e := event(at.Time, model.SourceContainerStatus, model.KindContainerStarted,
				fmt.Sprintf("container %s started (restartCount %d)", container, restartCount))
			e.Attrs = map[string]string{model.AttrRestartCount: strconv.Itoa(int(restartCount))}
			out = append(out, e)
		}
		switch {
		case cs.State.Running != nil:
			start(cs.State.Running.StartedAt, cs.RestartCount)
		case cs.State.Terminated != nil:
			start(cs.State.Terminated.StartedAt, cs.RestartCount)
		}
		// The pod object keeps only the previous attempt; older ones are gone.
		if cs.LastTerminationState.Terminated != nil && cs.RestartCount > 0 {
			start(cs.LastTerminationState.Terminated.StartedAt, cs.RestartCount-1)
		}
	}

	if c := condition(p, corev1.PodReady); c != nil {
		out = append(out, event(c.LastTransitionTime.Time, model.SourceContainerStatus, model.KindPodReady, "Ready condition True"))
	}
	return out
}

// condition returns the pod condition of this type if it is True and has a time.
func condition(p *corev1.Pod, t corev1.PodConditionType) *corev1.PodCondition {
	for i, c := range p.Status.Conditions {
		if c.Type == t && c.Status == corev1.ConditionTrue && !c.LastTransitionTime.IsZero() {
			return &p.Status.Conditions[i]
		}
	}
	return nil
}

// k8sEvents keeps the Event objects about this pod (matched by UID when both have one, so an
// older pod with the same name is excluded) and, for container-level events, about the
// profiled container only. An event's time is eventTime when set (microseconds), else
// firstTimestamp, else lastTimestamp; an event with none is dropped.
func k8sEvents(p *corev1.Pod, container string, items []corev1.Event) []model.Event {
	var out []model.Event
	for _, e := range items {
		obj := e.InvolvedObject
		if obj.Kind != "Pod" || obj.Name != p.Name || obj.Namespace != p.Namespace {
			continue
		}
		if obj.UID != "" && p.UID != "" && obj.UID != p.UID {
			continue
		}
		if obj.FieldPath != "" && obj.FieldPath != "spec.containers{"+container+"}" {
			continue
		}
		var ts time.Time
		switch {
		case !e.EventTime.IsZero():
			ts = e.EventTime.Time
		case !e.FirstTimestamp.IsZero():
			ts = e.FirstTimestamp.Time
		case !e.LastTimestamp.IsZero():
			ts = e.LastTimestamp.Time
		default:
			continue
		}
		ev := event(ts, model.SourceK8sEvent, e.Reason, e.Message)
		if e.Reason == model.KindPulled && strings.Contains(e.Message, cachedPullText) {
			ev.Attrs = map[string]string{model.AttrCached: "true"}
		}
		out = append(out, ev)
	}
	return out
}

// cachedPullText is how the kubelet's Pulled message says the image was already on the node
// (decision 0004). Kubelet 1.37 continues "... and can be accessed by the pod"; older ones
// stop here, so the match is on this part only.
const cachedPullText = "already present on machine"

func hasScheduled(events []model.Event) bool {
	for _, e := range events {
		if e.Kind == model.KindScheduled {
			return true
		}
	}
	return false
}

// scheduledFallback stands in for an expired Scheduled event with the PodScheduled
// condition's time (DESIGN.md section 4.2), so scheduling is not shown open on a running
// pod. It comes from the pod object, so its source says so.
func scheduledFallback(p *corev1.Pod) []model.Event {
	c := condition(p, corev1.PodScheduled)
	if c == nil {
		return nil
	}
	return []model.Event{event(c.LastTransitionTime.Time, model.SourceContainerStatus, model.KindScheduled, "PodScheduled condition True")}
}

// logLines reads `kubectl logs` output. A line counts as timed when it starts with an
// RFC 3339 timestamp and a space, as `kubectl logs --timestamps` writes it. An untimed line
// is kept whole, with a zero time and AttrUntimed, for enginelog to date from the engine's
// own timestamp or drop; the collector knows no pack, so it cannot tell which.
func logLines(r io.Reader) ([]model.Event, error) {
	var out []model.Event
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLogLine)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if line == "" {
			continue
		}
		prefix, rest, ok := strings.Cut(line, " ")
		ts, err := time.Parse(time.RFC3339Nano, prefix)
		if !ok || err != nil {
			e := event(time.Time{}, model.SourceEngineLog, model.KindLogLine, line)
			e.Attrs = map[string]string{model.AttrUntimed: "true"}
			e.Line = line
			out = append(out, e)
			continue
		}
		e := event(ts, model.SourceEngineLog, model.KindLogLine, rest)
		e.Line = rest
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, fmt.Errorf("a log line is longer than %d bytes", maxLogLine)
		}
		return nil, err
	}
	return out, nil
}
