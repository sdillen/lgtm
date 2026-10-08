package ui_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// TestThemeSwitchFadesInstead pins the cross-fade: with animations on,
// :theme does not switch at once but runs frames, and the end state
// renders exactly like the instant switch.
func TestThemeSwitchFadesInstead(t *testing.T) {
	day := theme.Default(theme.KindLight)
	load := func(string) (theme.Theme, error) { return day, nil }
	opts := animated()
	opts.LoadTheme = load

	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 6)
	before := m.View().Content
	m = typeText(press(m, ":"), "theme day")
	m, cmd := enter(m)
	if cmd == nil {
		t.Fatal(":theme returned no frame tick, the fade does not run")
	}
	if m.View().Content != before {
		t.Error("the fade must start at the old look")
	}
	m, cmd = m.Update(cmd())
	mid := m.View().Content
	if mid == before {
		t.Error("one frame later the fade has not started")
	}
	m = settle(m, cmd)

	plain := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{LoadTheme: load}), 40, 6)
	plain, _ = enter(typeText(press(plain, ":"), "theme day"))
	if got, want := m.View().Content, plain.View().Content; got != want {
		t.Errorf("fade did not settle on the instant result:\n%q\nwant\n%q", got, want)
	}
}

// TestReloadFlashesNewLines pins the reload flash: a line that is new
// after a reload is washed towards the accent colour and fades back to
// its normal look; old lines never change.
func TestReloadFlashesNewLines(t *testing.T) {
	next := diff.ReviewFile{
		Path: "main.go",
		Lines: append(append([]diff.Line{}, file.Lines...),
			diff.Line{Kind: diff.Added, New: 4, Text: "fresh"}),
	}
	opts := animated()
	opts.Reload = func() ([]diff.ReviewFile, error) { return []diff.ReviewFile{next}, nil }

	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 8)
	m, cmd := m.Update(key("R"))
	if cmd == nil {
		t.Fatal("reload scheduled no frames for the flash")
	}
	flashed := m.View().Content
	m = settle(m, cmd)
	rest := m.View().Content
	if flashed == rest {
		t.Error("the flash left no trace and did not fade")
	}
	// Reload again without a change: nothing is new, nothing flashes.
	m2, cmd2 := m.Update(key("R"))
	if cmd2 != nil {
		t.Error("reload without new lines should not flash")
	}
	if m2.View().Content != rest {
		t.Error("lines that were already there must not flash")
	}
}

// TestApprovalShimmers pins that the approval box keeps animating
// while it waits, and that the animation ticks end with the program.
func TestApprovalShimmers(t *testing.T) {
	opts := animated()
	opts.ApprovalDelay = 1 << 40
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 60, 10)
	m = typeText(press(m, ":"), "lgtm")
	m, cmd := enter(m)
	if cmd == nil {
		t.Fatal(":lgtm returned no command")
	}
	// The batch holds the approval timer and the frame tick; the
	// timer would block for the whole delay.
	var frame tea.Cmd
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			done := make(chan tea.Msg, 1)
			go func() { done <- c() }()
			select {
			case got := <-done:
				if _, ok := got.(tea.BatchMsg); !ok {
					frame = func() tea.Msg { return got }
				}
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	if frame == nil {
		t.Fatal("approval started no frame ticks")
	}
	a := m.View().Content
	for range 3 {
		m, _ = m.Update(frame())
	}
	if b := m.View().Content; a == b {
		t.Error("the approval gradient does not move")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "✓ Looks Good To Me") {
		t.Error("shimmer lost the LGTM text")
	}
}

// colours counts the distinct truecolour foregrounds in rendered text.
func colours(s string) int {
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`\x1b\[[0-9;]*38;2;(\d+;\d+;\d+)`).FindAllStringSubmatch(s, -1) {
		seen[m[1]] = true
	}
	return len(seen)
}

// TestStatusLineRunsThroughAGradient pins that every element of the
// status line has its own colour while its text stays the same.
func TestStatusLineRunsThroughAGradient(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 60, 6)
	rows := strings.Split(m.View().Content, "\n")
	last := rows[len(rows)-1]
	if got := ansi.Strip(last); !strings.Contains(got, "main.go  1/1  +1 −1") {
		t.Errorf("status text changed: %q", got)
	}
	if n := colours(last); n < 3 {
		t.Errorf("status line has %d distinct colours, want one per element", n)
	}
}

