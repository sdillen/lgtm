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
	if !strings.Contains(ansi.Strip(m.View().Content), "✓ LGTM") {
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

// TestStatusLineRunsThroughAGradient pins that the status line is
// coloured along a gradient while its text stays the same.
func TestStatusLineRunsThroughAGradient(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 60, 6)
	rows := strings.Split(m.View().Content, "\n")
	last := rows[len(rows)-1]
	if got := ansi.Strip(last); !strings.Contains(got, "main.go  1/1  +1 −1") {
		t.Errorf("status text changed: %q", got)
	}
	if n := colours(last); n < 5 {
		t.Errorf("status line has %d distinct colours, want a gradient", n)
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
