// Package text writes the report as the terminal waterfall of DESIGN.md section 2: one row per
// observed phase, bars placed by when the phase ran and sized by how long it took, 100 columns
// wide, with no colour unless asked for (section 7.5). It is pure: it reads only the report and
// writes only to the writer it is given; the caller decides whether colour is wanted.
package text

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Layout, in terminal columns. A row is the name, the bar track, a space, the duration right
// aligned, two spaces and the attributes: 17 + 44 + 1 + 7 + 2 + 29 = Width. The name column
// is one wider than the longest label, "earlier attempts", so no label touches its bar.
const (
	Width      = 100
	nameWidth  = 17
	trackWidth = 44
	durWidth   = 7
	attrWidth  = Width - nameWidth - trackWidth - 1 - durWidth - 2
	maxAdvice  = 3 // DESIGN.md section 2: at most three advice lines
)

// Options are the caller's choices; the zero value is plain text.
type Options struct {
	// Color styles the output with terminal escape codes. The caller sets it only for a
	// terminal with NO_COLOR unset.
	Color bool
}

type styles struct {
	header, bar, gap, tilde, dim, warn lipgloss.Style
}

func newStyles(color bool) styles {
	if !color {
		return styles{}
	}
	return styles{
		header: lipgloss.NewStyle().Bold(true),
		bar:    lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		gap:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		tilde:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		dim:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		warn:   lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
	}
}

// Write renders r. Rows are the phases that started, in the fixed order, each followed by the
// gap after it; skipped and absent phases are one line each; then the legend for "~",
// warnings and advice, each only when there is something to say.
func Write(w io.Writer, r model.Report, opts Options) error {
	st := newStyles(opts.Color)
	tr := newTrack(r)
	var lines []string
	add := func(s string) { lines = append(lines, s) }

	add(st.header.Render(header(r)))
	add("")
	var skipped, notApplicable, absent []string
	estimated := false
	for _, p := range r.Phases {
		switch p.State {
		case model.StateSkipped:
			if p.Attrs[model.AttrNotApplicable] == "true" {
				notApplicable = append(notApplicable, string(p.Name))
				continue
			}
			skipped = append(skipped, string(p.Name))
			continue
		case model.StateAbsent:
			absent = append(absent, string(p.Name))
			continue
		}
		add(phaseRow(p, tr, st))
		if p.Confidence != nil && *p.Confidence < 1 {
			estimated = true
		}
		// Gaps after one phase are consecutive pieces of one interval (earlier attempts, then
		// unattributed time), so each is drawn from where the previous one ended.
		from := p.End
		for _, g := range r.Gaps {
			if g.After == p.Name && !p.End.IsZero() {
				add(gapRow(g, from, tr, st))
				from = from.Add(time.Duration(g.Seconds * float64(time.Second)))
			}
		}
	}

	if len(skipped) > 0 || len(notApplicable) > 0 || len(absent) > 0 {
		add("")
	}
	if len(skipped) > 0 {
		lines = append(lines, wrap(st.dim, "skipped: "+strings.Join(skipped, ", ")+" (the engine said it skips them)", "  ")...)
	}
	if len(notApplicable) > 0 {
		lines = append(lines, wrap(st.dim, "skipped: "+strings.Join(notApplicable, ", ")+" (not applicable to this engine)", "  ")...)
	}
	if len(absent) > 0 {
		why := "not seen in the events or the log"
		if pack := parserPack(r); pack != "" {
			why += "; pack " + pack
		}
		lines = append(lines, wrap(st.dim, "absent: "+strings.Join(absent, ", ")+" ("+why+")", "  ")...)
	}
	if estimated {
		add(st.tilde.Render("~") + " estimated: confidence below 1.0 (a start derived from a logged duration, or a heuristic marker)")
	}
	if len(r.Warnings) > 0 {
		add("")
		add(st.header.Render("Warnings"))
		for _, wn := range r.Warnings {
			lines = append(lines, wrap(st.warn, "  "+wn.Code+": "+wn.Text, "    ")...)
		}
	}
	if len(r.Advice) > 0 {
		add("")
		add(st.header.Render("Advice"))
		for i, a := range r.Advice[:min(len(r.Advice), maxAdvice)] {
			lines = append(lines, wrap(lipgloss.Style{}, fmt.Sprintf("  %d. %s: %s", i+1, a.Rule, a.Text), "     ")...)
		}
	}
	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}

// header is "Pod ns/name   engine vllm 0.32.0   node n   total m:ss", leaving out what the
// report does not know. Under a pack with no engine (generic) it names the pack instead. A pod
// name too long for the line is shortened, so the engine, node and total always show.
func header(r model.Report) string {
	const sep = "   "
	var parts []string
	m := r.Manifest
	switch {
	case m != nil && m.Engine != nil && m.Engine.Name != "":
		parts = append(parts, strings.TrimSpace("engine "+m.Engine.Name+" "+m.Engine.Version))
	case parserPack(r) != "":
		parts = append(parts, "pack "+parserPack(r))
	}
	if m != nil && m.Node != nil && m.Node.Name != "" {
		parts = append(parts, "node "+m.Node.Name)
	}
	if t := r.Totals.PendingToReadySeconds; t != nil {
		parts = append(parts, "total "+duration(*t))
	}
	rest := strings.Join(parts, sep)
	if m == nil || m.Pod == nil {
		return fit(rest, Width)
	}
	pod := "Pod " + strings.TrimPrefix(m.Pod.Namespace+"/"+m.Pod.Name, "/")
	if rest == "" {
		return fit(pod, Width)
	}
	return fit(pod, max(Width-lipgloss.Width(sep+rest), len("Pod …"))) + sep + rest
}

