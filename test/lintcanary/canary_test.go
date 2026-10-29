//go:build lintcanary

// Package lintcanary proves the purity bans in .golangci.yml still catch what they must
// (DK-REL-ENH-20261024-forbidigo-canary-ci). internal/ holds Go source, built only with the
// lintcanary tag, that plants forbidden calls where the pure packages live; this test runs the
// real golangci-lint with the real config on it. Run it with `make lint-canary`;
// LINT_CANARY_CONFIG names another config to check, such as a narrowed copy.
package lintcanary

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// root is the repository root, from this package's directory.
const root = "../.."

// want marks a planted line golangci-lint must flag, naming the linter.
var want = regexp.MustCompile(`// want (\w+)$`)

// Exactly the planted lines marked "want" are flagged, each by the linter it names. A missing
// line is a ban that stopped working; an extra line is a ban that reached a line it must not
// (a test file, the cli package, an allowed call).
func TestPurityBans(t *testing.T) {
	checkVersion(t)
	config := filepath.Join(root, ".golangci.yml")
	if c := os.Getenv("LINT_CANARY_CONFIG"); c != "" {
		config = c
	}
	// golangci-lint runs from the root, so a relative path would point elsewhere.
	config, err := filepath.Abs(config)
	if err != nil {
		t.Fatal(err)
	}

	// The limits are off: by default golangci-lint shows 3 issues with the same text, which
	// would hide every planted time.Now() but three. Paths are absolute: by default they are
	// relative to the config's directory, so a config elsewhere would rename every file.
	cmd := exec.Command("golangci-lint", "run", "--config", config, "--build-tags", "lintcanary", // #nosec G204 G702 -- a fixed command; the config path is the caller's
		"--max-issues-per-linter", "0", "--max-same-issues", "0", "--show-stats=false", "--path-mode", "abs",
		"--output.text.path", "", "--output.json.path", "stdout", "./test/lintcanary/...")
	cmd.Dir = root
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) { // 1 means issues found
		t.Fatalf("golangci-lint: %v\n%s", err, stderr(err))
	}
	var result struct {
		Issues []struct {
			FromLinter string
			Pos        struct {
				Filename string
				Line     int
			}
		}
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("golangci-lint output: %v\n%s", err, out)
	}
	var got []string
	for _, i := range result.Issues {
		file, err := filepath.Rel(abs, i.Pos.Filename)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, key(file, i.Pos.Line, i.FromLinter))
	}

	expected := planted(t)
	for _, k := range expected {
		if !slices.Contains(got, k) {
			t.Errorf("not flagged: %s", k)
		}
	}
	for _, k := range got {
		if !slices.Contains(expected, k) {
			t.Errorf("flagged, not planted: %s", k)
		}
	}
	t.Logf("%d issues flagged, %d planted, with %s", len(got), len(expected), config)
}

// planted is every "want" line under internal/, as "file:line linter".
func planted(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir("internal", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" {
			return err
		}
		f, err := os.Open(path) // #nosec G304 G122 -- a file of this package, read once
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck // read-only file
		sc := bufio.NewScanner(f)
		for n := 1; sc.Scan(); n++ {
			if m := want.FindStringSubmatch(sc.Text()); m != nil {
				out = append(out, key(filepath.Join("test/lintcanary", path), n, m[1]))
			}
		}
		return sc.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("no planted lines found")
	}
	return out
}

func key(file string, line int, linter string) string {
	return filepath.ToSlash(file) + ":" + strconv.Itoa(line) + " " + linter
}

// checkVersion fails when the golangci-lint on PATH is not the version ci.yml pins, which is
// the one place the version is set: another version may catch a different set, and the diff
// would read as a broken ban.
func checkVersion(t *testing.T) {
	t.Helper()
	ci, err := os.ReadFile(filepath.Join(root, ".github/workflows/ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	pin := regexp.MustCompile(`golangci-lint-action@\S+.*\n\s+with:\n\s+version: v(\S+)`).FindSubmatch(ci)
	if pin == nil {
		t.Fatal("ci.yml pins no golangci-lint version")
	}
	out, err := exec.Command("golangci-lint", "version", "--short").Output()
	if err != nil {
		t.Fatalf("golangci-lint version: %v (is golangci-lint on PATH?)", err)
	}
	if got := strings.TrimPrefix(strings.TrimSpace(string(out)), "v"); got != string(pin[1]) {
		t.Fatalf("golangci-lint %s on PATH, ci.yml pins %s", got, pin[1])
	}
}

func stderr(err error) string {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(exit.Stderr)
	}
	return ""
}
