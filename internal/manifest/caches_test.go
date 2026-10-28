package manifest

import (
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// The caches.torch_compile fact fills caches.torch_compile. A miss anywhere in the boot is a
// miss: a later hit (another compile range loaded from the cache) does not undo it.
func TestBuildTorchCompileCache(t *testing.T) {
	pack := model.Pack{Engine: "e", Name: "e/1"}
	for _, c := range []struct {
		values []string
		want   string
	}{
		{[]string{"hit"}, "hit"},
		{[]string{"miss"}, "miss"},
		{[]string{"hit", "miss"}, "miss"},
		{[]string{"miss", "hit"}, "miss"},
	} {
		var events []model.Event
		for _, v := range c.values {
			events = append(events, fact("caches.torch_compile", v))
		}
		got := Build(events, pack)
		if got.Caches == nil || got.Caches.TorchCompile != c.want {
			t.Errorf("%v: caches = %+v, want torch_compile %s", c.values, got.Caches, c.want)
		}
	}
}
