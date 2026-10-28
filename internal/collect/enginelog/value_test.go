package enginelog

import (
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// A fact with a fixed value takes that value whenever its regex matches, with no group v: how a
// pack says "hit" or "miss" for a line that says neither word. Facts sharing a key are separate
// entries, each from its own first matching line, in log order; manifest decides which wins.
func TestFixedValueFacts(t *testing.T) {
	p := model.Pack{Name: "x/1", Facts: []model.PackFact{
		{Key: "caches.torch_compile", Regex: `from the cache, took`, Value: "hit"},
		{Key: "caches.torch_compile", Regex: `Compiling a graph for compile range .* takes`, Value: "miss"},
		{Key: "model.loaded_gib", Regex: `Model loading took (?P<v>[0-9.]+) GiB memory`},
	}}
	out, _ := mustApply(t, p, []model.Event{
		line("2026-10-25T09:30:00Z", "Compiling a graph for compile range (1, 8192) takes 41.20 s"),
		line("2026-10-25T09:30:01Z", "Directly load the compiled graph(s) for compile range (1, 8192) from the cache, took 2.100 s"),
		line("2026-10-25T09:30:02Z", "Compiling a graph for compile range (8193, 16384) takes 12.00 s"),
		line("2026-10-25T09:30:03Z", "Model loading took 14.99 GiB memory and 5.234000 seconds"),
	})
	var got [][2]string
	for _, e := range out {
		if e.Kind == model.KindFact {
			got = append(got, [2]string{e.Attrs[model.AttrFactKey], e.Attrs[model.AttrFactValue]})
		}
	}
	want := [][2]string{{"caches.torch_compile", "miss"}, {"caches.torch_compile", "hit"}, {"model.loaded_gib", "14.99"}}
	if len(got) != len(want) {
		t.Fatalf("facts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("facts = %v, want %v", got, want)
			break
		}
	}
}
