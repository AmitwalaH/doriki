//go:build lintcanary

// Package render sits where internal/render does: one planted call per purity ban proves the bans
// reach it (see ../phase/canary.go).
package render

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charm.land/lipgloss/v2"
	"k8s.io/apimachinery/pkg/util/sets" // want depguard
)

// Planted calls one function of each ban.
func Planted() {
	_ = time.Now()            // want forbidigo
	_, _ = os.ReadFile("x")   // want forbidigo
	_, _ = filepath.Glob("x") // want forbidigo
	fmt.Println("x")          // want forbidigo
	_ = sets.New[string]()
	_ = lipgloss.NewStyle() // allowed: render's declared library
}
