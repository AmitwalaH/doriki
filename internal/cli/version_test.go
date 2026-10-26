package cli

import (
	"bytes"
	"testing"
)

// runVersion executes `doriki version` through the real root command and returns stdout.
func runVersion(t *testing.T) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	return out.String()
}

// A clean `go build` or `go run` has no -ldflags, so the defaults must show. The generic
// fallback pack is always embedded.
func TestVersionDefaults(t *testing.T) {
	want := "version: dev\ncommit: none\ndate: unknown\npacks: generic, llamacpp/11889, vllm/0.32\n"
	if got := runVersion(t); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Sentinel values prove the output reads the -ldflags variables and the pack list rather than
// printing fixed strings.
func TestVersionReadsBuildVarsAndPacks(t *testing.T) {
	saved := [...]string{version, commit, date}
	savedPacks := packNames
	t.Cleanup(func() {
		version, commit, date = saved[0], saved[1], saved[2]
		packNames = savedPacks
	})

	version, commit, date = "v9.9.9-sentinel", "c0ffee", "2000-01-02T03:04:05Z"
	packNames = func() []string { return []string{"vllm/0.28", "llamacpp/b5000"} }

	want := "version: v9.9.9-sentinel\ncommit: c0ffee\ndate: 2000-01-02T03:04:05Z\npacks: vllm/0.28, llamacpp/b5000\n"
	if got := runVersion(t); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
