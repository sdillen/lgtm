package ui_test

import (
	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
	"strings"
	"testing"
	"time"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// TestLgtmShowsTheConfirmationMoment pins the acceptance criterion
// ":lgtm zeigt den Bestätigungsmoment": after :lgtm the glowing
// "✓ Looks Good To Me" box floats over the dimmed review until the program quits.
func TestLgtmShowsTheConfirmationMoment(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor,
		ui.Options{ApprovalDelay: time.Hour}), 60, 10)

	m = typeText(press(m, ":"), "lgtm")
	m, _ = enter(m)

	assertGolden(t, "lgtm.txt", screen(m))
	if !strings.Contains(screen(m), "✓ Looks Good To Me") || !strings.Contains(screen(m), "review approved") {
		t.Errorf("confirmation moment lacks the Looks-good-to-me box:\n%s", screen(m))
	}
}

// runUntilQuit runs the commands cmd returned and feeds their messages back
// into m until the program asks to quit; it reports whether it did.
// Commands that outlast the wait (animation ticks) are ignored.
func runUntilQuit(m tea.Model, cmd tea.Cmd) (tea.Model, bool) {
	if cmd == nil {
		return m, false
	}
	msgs := make(chan tea.Msg, 1)
	go func() { msgs <- cmd() }()
	select {
	case msg := <-msgs:
		if _, ok := msg.(tea.QuitMsg); ok {
			return m, true
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if m, quit := runUntilQuit(m, c); quit {
					return m, true
				}
			}
			return m, false
		}
		m, next := m.Update(msg)
		return runUntilQuit(m, next)
	case <-time.After(100 * time.Millisecond):
		return m, false
	}
}

// approveOpts are options whose Clear counts its calls.
func approveOpts(calls *int) ui.Options {
	opts := withState("")
	opts.ApprovalDelay = time.Millisecond
	opts.Clear = func() error { *calls++; return nil }
	return opts
}

func TestLgtmClearsTheReviewAfterTheConfirmationMoment(t *testing.T) {
	calls := 0
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, approveOpts(&calls)), 60, 10)

	m = typeText(press(m, ":"), "lgtm")
	m, cmd := enter(m)
	if calls != 0 {
		t.Fatalf("review cleared %d times before the confirmation moment ended", calls)
	}
	if _, quit := runUntilQuit(m, cmd); !quit {
		t.Fatal("approving never quit")
	}
	if calls != 1 {
		t.Errorf("review cleared %d times after approving, want once", calls)
	}
}

func TestLgtmWithOpenCommentsAsksBeforeDiscardingThem(t *testing.T) {
	calls := 0
	opts := approveOpts(&calls)
	opts.State.AddComment(review.Comment{Path: "a.go", Start: 1, End: 1, Quote: "x", Text: "one"})
	opts.State.AddComment(review.Comment{Path: "a.go", Start: 1, End: 1, Quote: "x", Text: "two"})
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 80, 10)

	m = typeText(press(m, ":"), "lgtm")
	m, cmd := enter(m)

	if s := screen(m); !strings.Contains(s, "2 offene Kommentare werden verworfen") {
		t.Errorf("no warning about the open comments:\n%s", s)
	}
	if _, quit := runUntilQuit(m, cmd); quit || calls != 0 {
		t.Fatalf("first :lgtm approved (quit=%v, cleared %d times) without confirmation", quit, calls)
	}

	m = typeText(press(m, ":"), "lgtm")
	m, cmd = enter(m)
	if _, quit := runUntilQuit(m, cmd); !quit || calls != 1 {
		t.Errorf("confirmed :lgtm: quit=%v, cleared %d times, want quit and once", quit, calls)
	}
}
