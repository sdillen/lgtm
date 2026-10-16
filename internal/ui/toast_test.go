package ui_test

import (
	"strings"
	"testing"

	"time"

	"github.com/sdillen/lgtm/internal/diff"
	"github.com/sdillen/lgtm/internal/ui"
)

// TestMessagesShowAsToastInTheTopRightCorner: command results are
// framed in the top right corner, not in the status line.
func TestMessagesShowAsToastInTheTopRightCorner(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 60, 8)

	m = typeText(press(m, ":"), "theme")
	m, _ = enter(m)

	lines := strings.Split(screen(m), "\n")
	if !strings.HasSuffix(strings.TrimRight(lines[0], " "), "╮") {
		t.Fatalf("no toast box in the top right corner:\n%s", screen(m))
	}
	if !strings.Contains(lines[1], "theme:") || !strings.HasSuffix(strings.TrimRight(lines[1], " "), "│") {
		t.Errorf("toast lacks the message:\n%s", screen(m))
	}
	if last := lines[len(lines)-1]; strings.Contains(last, "theme:") {
		t.Errorf("message still in the status line: %q", last)
	}
}

// TestToastDecays: the toast disappears on the first decay tick after
// its lifetime; before that it stays.
func TestToastDecays(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{ToastTTL: time.Hour}), 60, 8)
	m = typeText(press(m, ":"), "theme")
	m, _ = enter(m)
	tick := m.Init()()

	m, _ = m.Update(tick)
	if !strings.Contains(screen(m), "theme:") {
		t.Fatalf("toast gone before its lifetime:\n%s", screen(m))
	}

	m = start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{ToastTTL: time.Nanosecond}), 60, 8)
	m = typeText(press(m, ":"), "theme")
	m, _ = enter(m)
	m, _ = m.Update(tick)
	if strings.Contains(screen(m), "theme:") {
		t.Errorf("toast still shown after its lifetime:\n%s", screen(m))
	}
}
