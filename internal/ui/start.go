package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// maxDirBars is how many directories the start screen's distribution
// lists; the rest is cut.
const maxDirBars = 6

// barWidth is the width of the directory distribution bars.
const barWidth = 8

// startBox is the overview shown at startup: branch, base, round, the
// size of the review and the distribution of the changes over the
// directories. It disappears on the first key press.
func (m Model) startBox() string {
	var lines []string
	where := ""
	if m.opts.Branch != "" {
		where += "Branch " + m.opts.Branch
	}
	if m.opts.Base != "" {
		if where != "" {
			where += "  "
		}
		where += "Base " + m.opts.Base
	}
	if where != "" {
		lines = append(lines, m.theme.Styles.Status.Render(where))
	}
	if m.opts.Round > 0 {
		lines = append(lines, fmt.Sprintf("Runde %d", m.opts.Round))
	}
	added, deleted := m.totalStats()
	lines = append(lines, fmt.Sprintf("%d Dateien · +%d −%d", len(m.files), added, deleted))
	lines = append(lines, "")
	lines = append(lines, m.dirBars()...)
	lines = append(lines, "", m.theme.Styles.Fold.Render("Press any key to review"))
	return m.box("lgtm", lines)
}

// totalStats are the added and deleted lines of the whole review.
func (m Model) totalStats() (added, deleted int) {
	for _, f := range m.files {
		a, d := f.Stats()
		added += a
		deleted += d
	}
	return added, deleted
}

// dirStat is the change size of one directory.
type dirStat struct {
	name    string
	added   int
	deleted int
}

// dirBars are the distribution bars of the review: one line per
// directory, sorted by the size of the change, with a bar of the
// share of the whole review and the +/− numbers.
func (m Model) dirBars() []string {
	dirs := map[string]*dirStat{}
	for _, f := range m.files {
		a, d := f.Stats()
		if a+d == 0 {
			continue
		}
		name := "."
		if i := strings.LastIndexByte(f.Path, '/'); i >= 0 {
			name = f.Path[:i]
		}
		s, ok := dirs[name]
		if !ok {
			s = &dirStat{name: name}
			dirs[name] = s
		}
		s.added += a
		s.deleted += d
	}
	stats := make([]*dirStat, 0, len(dirs))
	total := 0
	for _, s := range dirs {
		stats = append(stats, s)
		total += s.added + s.deleted
	}
	if total == 0 {
		return nil
	}
	sort.Slice(stats, func(i, j int) bool {
		ti, tj := stats[i].added+stats[i].deleted, stats[j].added+stats[j].deleted
		if ti != tj {
			return ti > tj
		}
		return stats[i].name < stats[j].name
	})
	if len(stats) > maxDirBars {
		stats = stats[:maxDirBars]
	}
	w := 0
	for _, s := range stats {
		w = max(w, ansi.StringWidth(s.name))
	}
	lines := make([]string, 0, len(stats))
	for _, s := range stats {
		n := s.added + s.deleted
		fill := (n*barWidth + total/2) / total
		bar := strings.Repeat("█", fill) + strings.Repeat("░", barWidth-fill)
		name := s.name + strings.Repeat(" ", w-ansi.StringWidth(s.name))
		lines = append(lines, fmt.Sprintf("%s  %s  +%d −%d", name, bar, s.added, s.deleted))
	}
	return lines
}
