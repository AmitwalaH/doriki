package files

import (
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AmitwalaH/doriki/internal/model"
)

// facts returns the collector's fact events by key.
func facts(t *testing.T, events []model.Event) map[string]model.Event {
	t.Helper()
	out := map[string]model.Event{}
	for _, e := range events {
		if e.Source == model.SourceManifest && e.Kind == model.KindFact {
			k := e.Attrs[model.AttrFactKey]
			if _, dup := out[k]; dup {
				t.Errorf("fact %s emitted twice", k)
			}
			out[k] = e
		}
	}
	return out
}

func manifestPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vllm-785fb8997d-bvsz4", Namespace: "default", UID: "u-1",
			CreationTimestamp: metav1.NewTime(ts("2026-10-25T09:23:40Z")),
			Labels:            map[string]string{"pod-template-hash": "785fb8997d"},
			OwnerReferences:   []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "vllm-785fb8997d", Controller: new(true)}},
		},
		Spec: corev1.PodSpec{
			NodeName: "gpu-1",
			Containers: []corev1.Container{{
				Name: "main", Image: "vllm/vllm-openai:v0.32.0",
				Args: []string{"--model", "facebook/opt-125m"},
				Env: []corev1.EnvVar{
					{Name: "VLLM_MODEL", Value: "facebook/opt-125m"},
					{Name: "HF_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{Key: "token"}}},
					{Name: "PATH", Value: "/usr/bin"},
				},
				Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("2")}},
			}},
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name: "main", ImageID: "docker.io/vllm/vllm-openai@sha256:3f9a",
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(ts("2026-10-25T09:25:00Z"))}},
		}}},
	}
}

func manifestNode() *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu-1", Labels: map[string]string{
		"node.kubernetes.io/instance-type": "p4d.24xlarge",
		"topology.kubernetes.io/zone":      "us-east-1a",
		"nvidia.com/gpu.product":           "A100-SXM4-80GB",
	}}}
}

// The pod, its node and its owner become one fact event each, under the model's keys, stamped
// with pod creation (DESIGN.md section 4.3). Env facts carry literal values only, one per
// variable, unfiltered: manifest.Sanitize decides what is kept. No env or arg value is in Raw.
func TestManifestFacts(t *testing.T) {
	events, _, err := FromObjects(Objects{Pod: manifestPod(), Node: manifestNode(), Log: strings.NewReader("")})
	if err != nil {
		t.Fatal(err)
	}
	got := facts(t, events)
	want := map[string]string{
		model.FactPodUID:                   "u-1",
		model.FactPodOwner:                 "Deployment/vllm",
		model.FactPodOwnerRef:              "ReplicaSet/vllm-785fb8997d",
		model.FactImageDigest:              "sha256:3f9a",
		model.FactArgs:                     `["--model","facebook/opt-125m"]`,
		model.FactEnvPrefix + "VLLM_MODEL": "facebook/opt-125m",
		model.FactEnvPrefix + "PATH":       "/usr/bin",
		model.FactNodeInstanceType:         "p4d.24xlarge",
		model.FactNodeZone:                 "us-east-1a",
		model.FactAcceleratorVendor:        "nvidia",
		model.FactAcceleratorProduct:       "A100-SXM4-80GB",
		model.FactAcceleratorCount:         "2",
	}
	for k, v := range want {
		e, ok := got[k]
		if !ok || e.Attrs[model.AttrFactValue] != v {
			t.Errorf("fact %s = %q, want %q", k, e.Attrs[model.AttrFactValue], v)
			continue
		}
		if !e.TS.Equal(ts("2026-10-25T09:23:40Z")) {
			t.Errorf("fact %s at %v, want pod creation", k, e.TS)
		}
	}
	if len(got) != len(want) {
		t.Errorf("facts = %d keys, want %d (a valueFrom env has no literal value): %v", len(got), len(want), got)
	}
	for k, e := range got {
		if strings.HasPrefix(k, model.FactEnvPrefix) || k == model.FactArgs {
			if strings.Contains(e.Raw, e.Attrs[model.AttrFactValue]) {
				t.Errorf("fact %s Raw %q holds the value", k, e.Raw)
			}
		}
	}
}