func parserPack(r model.Report) string {
	if r.Manifest == nil || r.Manifest.Engine == nil {
		return ""
	}
	return r.Manifest.Engine.ParserPack
}

// track maps times onto the bar track. It runs from pod creation (scheduling's start, or the
// earliest phase start when scheduling was not seen) to the later of Ready and the last phase
// end: the wall-clock span, so bars are proportional to it (DESIGN.md section 4.2).
type track struct {
	origin time.Time
	span   time.Duration
}

func newTrack(r model.Report) track {
	var origin, end time.Time
	for _, p := range r.Phases {
		if p.Start.IsZero() {
			continue
		}
		if p.Name == model.PhaseScheduling {
			origin = p.Start
		} else if !hasScheduling(r) && (origin.IsZero() || p.Start.Before(origin)) {
			origin = p.Start
		}
		for _, t := range []time.Time{p.Start, p.End} {
			if t.After(end) {
				end = t
			}
		}
	}
	if t := r.Totals.PendingToReadySeconds; t != nil && !origin.IsZero() {
		if ready := origin.Add(time.Duration(*t * float64(time.Second))); ready.After(end) {
			end = ready
		}
	}
	span := end.Sub(origin)
	if span <= 0 {
		span = time.Second
	}
	return track{origin, span}
}

func hasScheduling(r model.Report) bool {
	return slices.ContainsFunc(r.Phases, func(p model.Phase) bool {
		return p.Name == model.PhaseScheduling && !p.Start.IsZero()
	})
}

// edge is a time's position on the track, rounded to a cell boundary and kept on the track.
func (t track) edge(at time.Time) int {
	x := math.Round(float64(at.Sub(t.origin)) / float64(t.span) * trackWidth)
	return int(max(0, min(trackWidth, x)))
}

// cells draws [from, to) with fill, at least one cell; a zero-length span is a "│" marker.
func (t track) cells(from, to time.Time, fill rune) string {
	b := []rune(strings.Repeat(" ", trackWidth))
	first := min(t.edge(from), trackWidth-1)
	if !to.After(from) {
		b[first] = '│'
		return string(b)
	}
	last := max(first, t.edge(to)-1)
	for i := first; i <= last; i++ {
		b[i] = fill
	}
	return string(b)
}

func phaseRow(p model.Phase, tr track, st styles) string {
	var bar, dur string
	switch p.State {
	case model.StateComplete:
		// The report's seconds, not end minus start: the phase engine clamps an end within
		// 1 s before the start to 0 (timestamp resolution), and text must agree with JSON.
		seconds := p.End.Sub(p.Start).Seconds()
		if p.Seconds != nil {
			seconds = *p.Seconds
		}
		bar, dur = tr.cells(p.Start, p.End, '█'), duration(seconds)
	case model.StateOpen:
		bar, dur = tr.cells(p.Start, tr.origin.Add(tr.span), '█'), "open"
	default: // invalid: its times contradict each other, so it gets no bar
		bar, dur = strings.Repeat(" ", trackWidth), string(p.State)
	}
	durText := fmt.Sprintf("%*s", durWidth, dur)
	if p.Confidence != nil && *p.Confidence < 1 {
		durText = fmt.Sprintf("%*s", durWidth, "~"+dur)
		durText = strings.Replace(durText, "~", st.tilde.Render("~"), 1)
	}
	line := fmt.Sprintf("%-*s%s %s", nameWidth, p.Name, st.bar.Render(bar), durText)
	if a := attrs(p); a != "" {
		line += "  " + fit(a, attrWidth)
	}
	return strings.TrimRight(line, " ")
}

func gapRow(g model.Gap, from time.Time, tr track, st styles) string {
	to := from.Add(time.Duration(g.Seconds * float64(time.Second)))
	label := "gap"
	if g.Reason != "" {
		label = g.Reason
	}
	line := fmt.Sprintf("%-*s%s %*s", nameWidth, label, st.gap.Render(tr.cells(from, to, '░')), durWidth, duration(g.Seconds))
	return strings.TrimRight(line, " ")
}

// attrs is the short note after a phase's duration: only attributes a reader acts on, in a
// fixed order, so the report's raw attributes (seconds, start_derived, skipped_by) stay in JSON.
func attrs(p model.Phase) string {
	var parts []string
	if p.Attrs[model.AttrCached] == "true" {
		parts = append(parts, "image already on node")
	}
	if m := p.Attrs["model"]; m != "" {
		parts = append(parts, m)
	}
	if g := p.Attrs["gib"]; g != "" {
		parts = append(parts, g+" GiB")
	}
	if n := p.Attrs["tokens"]; n != "" {
		parts = append(parts, n+" tokens")
	}
	return strings.Join(parts, ", ")
}

// duration is m:ss, h:mm:ss from an hour, and one decimal under a second (DESIGN.md 7.5).
func duration(s float64) string {
	if s < 1 {
		return fmt.Sprintf("%.1fs", math.Round(s*10)/10)
	}
	total := int(math.Round(s))
	h, m, sec := total/3600, total/60%60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

// fit cuts s to width columns, ending in "…" when cut.
func fit(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	for lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// wrap breaks s into lines of at most Width columns at spaces, indenting continuation lines,
// and styles each line.
func wrap(style lipgloss.Style, s, indent string) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		lead := strings.Repeat(" ", len(s)-len(strings.TrimLeft(s, " ")))
		switch {
		case line == "" && len(out) == 0:
			line = lead + word
		case line == "":
			line = indent + word
		case lipgloss.Width(line)+1+lipgloss.Width(word) <= Width:
			line += " " + word
		default:
			out = append(out, style.Render(fit(line, Width)))
			line = indent + word
		}
	}
	if line != "" {
		out = append(out, style.Render(fit(line, Width)))
	}
	return out
}
