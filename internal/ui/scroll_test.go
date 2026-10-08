package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// animated is the options of the animation tests: smooth scrolling and
// slide-in on.
func animated() ui.Options {
	on := true
	return ui.Options{Animate: &on}
}

// settle feeds the scheduled animation frames back into the model
// until no tick is scheduled any more, like the runtime would.
func settle(m tea.Model, cmd tea.Cmd) tea.Model {
	for i := 0; cmd != nil && i < 200; i++ {
		m, cmd = m.Update(cmd())
	}
	return m
}

// TestSmoothScrollAnimatesHalfPageDown pins the acceptance criterion
// "Smooth Scrolling flüssig": with the animation on, <C-d> starts a
// glide from the old position, and the spring settles on the same
// target as the instant jump.
func TestSmoothScrollAnimatesHalfPageDown(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, animated()), 40, 5)

	m, cmd := m.Update(ctrl("d"))
	// The first frame still shows the old position; the scroll runs
	// in the background.
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})
	if cmd == nil {
		t.Fatal("animated half-page scroll returned no command")
	}

	// One frame later the view has started to glide.
	m, cmd = m.Update(cmd())
	if rows := strings.Split(screen(m), "\n"); rows[0] != "   2      - b" {
		t.Errorf("animation did not move the view:\n%s", screen(m))
	}

	// The glide settles on the target of the instant jump.
	m = settle(m, cmd)
	assertScreen(t, m, []string{
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"big.go  1/1  +2 −2",
	})
}

// TestSmoothScrollAnimatesJumps pins that jumps glide too: G starts at
// the old position and settles at the last line.
func TestSmoothScrollAnimatesJumps(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, animated()), 40, 5)

	m, cmd := m.Update(key("G"))
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})

	m = settle(m, cmd)
	assertScreen(t, m, []string{
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"⋯ 10 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})
}

// TestSmoothScrollFinishesOnTheNextKey pins that a key during the
// glide completes the scroll instead of racing it: the view sits at
// the target, and the key acts from there.
func TestSmoothScrollFinishesOnTheNextKey(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, animated()), 40, 5)

	m, _ = m.Update(ctrl("d"))
	m = press(m, "j")
	assertScreen(t, m, []string{
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"big.go  1/1  +2 −2",
	})
}

// TestSmoothScrollIsOffWithoutTheOption pins that the zero options
// scroll instantly: the default is the nvim-like jump.
func TestSmoothScrollIsOffWithoutTheOption(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 40, 5)

	m, cmd := m.Update(ctrl("d"))
	if cmd != nil {
		t.Error("instant scroll returned a command")
	}
	assertScreen(t, m, []string{
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"big.go  1/1  +2 −2",
	})
}

// TestFileJumpsAreNotAnimated pins that jumps to another file show the
// new file directly: the glide would scroll over the old content.
func TestFileJumpsAreNotAnimated(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big, small}, "main", themeFor, animated()), 40, 3)

	var cmd tea.Cmd
	m, _ = m.Update(key("]"))
	m, cmd = m.Update(key("f"))
	if cmd != nil {
		t.Error("file jump returned an animation command")
	}
	assertScreen(t, m, []string{
		"   1      - x",
		"        1 + X",
		"small.txt  2/2  +1 −1  ▰▰▰▱▱▱ 1/2",
	})
}
