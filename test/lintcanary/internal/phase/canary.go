//go:build lintcanary

// Package phase sits where internal/phase does, so .golangci.yml treats it as a pure package.
// Every call the purity bans name is planted here once; a line that must be flagged says
// which linter flags it in a "want" comment, and test/lintcanary checks that exactly those
// lines are flagged. The other pure packages plant one call per ban, to prove the scope.
package phase

import (
	"fmt"
	"io/ioutil" //nolint:staticcheck // deprecated; planted for the io/ioutil ban
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/util/sets" // want depguard
)

// Planted calls every forbidden function once, and a few allowed ones.
func Planted(t time.Time) {
	_ = time.Now()                                // want forbidigo
	_ = time.Since(t)                             // want forbidigo
	_ = time.Until(t)                             // want forbidigo
	time.Sleep(0)                                 // want forbidigo
	_ = time.After(0)                             // want forbidigo
	_ = time.AfterFunc(0, func() {})              // want forbidigo
	_ = time.Tick(time.Second)                    //nolint:staticcheck // want forbidigo
	_ = time.NewTicker(time.Second)               // want forbidigo
	_ = time.NewTimer(0)                          // want forbidigo
	_, _ = os.ReadFile("x")                       // want forbidigo
	_ = exec.Command("x")                         // want forbidigo
	signal.Ignore(syscall.SIGHUP)                 // want forbidigo
	_ = syscall.Getpid()                          // want forbidigo
	_, _ = ioutil.ReadAll(strings.NewReader("x")) // want forbidigo
	_, _ = net.Dial("tcp", "x")                   // want forbidigo
	_, _ = http.Get("http://x")                   // want forbidigo
	_, _ = filepath.Abs("x")                      // want forbidigo
	_, _ = filepath.EvalSymlinks("x")             // want forbidigo
	_, _ = filepath.Glob("x")                     // want forbidigo
	_ = filepath.Walk("x", nil)                   // want forbidigo
	_ = filepath.WalkDir("x", nil)                // want forbidigo
	fmt.Print("x")                                // want forbidigo
	fmt.Printf("x")                               // want forbidigo
	fmt.Println("x")                              // want forbidigo
	print("x")                                    // want forbidigo
	println("x")                                  // want forbidigo
	_ = sets.New[string]()

	// Allowed: time types and arithmetic, pure path and string functions, formatting to a
	// string or to a writer the caller gives.
	_ = t.Add(time.Second)
	_ = filepath.Join("a", "b")
	_ = fmt.Sprintf("%d", 1)
	_, _ = fmt.Fprintln(&strings.Builder{}, "x")
}
