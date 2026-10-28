package files

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/AmitwalaH/doriki/internal/model"
)

// A readable node gives the manifest its kubelet version, from status.nodeInfo, which
// pull-dominant needs to know whether image volumes are on by default (Kubernetes 1.35). No
// node, or a node without one, gives no fact rather than an empty one.
func TestKubeletFact(t *testing.T) {
	node := manifestNode()
	node.Status.NodeInfo = corev1.NodeSystemInfo{KubeletVersion: "v1.33.2"}
	events, _, err := FromObjects(Objects{Pod: manifestPod(), Node: node, Log: strings.NewReader("")})
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := facts(t, events)[model.FactNodeKubelet]; !ok || e.Attrs[model.AttrFactValue] != "v1.33.2" {
		t.Errorf("kubelet fact = %+v, want v1.33.2", e.Attrs)
	}
	for name, n := range map[string]*corev1.Node{"no node": nil, "no version": manifestNode()} {
		events, _, err := FromObjects(Objects{Pod: manifestPod(), Node: n, Log: strings.NewReader("")})
		if err != nil {
			t.Fatal(err)
		}
		if e, ok := facts(t, events)[model.FactNodeKubelet]; ok {
			t.Errorf("%s: kubelet fact %+v, want none", name, e.Attrs)
		}
	}
}
