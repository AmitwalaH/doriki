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

// Load opens the saved files and reads them with Read: pod, events and log, and optionally the
// previous container's log. Errors name the file.
func Load(podPath, eventsPath, logPath string, previousLogPath ...string) ([]model.Event, []model.Warning, error) {
	paths := append([]string{podPath, eventsPath, logPath}, previousLogPath...)
	readers := make([]io.Reader, len(paths))
	for i, path := range paths {
		f, err := os.Open(path) // #nosec G304 -- the user names these files on the command line
		if err != nil {
			return nil, nil, err
		}
		defer f.Close() //nolint:errcheck // read-only file
		readers[i] = namedReader{f, path}
	}
	return Read(readers[0], readers[1], readers[2], readers[3:]...)
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
//
// previousLog, at most one, is the previous container's log (`kubectl logs --previous`). Its
// lines are tagged with that container's attempt (AttrRestartCount) so normalize and phase
// keep them in it; the current log's lines stay untagged.
func Read(pod, events, log io.Reader, previousLog ...io.Reader) ([]model.Event, []model.Warning, error) {
	if len(previousLog) > 1 {
		return nil, nil, fmt.Errorf("one previous log at most, got %d", len(previousLog))
	}
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
	logEvents, err := logLines(log, "")
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", nameOf(log, "log"), err)
	}
	out = append(out, logEvents...)

	if len(previousLog) == 1 {
		restarts := restartCount(&p, container)
		if restarts < 1 {
			return nil, nil, fmt.Errorf("%s: container %s has not restarted, so it has no previous log", nameOf(previousLog[0], "previous log"), container)
		}
		prev, err := logLines(previousLog[0], strconv.Itoa(int(restarts-1)))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", nameOf(previousLog[0], "previous log"), err)
		}
		out = append(out, prev...)
	}
	return out, nil, nil
}

// restartCount is the profiled container's restartCount, or 0 without a status.
func restartCount(p *corev1.Pod, container string) int32 {
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Name == container {
			return cs.RestartCount
		}
	}
	return 0
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
		// The pod object keeps only the previous attempt; older ones are gone. Its end is where
		// the next attempt starts: a re-pull before the restart belongs to the restart.
		if last := cs.LastTerminationState.Terminated; last != nil && cs.RestartCount > 0 {
			start(last.StartedAt, cs.RestartCount-1)
			if !last.FinishedAt.IsZero() {
				e := event(last.FinishedAt.Time, model.SourceContainerStatus, model.KindContainerTerminated,
					fmt.Sprintf("container %s terminated (restartCount %d, exit code %d)", container, cs.RestartCount-1, last.ExitCode))
				e.Attrs = map[string]string{model.AttrRestartCount: strconv.Itoa(int(cs.RestartCount - 1))}
				out = append(out, e)
			}
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
//
// The kubelet merges repeats of an event into one with a count. For the image-pull events, a
// merged event is also emitted at its lastTimestamp, so the latest attempt sees its own pull;
// a pull between the first and the last is not recoverable, so earlier restarts' pulls are
// approximate. Other merged events keep only their first time.
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
		if (e.Reason == model.KindPulling || e.Reason == model.KindPulled) && e.Count > 1 && e.LastTimestamp.After(ts) {
			last := ev
			last.TS = e.LastTimestamp.UTC()
			out = append(out, last)
		}
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
// own timestamp or drop; the collector knows no pack, so it cannot tell which. A non-empty
// attempt tags every line with AttrRestartCount.
func logLines(r io.Reader, attempt string) ([]model.Event, error) {
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
			out = append(out, tagAttempt(e, attempt))
			continue
		}
		e := event(ts, model.SourceEngineLog, model.KindLogLine, rest)
		e.Line = rest
		out = append(out, tagAttempt(e, attempt))
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, fmt.Errorf("a log line is longer than %d bytes", maxLogLine)
		}
		return nil, err
	}
	return out, nil
}

func tagAttempt(e model.Event, attempt string) model.Event {
	if attempt == "" {
		return e
	}
	if e.Attrs == nil {
		e.Attrs = map[string]string{}
	}
	e.Attrs[model.AttrRestartCount] = attempt
	return e
}