// TestStartScreenBarsHeatUp pins the heat ramp of the distribution
// bars: a filled bar spans several colours, not one flat fill.
func TestStartScreenBarsHeatUp(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{StartScreen: true}), 60, 12)
	var bar string
	for _, l := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(l, "█") {
			bar = l
		}
	}
	if bar == "" {
		t.Fatal("no distribution bar on the start screen")
	}
	if n := colours(bar); n < 3 {
		t.Errorf("bar has %d colours, want a ramp", n)
	}
}

// TestStatusLineSqueezesOnNarrowTerminals pins idea 7: the progress
// label goes first, then the bar, and only then the path is cut in the
// middle, keeping the file name and the state markers.
func TestStatusLineSqueezesOnNarrowTerminals(t *testing.T) {
	long := diff.ReviewFile{Path: "internal/deeply/nested/window.go", Lines: file.Lines}
	files := []diff.ReviewFile{long, file}
	last := func(width int) string {
		m := start(ui.New(files, "main", themeFor, ui.Options{}), width, 5)
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	for _, c := range []struct {
		width int
		want  []string
		not   []string
	}{
		{80, []string{"internal/deeply/nested/window.go", "▰", "gelesen"}, nil},
		{60, []string{"internal/deeply/nested/window.go", "▰"}, []string{"gelesen"}},
		{46, []string{"internal/deeply/nested/window.go", "1/2", "+1 −1"}, []string{"▰"}},
		{30, []string{"…/window.go", "+1 −1"}, []string{"▰", "nested"}},
	} {
		got := last(c.width)
		if w := ansi.StringWidth(got); w > c.width {
			t.Errorf("width %d: line is %d cells wide: %q", c.width, w, got)
		}
		for _, s := range c.want {
			if !strings.Contains(got, s) {
				t.Errorf("width %d: %q lacks %q", c.width, got, s)
			}
		}
		for _, s := range c.not {
			if strings.Contains(got, s) {
				t.Errorf("width %d: %q still has %q", c.width, got, s)
			}
		}
	}
}

// TestStatusLineNamesTheReviewRange pins the review range at the start
// of the status line and that it gives way before the path does.
func TestStatusLineNamesTheReviewRange(t *testing.T) {
	last := func(width int) string {
		m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{Base: "main", Branch: "feat/wow"}), width, 5)
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	if got := last(100); !strings.HasPrefix(got, "main → feat/wow  main.go  1/1") {
		t.Errorf("width 100: %q, want the range before the path", got)
	}
	if got := last(30); strings.Contains(got, "feat/wow") || !strings.HasPrefix(got, "main.go  1/1") {
		t.Errorf("width 30: %q, want the full path without the range", got)
	}
}

// TestStatusLineShowsProgressAndBadges pins the progress bar of read
// files and the open comments as badges per category.
func TestStatusLineShowsProgressAndBadges(t *testing.T) {
	st := stateWithComments()
	m := start(ui.New([]diff.ReviewFile{file, big}, "main", themeFor, ui.Options{State: st}), 100, 8)
	line := func() string {
		rows := strings.Split(m.View().Content, "\n")
		return rows[len(rows)-1]
	}
	if got := ansi.Strip(line()); !strings.Contains(got, "/2 gelesen") {
		t.Errorf("no progress in %q", got)
	}
	got := ansi.Strip(line())
	if !strings.Contains(got, " 1 must ") || !strings.Contains(got, "● 1 question") {
		t.Errorf("no comment badges in %q", got)
	}
}

// TestStatusLineIsSetOffByARule pins the thin rule above the status
// line: it spans the terminal width, sits directly above the status
// line, and tiny terminals keep the row for the code.
func TestStatusLineIsSetOffByARule(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 30, 6)
	rows := strings.Split(ansi.Strip(m.View().Content), "\n")
	if len(rows) != 7 {
		t.Fatalf("view has %d rows, want the terminal height 7:\n%s", len(rows), strings.Join(rows, "\n"))
	}
	if want := strings.Repeat("─", 30); rows[len(rows)-2] != want {
		t.Errorf("row above the status line = %q, want the rule", rows[len(rows)-2])
	}
	if !strings.HasPrefix(rows[len(rows)-1], "main.go") {
		t.Errorf("last row = %q, want the status line", rows[len(rows)-1])
	}

	tiny, _ := ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}).Update(tea.WindowSizeMsg{Width: 30, Height: 3})
	if strings.Contains(tiny.View().Content, "──────") {
		t.Error("a 3-row terminal must keep its rows for the code, not for a rule")
	}
}
