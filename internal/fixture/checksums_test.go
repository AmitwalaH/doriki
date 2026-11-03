package fixture

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// entry is one checksum line for body, in the format the fixtures' SOURCE.md files use.
func entry(name, body string) string {
	sum := sha256.Sum256([]byte(body))
	return "- `" + name + "`: `sha256:" + hex.EncodeToString(sum[:]) + "`\n"
}

// write makes a fixture directory with the given files.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Verify passes a directory whose SOURCE.md lists every captured file with its sha256, and
// fails, naming the file, when one changed, is missing a line, or is listed but absent.
// SOURCE.md, expected.json and report.json are not captures and need no line.
func TestVerify(t *testing.T) {
	const log, pod = "a log line\n", "{}\n"
	section := "# x\n\n## Checksums\n\nText.\n\n" + entry("container.log", log) + entry("pod.json", pod)
	notCaptures := map[string]string{"expected.json": "{}", "report.json": "{}"}
	tests := []struct {
		name  string
		files map[string]string
		want  []string // substrings of the error; none means it passes
	}{
		{"every file listed", map[string]string{"SOURCE.md": section, "container.log": log, "pod.json": pod}, nil},
		{"changed", map[string]string{"SOURCE.md": section, "container.log": "edited\n", "pod.json": pod},
			[]string{"container.log has sha256 ", "the capture changed"}},
		{"no line", map[string]string{"SOURCE.md": section, "container.log": log, "pod.json": pod, "node.json": "{}"},
			[]string{"node.json has no checksum in SOURCE.md"}},
		{"listed but absent", map[string]string{"SOURCE.md": section, "container.log": log},
			[]string{"SOURCE.md lists pod.json, which is not in the fixture"}},
		{"no section", map[string]string{"SOURCE.md": "# x\n\n" + entry("container.log", log), "container.log": log},
			[]string{`no checksums under a "## Checksums" heading`, "container.log has no checksum"}},
		{"a line after a # heading in the section", map[string]string{"SOURCE.md": section + "\n# y\n\n" + entry("node.json", "{}"), "container.log": log, "pod.json": pod, "node.json": "{}"}, nil},
		{"a line under another heading", map[string]string{"SOURCE.md": section + "\n## Notes\n\n" + entry("node.json", "{}"), "container.log": log, "pod.json": pod, "node.json": "{}"},
			[]string{"node.json has no checksum in SOURCE.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{}
			for k, v := range notCaptures {
				files[k] = v
			}
			for k, v := range tt.files {
				files[k] = v
			}
			err := Verify(write(t, files))
			if len(tt.want) == 0 {
				if err != nil {
					t.Errorf("Verify: %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Verify passed, want an error with %q", tt.want)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not say %q", err, w)
				}
			}
		})
	}
}

// A SOURCE.md may hold several Checksums sections: a fixture made with doriki capture has the
// hand-written part's, for files the contributor added, and capture's own below it. Every one
// counts; a "# " heading between them does not end a section, and any other "## " heading
// does. Each test file's line is in one section only, so dropping either section fails.
func TestVerifyMergesSections(t *testing.T) {
	const manifest, log = "kind: Deployment\n", "a log line\n"
	source := "# Hand-written\n\n## Checksums\n\n" + entry("llamacpp.yaml", manifest) +
		"\n## Notes\n\nText.\n\n# doriki capture\n\n## Checksums\n\nText.\n\n" + entry("container.log", log)
	dir := write(t, map[string]string{"SOURCE.md": source, "llamacpp.yaml": manifest, "container.log": log})
	if err := Verify(dir); err != nil {
		t.Errorf("Verify: %v", err)
	}

	// The same lines with the second section's heading gone: the "# " heading does not start
	// collecting, so container.log has no line.
	broken := strings.Replace(source, "# doriki capture\n\n## Checksums\n", "# doriki capture\n", 1)
	dir = write(t, map[string]string{"SOURCE.md": broken, "llamacpp.yaml": manifest, "container.log": log})
	if err := Verify(dir); err == nil || !strings.Contains(err.Error(), "container.log has no checksum") {
		t.Errorf("Verify: %v, want container.log has no checksum", err)
	}
}
