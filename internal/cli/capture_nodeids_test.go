package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/AmitwalaH/doriki/internal/collect/k8s"
)

// A bundle's node.json keeps what the manifest reads (the kubelet version among the system
// info) and drops the node's machine ID, system UUID and boot ID, at both redaction levels:
// they identify the host, and nothing doriki reads needs them
// (DK-CLI-BUG-20261029-capture-node-host-ids).
func TestCaptureDropsNodeHostIDs(t *testing.T) {
	ids := []string{"planted-machine-id-7f3a", "planted-system-uuid-7f3a", "planted-boot-id-7f3a"}
	for _, strict := range []bool{false, true} {
		b, err := os.ReadFile(llamacppFixture + "pod.json")
		if err != nil {
			t.Fatal(err)
		}
		var pod corev1.Pod
		if err := json.Unmarshal(b, &pod); err != nil {
			t.Fatal(err)
		}
		node := &corev1.Node{}
		node.Name = pod.Spec.NodeName
		node.Status.NodeInfo = corev1.NodeSystemInfo{MachineID: ids[0], SystemUUID: ids[1], BootID: ids[2],
			KubeletVersion: "v1.37.0", ContainerRuntimeVersion: "containerd://2.3.4", OSImage: "Debian GNU/Linux 13 (trixie)"}

		c, err := redactObjects(k8s.Objects{Pod: &pod, Node: node}, strict)
		if err != nil {
			t.Fatal(err)
		}
		got := string(c.files["node.json"])
		for _, id := range ids {
			if strings.Contains(got, id) {
				t.Errorf("strict=%v: node.json has %q:\n%s", strict, id, got)
			}
		}
		for _, kept := range []string{"v1.37.0", "containerd://2.3.4", "Debian GNU/Linux 13 (trixie)"} {
			if !strings.Contains(got, kept) {
				t.Errorf("strict=%v: node.json lost %q:\n%s", strict, kept, got)
			}
		}
	}
}
