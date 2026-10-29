//go:build lintcanary

// Package cli sits where internal/cli does, which imports everything and does the I/O: the
// same calls as the pure packages, none of which may be flagged.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
)

// Planted calls what the pure packages may not.
func Planted() {
	_ = time.Now()
	_, _ = os.ReadFile("x")
	_, _ = filepath.Glob("x")
	fmt.Println("x")
	_ = sets.New[string]()
}
