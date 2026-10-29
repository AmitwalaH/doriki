//go:build lintcanary

package phase

import (
	"os"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
)

// Tests of a pure package may read the clock and files and import anything: nothing here may
// be flagged.
func TestPlanted(t *testing.T) {
	_ = time.Now()
	_, _ = os.ReadFile("x")
	_ = sets.New[string]()
}
