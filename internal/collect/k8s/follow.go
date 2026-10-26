package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Follow keeps one pod's objects current (DESIGN.md section 3, --follow): the container's log
// streams as it is written, and the pod and its events are read again every interval, since
// server-ready ends at the Ready condition, which no log line says. update gets the objects
// each time they change and returns true to stop. A workload with no pod yet is waited for;
// anything else the API refuses ends Follow with an UnavailableError, and the end of ctx with
// its error. It only gets and lists, like Collect.
func Follow(ctx context.Context, c Client, t Target, notBefore time.Time, every time.Duration, update func(Objects) bool) error {
	c = c.withLogs()
	tick := time.NewTicker(every)
	defer tick.Stop()

	var pod *corev1.Pod
	for pod == nil {
		p, err := resolve(ctx, c.API, t)
		switch {
		case err == nil:
			pod = p
		case !errors.Is(err, errNoPods):
			return err
		default:
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tick.C:
			}
		}
	}

	var warnings []model.Warning
	node := node(ctx, c.API, pod, &warnings)
	owner := owner(ctx, c.API, pod, &warnings)
	s := &stream{get: c.Logs, grew: make(chan struct{}, 1)}
	defer s.stop()

	var sent string
	for {
		s.sync(ctx, pod)
		events, err := podEvents(ctx, c.API, pod, notBefore)
		if err != nil {
			return err
		}
		log, previous, logWarnings := s.snapshot()
		o := Objects{Pod: pod, Events: events, Node: node, Owner: owner, Log: log, PreviousLog: previous,
			Warnings: slices.Concat(warnings, logWarnings)}
		if key := changeKey(o); key != sent {
			sent = key
			if update(o) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		case <-s.grew:
		}
		if pod, err = c.API.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{}); err != nil {
			return unavailable("%w", err)
		}
	}
}

// changeKey identifies what update would see, so an unchanged state is not sent twice.
func changeKey(o Objects) string {
	b, _ := json.Marshal(struct { // status, events and lengths: plain data that always marshals
		Status   corev1.PodStatus
		Events   []corev1.Event
		Log      int
		Previous int
		Warnings []model.Warning
	}{o.Pod.Status, o.Events, len(o.Log), len(o.PreviousLog), o.Warnings})
	return string(b)
}

// stream is the followed log of a pod's profiled container. When the container restarts the
// stream starts over for the new one, and the previous container's log is read once.
type stream struct {
	get  LogFunc
	grew chan struct{} // signalled when the log grows or the stream ends

	mu          sync.Mutex
	gen         int // bumped when a stream is replaced; an old reader's late writes are dropped
	open        bool
	cancel      context.CancelFunc
	buf         []byte
	synced      bool
	restarts    int32
	previous    []byte
	logWarn     *model.Warning
	previousErr *model.Warning
}

// sync opens the followed log when it is not open (the container may not have started yet,
// or the API ended the stream), and starts over when the restart count moved.
func (s *stream) sync(ctx context.Context, pod *corev1.Pod) {
	if len(pod.Spec.Containers) == 0 {
		return
	}
	container := pod.Spec.Containers[0].Name
	var restarts int32
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == container {
			restarts = cs.RestartCount
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.synced || restarts != s.restarts {
		if s.synced {
			s.stopLocked()
		}
		s.synced, s.restarts, s.previous, s.previousErr = true, restarts, nil, nil
		if restarts > 0 {
			s.previous, s.previousErr = s.readPrevious(ctx, pod, container)
		}
	}
	if s.open {
		return
	}
	sctx, cancel := context.WithCancel(ctx)
	r, err := s.get(sctx, pod.Namespace, pod.Name, &corev1.PodLogOptions{Container: container, Timestamps: true, Follow: true})
	if err != nil {
		cancel()
		s.logWarn = &model.Warning{Code: "log-unavailable",
			Text: fmt.Sprintf("the log of container %s could not be read yet, so engine phases are not shown: %v", container, apiMessage(err))}
		return
	}
	s.gen++
	s.open, s.cancel, s.buf, s.logWarn = true, cancel, nil, nil
	go s.read(r, s.gen)
}

func (s *stream) readPrevious(ctx context.Context, pod *corev1.Pod, container string) ([]byte, *model.Warning) {
	r, err := s.get(ctx, pod.Namespace, pod.Name, &corev1.PodLogOptions{Container: container, Timestamps: true, Previous: true})
	if err == nil {
		defer r.Close() //nolint:errcheck // read-only stream
		var b []byte
		if b, err = io.ReadAll(r); err == nil {
			return b, nil
		}
	}
	return nil, &model.Warning{Code: "previous-log-unavailable",
		Text: fmt.Sprintf("the previous container's log could not be read, so earlier attempts have no engine lines: %v", apiMessage(err))}
}

// read copies one stream into buf until it ends.
func (s *stream) read(r io.ReadCloser, gen int) {
	defer r.Close() //nolint:errcheck // read-only stream
	chunk := make([]byte, 32*1024)
	for {
		n, err := r.Read(chunk)
		s.mu.Lock()
		current := gen == s.gen
		if current && n > 0 {
			s.buf = append(s.buf, chunk[:n]...)
		}
		if current && err != nil {
			s.open = false
		}
		s.mu.Unlock()
		if current {
			select {
			case s.grew <- struct{}{}:
			default:
			}
		}
		if err != nil || !current {
			return
		}
	}
}

// snapshot is the log so far, up to its last complete line, the previous container's log,
// and the warnings about either.
func (s *stream) snapshot() (log, previous []byte, warnings []model.Warning) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := bytes.LastIndexByte(s.buf, '\n'); i >= 0 {
		log = slices.Clone(s.buf[:i+1])
	}
	if s.restarts > 0 && s.previousErr == nil {
		previous = slices.Clone(s.previous)
		if previous == nil {
			previous = []byte{}
		}
	}
	for _, w := range []*model.Warning{s.logWarn, s.previousErr} {
		if w != nil {
			warnings = append(warnings, *w)
		}
	}
	return log, previous, warnings
}

func (s *stream) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *stream) stopLocked() {
	if s.cancel != nil {
		s.cancel()
	}
	s.gen++
	s.open, s.cancel, s.buf = false, nil, nil
}
