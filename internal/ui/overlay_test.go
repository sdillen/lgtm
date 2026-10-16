package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sdillen/lgtm/internal/diff"
	"github.com/sdillen/lgtm/internal/ui"
)

// TestOverlayDimsTheViewBehind pins the acceptance criterion
// "Overlays mit ... Hintergrundabdunklung": the scrim darkens the
// colours of the view behind the open drawer, while the overlay's own
// border stays bright.
func TestOverlayDimsTheViewBehind(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 60, 10)

	// #7aa2f7 is the status accent of the night theme, undimmed
	// without an overlay.
	if !strings.Contains(m.View().Content, "38;2;122;162;247") {
		t.Fatalf("view lacks the accent colour:\n%s", m.View().Content)
	}

	m = press(leader(m), "d")
	content := m.View().Content

	// The drawer's own border keeps the accent colour...
	if !strings.Contains(content, "38;2;122;162;247") {
		t.Errorf("overlay border lost its colour:\n%s", content)
	}
	// ...and the view behind is dimmed to 55%: 122→67, 162→89, 247→135.
	if !strings.Contains(content, "38;2;67;89;135") {
		t.Errorf("scrim does not dim the view behind the overlay:\n%s", content)
	}
}

// TestOverlaySlidesIn pins the acceptance criterion "Overlays mit ...
// Animation": the drawer starts above the screen edge and glides to
// its final position, which matches the static overlay.
func TestOverlaySlidesIn(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, animated()), 40, 10)
	still := press(leader(start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 40, 10)), "d")

	var cmd tea.Cmd
	m, _ = m.Update(teaKeySpace())
	m, cmd = m.Update(key("d"))
	// The first frame still shows the drawer off the screen edge;
	// only the scrim dims the view.
	if strings.Contains(screen(m), "Files") {
		t.Errorf("drawer visible before the slide:\n%s", screen(m))
	}

	m = settle(m, cmd)
	if !strings.Contains(screen(m), "Files") {
		t.Errorf("drawer did not slide in:\n%s", screen(m))
	}
	if screen(m) != screen(still) {
		t.Errorf("settled drawer differs from the static overlay:\n%s\nwant:\n%s",
			screen(m), screen(still))
	}
}

// TestDrawerRendersTheScrimmedOverlay pins the core overlay view: the
// drawer over the dimmed file view, as a golden frame.
func TestDrawerRendersTheScrimmedOverlay(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 60, 12)
	m = press(leader(m), "d")

	assertGolden(t, "drawer.txt", screen(m))
}

// teaKeySpace presses the leader key like the runtime sends it.
func teaKeySpace() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}
