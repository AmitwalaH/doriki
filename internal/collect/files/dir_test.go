package files

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/AmitwalaH/doriki/internal/model"
)

// copyFixture copies a real fixture's files into a new directory, as a capture bundle lays
// them out.
func copyFixture(t *testing.T, fixture string, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(fixture, n)) // #nosec G304 -- a fixture path
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o600); err != nil { // #nosec G703 -- a file in t.TempDir()
			t.Fatal(err)
		}
	}
	return dir
}

// A capture directory reads like the files named one by one, plus its optional node.json and
// container-previous.log.
func TestLoadDir(t *testing.T) {
	restart := "../../../packs/generic/testdata/kind-restart"
	dir := copyFixture(t, restart, "pod.json", "events.json", "container.log", "container-previous.log")
	got, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, _, err := Load(filepath.Join(restart, "pod.json"), filepath.Join(restart, "events.json"),
		filepath.Join(restart, "container.log"), filepath.Join(restart, "container-previous.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("LoadDir gave %d events, Load %d", len(got), len(want))
	}
	for i := range want {
		if model.CompareEvents(got[i], want[i]) != 0 {
			t.Errorf("event %d: %+v, want %+v", i, got[i], want[i])
		}
	}

	node := corev1.Node{}
	node.Name = "n1"
	node.Status.NodeInfo.KubeletVersion = "v1.35.1"
	b, _ := json.Marshal(node)
	if err := os.WriteFile(filepath.Join(dir, "node.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	got, _, err = LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := facts(t, got)[model.FactNodeKubelet]; !ok || e.Attrs[model.AttrFactValue] != "v1.35.1" {
		t.Errorf("node.json not read: kubelet fact %+v", e.Attrs)
	}
}

// A directory without the required files names what is missing.
func TestLoadDirMissing(t *testing.T) {
	_, _, err := LoadDir(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "pod.json") {
		t.Errorf("err = %v, want one naming pod.json", err)
	}
}