// Without a node, node and accelerator-product facts are absent; the count from the limits
// stays, and so does the vendor it implies. Without a GPU limit or label, no accelerator.
func TestManifestFactsWithoutNode(t *testing.T) {
	events, _, _ := FromObjects(Objects{Pod: manifestPod(), Log: strings.NewReader("")})
	got := facts(t, events)
	for _, k := range []string{model.FactNodeInstanceType, model.FactNodeZone, model.FactAcceleratorProduct} {
		if _, ok := got[k]; ok {
			t.Errorf("fact %s present without a node", k)
		}
	}
	if got[model.FactAcceleratorCount].Attrs[model.AttrFactValue] != "2" || got[model.FactAcceleratorVendor].Attrs[model.AttrFactValue] != "nvidia" {
		t.Errorf("accelerator facts = %v, want count 2 and vendor nvidia from the limits", got)
	}

	p := manifestPod()
	p.Spec.Containers[0].Resources = corev1.ResourceRequirements{}
	events, _, _ = FromObjects(Objects{Pod: p, Log: strings.NewReader("")})
	for k := range facts(t, events) {
		if strings.HasPrefix(k, "accelerator.") {
			t.Errorf("fact %s without a GPU", k)
		}
	}
}

// The owner: a ReplicaSet named <deployment>-<pod-template-hash> belongs to that Deployment.
// When the label is missing or the name does not end in it, the ReplicaSet is kept. Any other
// owner kind is recorded as written. owner_ref is always the pod's own entry.
func TestOwnerInferred(t *testing.T) {
	cases := map[string]struct {
		kind, name string
		hash       string
		want       string
	}{
		"deployment":      {"ReplicaSet", "vllm-785fb8997d", "785fb8997d", "Deployment/vllm"},
		"hash mismatch":   {"ReplicaSet", "vllm-785fb8997d", "aaaa", "ReplicaSet/vllm-785fb8997d"},
		"no hash label":   {"ReplicaSet", "vllm-785fb8997d", "", "ReplicaSet/vllm-785fb8997d"},
		"statefulset":     {"StatefulSet", "vllm", "", "StatefulSet/vllm"},
		"inferenceserver": {"InferenceService", "llm", "", "InferenceService/llm"},
	}
	for name, c := range cases {
		p := manifestPod()
		p.OwnerReferences[0].Kind, p.OwnerReferences[0].Name = c.kind, c.name
		p.Labels = map[string]string{}
		if c.hash != "" {
			p.Labels["pod-template-hash"] = c.hash
		}
		events, _, _ := FromObjects(Objects{Pod: p, Log: strings.NewReader("")})
		got := facts(t, events)
		if v := got[model.FactPodOwner].Attrs[model.AttrFactValue]; v != c.want {
			t.Errorf("%s: owner = %q, want %q", name, v, c.want)
		}
		if v := got[model.FactPodOwnerRef].Attrs[model.AttrFactValue]; v != c.kind+"/"+c.name {
			t.Errorf("%s: owner_ref = %q", name, v)
		}
	}

	p := manifestPod()
	p.OwnerReferences = nil
	events, _, _ := FromObjects(Objects{Pod: p, Log: strings.NewReader("")})
	got := facts(t, events)
	if _, ok := got[model.FactPodOwner]; ok {
		t.Errorf("owner fact for a bare pod")
	}
}

// Args are written as a JSON array so a value with spaces or quotes survives intact.
func TestArgsAsJSON(t *testing.T) {
	p := manifestPod()
	p.Spec.Containers[0].Args = []string{"--chat-template", `{"a": "b c"}`}
	events, _, _ := FromObjects(Objects{Pod: p, Log: strings.NewReader("")})
	var args []string
	if err := json.Unmarshal([]byte(facts(t, events)[model.FactArgs].Attrs[model.AttrFactValue]), &args); err != nil || len(args) != 2 || args[1] != `{"a": "b c"}` {
		t.Errorf("args = %v (%v)", args, err)
	}
}
