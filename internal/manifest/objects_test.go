package manifest

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// The collector's facts fill the pod, node, container and accelerator fields (DESIGN.md
// section 4.3); args keep their order and env becomes the allowlist map.
func TestBuildObjectFacts(t *testing.T) {
	events := []model.Event{
		image(map[string]string{model.AttrImage: "vllm/vllm-openai:v0.32.0", model.AttrNamespace: "default", model.AttrPod: "vllm-1", model.AttrNode: "gpu-1"}),
		fact(model.FactPodUID, "u-1"),
		fact(model.FactPodOwner, "Deployment/vllm"),
		fact(model.FactPodOwnerRef, "ReplicaSet/vllm-785fb8997d"),
		fact(model.FactImageDigest, "sha256:3f9a"),
		fact(model.FactArgs, `["--model","m","--tp","2"]`),
		fact(model.FactEnvPrefix+"VLLM_MODEL", "m"),
		fact(model.FactEnvPrefix+"HF_TOKEN", Redacted),
		fact(model.FactNodeInstanceType, "p4d.24xlarge"),
		fact(model.FactNodeZone, "us-east-1a"),
		fact(model.FactAcceleratorVendor, "nvidia"),
		fact(model.FactAcceleratorProduct, "A100-SXM4-80GB"),
		fact(model.FactAcceleratorCount, "2"),
	}
	got := Build(events, model.Pack{Engine: "vllm", Name: "vllm/0.32"})
	want := &model.Manifest{
		ManifestVersion: model.ManifestVersion,
		Pod:             &model.ManifestPod{Namespace: "default", Name: "vllm-1", UID: "u-1", Owner: "Deployment/vllm", OwnerRef: "ReplicaSet/vllm-785fb8997d"},
		Node:            &model.ManifestNode{Name: "gpu-1", InstanceType: "p4d.24xlarge", Zone: "us-east-1a"},
		Container: &model.ManifestContainer{Image: "vllm/vllm-openai:v0.32.0", ImageDigest: "sha256:3f9a", Args: []string{"--model", "m", "--tp", "2"},
			EnvAllowlist: map[string]string{"VLLM_MODEL": "m", "HF_TOKEN": Redacted}},
		Engine:      &model.ManifestEngine{Name: "vllm", ParserPack: "vllm/0.32"},
		Accelerator: &model.ManifestAccelerator{Vendor: "nvidia", Product: "A100-SXM4-80GB", Count: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Build =\n %+v\nwant\n %+v", got, want)
	}
}

// The fingerprint ignores the pod's name and UID, so two boots of one deployment share it,
// and changes with any other field. It is "sha256:" and 64 hex digits, and the same for the
// same manifest however its maps were built.
func TestFingerprint(t *testing.T) {
	base := func() *model.Manifest {
		return &model.Manifest{
			ManifestVersion: model.ManifestVersion,
			Pod:             &model.ManifestPod{Namespace: "default", Name: "vllm-1", UID: "u-1", Owner: "Deployment/vllm"},
			Container:       &model.ManifestContainer{Image: "img", ImageDigest: "sha256:1", EnvAllowlist: map[string]string{"A_X": "1", "B_Y": "2"}},
		}
	}
	a := Fingerprint(base())
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(a) {
		t.Fatalf("fingerprint %q", a)
	}

	other := base()
	other.Pod.Name, other.Pod.UID, other.Fingerprint = "vllm-2", "u-2", "sha256:old"
	if Fingerprint(other) != a {
		t.Errorf("name, UID or an old fingerprint changed the fingerprint")
	}
	if other.Pod.Name != "vllm-2" || other.Fingerprint != "sha256:old" {
		t.Errorf("Fingerprint modified its argument")
	}

	changed := base()
	changed.Container.ImageDigest = "sha256:2"
	if Fingerprint(changed) == a {
		t.Errorf("a different image digest gave the same fingerprint")
	}
	reordered := base()
	reordered.Container.EnvAllowlist = map[string]string{"B_Y": "2", "A_X": "1"}
	if Fingerprint(reordered) != a {
		t.Errorf("map order changed the fingerprint")
	}
}
