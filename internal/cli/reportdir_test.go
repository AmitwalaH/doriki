package cli

import (
	"bytes"
	"testing"

	renderjson "github.com/AmitwalaH/doriki/internal/render/json"
)

const llamacppLv4Fixture = "../../packs/llamacpp/testdata/11889-lv4/"

// ReportFromDir is the report --from-files <dir> prints, node included: the -lv 4 capture has
// a node.json, which the three-file form does not read.
func TestReportFromDir(t *testing.T) {
	out, err := run(t, "--from-files", "-o", "json", llamacppLv4Fixture)
	if code, msg := Exit(err); code != 0 {
		t.Fatalf("exit %d %s", code, msg)
	}
	r, err := ReportFromDir(llamacppLv4Fixture, false, now())
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := renderjson.Write(&b, r); err != nil {
		t.Fatal(err)
	}
	if b.String() != out {
		t.Errorf("ReportFromDir differs from --from-files <dir>:\n%s\nwant\n%s", b.String(), out)
	}
	if r.Manifest == nil || r.Manifest.Node == nil || r.Manifest.Node.Kubelet != "v1.37.0" {
		t.Errorf("manifest node %+v, want the kubelet v1.37.0 node.json records", r.Manifest)
	}
}
