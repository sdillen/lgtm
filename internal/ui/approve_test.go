package ui_test

import (
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
