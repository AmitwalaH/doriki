package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"

	"github.com/AmitwalaH/doriki/internal/collect/files"
	"github.com/AmitwalaH/doriki/internal/collect/k8s"
	"github.com/AmitwalaH/doriki/internal/model"
)

// liveClient connects to the cluster the standard kubectl flags name (--kubeconfig, --context,
// -n, --as, ...) and returns the namespace they select. A variable so tests use a fake API
// and never a real cluster.
var liveClient = func(f *genericclioptions.ConfigFlags) (k8s.Client, string, error) {
	cfg, err := f.ToRESTConfig()
	if err != nil {
		return k8s.Client{}, "", err
	}
	ns, _, err := f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return k8s.Client{}, "", err
	}
	api, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return k8s.Client{}, "", err
	}
	return k8s.Client{API: api}, ns, nil
}

// collect is a variable so a test can see what the CLI asks the collector for.
var collect = k8s.Collect

// targetKinds are the resource names doriki accepts, as kubectl spells them.
var targetKinds = map[string]k8s.Kind{
	"pod": k8s.KindPod, "pods": k8s.KindPod, "po": k8s.KindPod,
	"deploy": k8s.KindDeployment, "deployment": k8s.KindDeployment, "deployments": k8s.KindDeployment,
	"sts": k8s.KindStatefulSet, "statefulset": k8s.KindStatefulSet, "statefulsets": k8s.KindStatefulSet,
}

const targetUsage = "pod/<name>, deploy/<name> or sts/<name>"

// parseTarget reads pod/<name>, deploy/<name> or sts/<name>; a bare name is a pod (DESIGN.md
// section 7.1).
func parseTarget(s string) (k8s.Target, error) {
	kind, name, ok := strings.Cut(s, "/")
	if !ok {
		kind, name = "pod", s
	}
	k, known := targetKinds[strings.ToLower(kind)]
	if !known {
		return k8s.Target{}, fmt.Errorf("doriki profiles %s; %q is not one of those", targetUsage, kind)
	}
	if name == "" || strings.Contains(name, "/") {
		return k8s.Target{}, fmt.Errorf("want %s, got %q", targetUsage, s)
	}
	return k8s.Target{Kind: k, Name: name}, nil
}

// collectLive reads the target from the cluster and turns it into events through the same
// conversion as --from-files. Anything the API refuses or cannot answer is exit 2, with its
// message.
func collectLive(ctx context.Context, f *profileFlags, arg string, now time.Time) ([]model.Event, []model.Warning, error) {
	t, err := parseTarget(arg)
	if err != nil {
		return nil, nil, err
	}
	client, ns, err := liveClient(f.kube)
	if err != nil {
		return nil, nil, &ExitError{Code: ExitUnavailable, Err: err}
	}
	t.Namespace = ns
	var notBefore time.Time
	if f.since > 0 {
		notBefore = now.Add(-f.since)
	}
	o, err := collect(ctx, client, t, notBefore)
	if err != nil {
		return nil, nil, &ExitError{Code: ExitUnavailable, Err: err}
	}
	objs := files.Objects{Pod: o.Pod, Events: o.Events, Node: o.Node, Owner: o.Owner, Log: bytes.NewReader(o.Log)}
	if o.PreviousLog != nil {
		objs.PreviousLog = bytes.NewReader(o.PreviousLog)
	}
	events, warnings, err := files.FromObjects(objs)
	if err != nil {
		return nil, nil, err
	}
	return events, append(o.Warnings, warnings...), nil
}

// writeAttempts prints each boot attempt's waterfall under an "attempt N of M" line; only the
// reported attempt carries the warnings.
func writeAttempts(w io.Writer, r model.Report, write func(io.Writer, model.Report) error) error {
	for i, a := range r.Attempts {
		one := r
		one.Attempt, one.Phases, one.Gaps, one.Totals, one.Attempts = a.Attempt, a.Phases, a.Gaps, a.Totals, nil
		if a.Attempt != r.Attempt {
			one.Warnings = nil
		}
		if i > 0 {
			fmt.Fprintln(w) //nolint:errcheck // a failed write shows in the next one
		}
		if err := writeAttempt(w, one, write); err != nil {
			return err
		}
	}
	return nil
}

func writeAttempt(w io.Writer, r model.Report, write func(io.Writer, model.Report) error) error {
	if _, err := fmt.Fprintf(w, "attempt %d of %d\n", r.Attempt, r.AttemptsTotal); err != nil {
		return err
	}
	return write(w, r)
}
