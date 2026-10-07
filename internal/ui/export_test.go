package ui_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// errClipboard is the error of a broken clipboard stub.
var errClipboard = errors.New("broken")

// writeFile creates path with content, creating parent directories.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// captureCopies installs a clipboard stub recording the copied text.
func captureCopies(opts *ui.Options) *[]string {
	var copied []string
	opts.Copy = func(text string) error {
		copied = append(copied, text)
		return nil
	}
	return &copied
}

// stateWithComments returns a state with two open comments on the file
// fixture: a must on line 1 and a question on line 3.
func stateWithComments() *review.State {
	s := review.NewState()
	s.AddComment(review.Comment{
		Path: "main.go", Start: 1, End: 1, Category: review.Must,
		Quote: "package main", Text: "Handle the error.",
	})
	s.AddComment(review.Comment{
		Path: "main.go", Start: 3, End: 3, Category: review.Question,
		Quote: "end", Text: "Why end?",
	})
	return s
}

// TestYCopiesThePromptOfAllOpenComments pins the acceptance criterion
// "Y kopiert alle offenen Kommentare als fertigen Prompt": the copied
// text is the rendered prompt with branch, base, round and the open
// comments, and the status line confirms the copy.
func TestYCopiesThePromptOfAllOpenComments(t *testing.T) {
	opts := ui.Options{
		State:  stateWithComments(),
		Branch: "feat/export",
		Base:   "main",
		Round:  1,
	}
	copied := captureCopies(&opts)
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 60, 5)

	m = press(m, "Y")

	if len(*copied) != 1 {
		t.Fatalf("copied %d prompts, want 1", len(*copied))
	}
	prompt := (*copied)[0]
	for _, want := range []string{
		"# Review feat/export (Basis: main, Runde 1)",
		"## main.go",
		"### must (Zeilen 1)",
		"Handle the error.",
		"### question – nur beantworten, nichts ändern (Zeilen 3)",
		"Why end?",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if !strings.Contains(screen(m), "2 comments copied as prompt to the clipboard") {
		t.Errorf("status line lacks the confirmation:\n%s", screen(m))
	}
}

// TestYCopiesOnlyTheOpenComments pins that comments which are no longer
// open are not exported.
func TestYCopiesOnlyTheOpenComments(t *testing.T) {
	state := stateWithComments()
	state.CommentList[1].Status = review.Status("done")
	opts := ui.Options{State: state}
	copied := captureCopies(&opts)
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 30, 5)

	press(m, "Y")

	if len(*copied) != 1 {
		t.Fatalf("copied %d prompts, want 1", len(*copied))
	}
	if strings.Contains((*copied)[0], "Why end?") {
		t.Errorf("prompt lists a done comment:\n%s", (*copied)[0])
	}
	if !strings.Contains((*copied)[0], "Handle the error.") {
		t.Errorf("prompt lacks the open comment:\n%s", (*copied)[0])
	}
}

// TestYCopiesThePromptTemplateFromTheProject pins the resolution order
// end to end: a project template in .lgtm wins over the built-in one.
func TestYCopiesThePromptTemplateFromTheProject(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir+"/.lgtm/prompt.template", "Project: {{.Branch}} ({{.Base}})"); err != nil {
		t.Fatal(err)
	}
	opts := ui.Options{
		State:  stateWithComments(),
		Dir:    dir,
		Branch: "feat/export",
		Base:   "main",
	}
	copied := captureCopies(&opts)
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 30, 5)

	press(m, "Y")

	if len(*copied) != 1 || (*copied)[0] != "Project: feat/export (main)" {
		t.Errorf("copied %q, want the rendered project template", (*copied))
	}
}

// TestYCopiesThePromptTemplateFromTheConfigDir pins the global template
// as the middle step of the resolution order.
func TestYCopiesThePromptTemplateFromTheConfigDir(t *testing.T) {
	cfgDir := t.TempDir()
	if err := writeFile(cfgDir+"/prompt.template", "Global: {{.Base}}"); err != nil {
		t.Fatal(err)
	}
	opts := ui.Options{
		State:  stateWithComments(),
		CfgDir: cfgDir,
		Base:   "main",
	}
	copied := captureCopies(&opts)
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 30, 5)

	press(m, "Y")

	if len(*copied) != 1 || (*copied)[0] != "Global: main" {
		t.Errorf("copied %q, want the rendered global template", (*copied))
	}
}

// TestYCopiesNothingWithoutComments pins that Y without open comments
// reports instead of copying an empty prompt.
func TestYCopiesNothingWithoutComments(t *testing.T) {
	opts := ui.Options{}
	copied := captureCopies(&opts)
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 60, 5)

	m = press(m, "Y")

	if len(*copied) != 0 {
		t.Errorf("copied %d prompts without comments, want 0", len(*copied))
	}
	if !strings.Contains(screen(m), "no open comments to copy") {
		t.Errorf("status line lacks the hint:\n%s", screen(m))
	}
}

// TestYCopiesOnlyTheCommentUnderTheCursor pins the acceptance criterion
// "yc kopiert nur den Kommentar unter dem Cursor": the cursor is on the
// comment note below its code line.
func TestYCopiesOnlyTheCommentUnderTheCursor(t *testing.T) {
	opts := ui.Options{State: stateWithComments()}
	copied := captureCopies(&opts)
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 60, 8)

	// The rows of the file fixture: line 1, its comment note, the
	// deleted line, the added line, line 3 and its comment note.
	m = press(m, "j", "j", "j", "j", "j", "y", "c")

	if len(*copied) != 1 {
		t.Fatalf("copied %d prompts, want 1", len(*copied))
	}
	if !strings.Contains((*copied)[0], "Why end?") {
		t.Errorf("prompt lacks the comment under the cursor:\n%s", (*copied)[0])
	}
	if strings.Contains((*copied)[0], "Handle the error.") {
		t.Errorf("prompt contains a comment not under the cursor:\n%s", (*copied)[0])
	}
	if !strings.Contains(screen(m), "comment copied to the clipboard") {
		t.Errorf("status line lacks the confirmation:\n%s", screen(m))
	}
}

// TestYCWithoutCommentUnderTheCursorReports pins that yc on a code line
// (no comment note) reports instead of copying.
func TestYCWithoutCommentUnderTheCursorReports(t *testing.T) {
	opts := ui.Options{State: stateWithComments()}
	copied := captureCopies(&opts)
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 60, 8)

	m = press(m, "y", "c")

	if len(*copied) != 0 {
		t.Errorf("copied %d prompts without a comment under the cursor, want 0", len(*copied))
	}
	if !strings.Contains(screen(m), "move the cursor to a comment to copy") {
		t.Errorf("status line lacks the hint:\n%s", screen(m))
	}
}

// TestCopyReportsClipboardFailures pins that a failing clipboard shows
// the error in the status line instead of claiming success.
func TestCopyReportsClipboardFailures(t *testing.T) {
	opts := ui.Options{State: stateWithComments(), Copy: func(string) error {
		return errClipboard
	}}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 60, 5)

	m = press(m, "Y")

	if !strings.Contains(screen(m), "clipboard: broken") {
		t.Errorf("status line lacks the clipboard error:\n%s", screen(m))
	}
}

// TestQWithOpenCommentsWarnsBeforeQuitting pins the acceptance criterion
// "Beim Beenden mit offenen Kommentaren erscheint beim ersten q ein
// Hinweis": the first q only warns, the second q quits with exit code 1.
func TestQWithOpenCommentsWarnsBeforeQuitting(t *testing.T) {
	opts := ui.Options{State: stateWithComments()}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 5)

	m, cmd := m.Update(key("q"))
	if cmd != nil {
		t.Fatalf("first q returned %T, want no command", cmd())
	}
	if !strings.Contains(screen(m), "2 offene Kommentare") || !strings.Contains(screen(m), "Y kopieren") {
		t.Errorf("status line lacks the quit warning:\n%s", screen(m))
	}
	if m.(ui.Model).ExitCode() != 0 {
		t.Error("exit code changed although the review is still open")
	}

	m, cmd = m.Update(key("q"))
	if cmd == nil {
		t.Fatal("second q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("second q returned %T, want tea.QuitMsg", cmd())
	}
	if m.(ui.Model).ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1 for open comments", m.(ui.Model).ExitCode())
	}
}

// TestQWithoutCommentsQuitsCleanly pins the exit contract: quitting
// without open comments is exit code 0.
func TestQWithoutCommentsQuitsCleanly(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 30, 5)

	m, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q returned %T, want tea.QuitMsg", cmd())
	}
	if m.(ui.Model).ExitCode() != 0 {
		t.Errorf("exit code = %d, want 0 without open comments", m.(ui.Model).ExitCode())
	}
}

// TestAnyKeyDisarmsTheQuitWarning pins that the warning is for the next
// q only: a different key in between makes q warn again.
func TestAnyKeyDisarmsTheQuitWarning(t *testing.T) {
	opts := ui.Options{State: stateWithComments()}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 5)

	m, _ = m.Update(key("q")) // warns
	m = press(m, "j")         // any other action disarms
	m, cmd := m.Update(key("q"))

	if cmd != nil {
		t.Fatalf("q after a different key returned %T, want the warning again", cmd())
	}
	if !strings.Contains(screen(m), "2 offene Kommentare") {
		t.Errorf("status line lacks the warning:\n%s", screen(m))
	}
}

// TestColonQWithOpenCommentsWarns pins that the command line :q warns
// like the q key.
func TestColonQWithOpenCommentsWarns(t *testing.T) {
	opts := ui.Options{State: stateWithComments()}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 5)

	m = typeText(press(m, ":"), "q")
	m, cmd := enter(m)

	if cmd != nil {
		t.Fatalf(":q returned %T, want the warning", cmd())
	}
	if !strings.Contains(screen(m), "2 offene Kommentare") {
		t.Errorf("status line lacks the quit warning:\n%s", screen(m))
	}
}

// TestColonQForceQuitsWithoutWarning pins that :q! quits immediately;
// the exit code still reports the open comments.
func TestColonQForceQuitsWithoutWarning(t *testing.T) {
	opts := ui.Options{State: stateWithComments()}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 5)

	m = typeText(press(m, ":"), "q!")
	m, cmd := enter(m)

	if cmd == nil {
		t.Fatal(":q! returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf(":q! returned %T, want tea.QuitMsg", cmd())
	}
	if m.(ui.Model).ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1 for open comments", m.(ui.Model).ExitCode())
	}
}

// TestCtrlCQuitsAndReportsOpenComments pins that ctrl+c always quits,
// with the exit code of the review state.
func TestCtrlCQuitsAndReportsOpenComments(t *testing.T) {
	opts := ui.Options{State: stateWithComments()}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 5)

	m, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	if cmd == nil {
		t.Fatal("ctrl+c returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c returned %T, want tea.QuitMsg", cmd())
	}
	if m.(ui.Model).ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1 for open comments", m.(ui.Model).ExitCode())
	}
}

// TestColonLgtmApprovesAndQuitsWithZero pins the acceptance criterion
// ":lgtm beendet mit Exit-Code 0": a confirmation moment, then quit.
func TestColonLgtmApprovesAndQuitsWithZero(t *testing.T) {
	opts := ui.Options{State: stateWithComments(), ApprovalDelay: time.Nanosecond}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 5)

	m = typeText(press(m, ":"), "lgtm")
	m, cmd := enter(m)

	if cmd == nil {
		t.Fatal(":lgtm returned no command")
	}
	if !strings.Contains(screen(m), "lgtm") || !strings.Contains(screen(m), "approved") {
		t.Errorf("status line lacks the approval moment:\n%s", screen(m))
	}
	msg := cmd()
	m, cmd = m.Update(msg)
	if cmd == nil {
		t.Fatal("approval moment returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("approval moment returned %T, want tea.QuitMsg", cmd())
	}
	if m.(ui.Model).ExitCode() != 0 {
		t.Errorf("exit code = %d, want 0 after :lgtm", m.(ui.Model).ExitCode())
	}
}

// TestColonLgtmApprovesWithoutComments pins that :lgtm approves an
// empty review the same way.
func TestColonLgtmApprovesWithoutComments(t *testing.T) {
	opts := ui.Options{ApprovalDelay: time.Nanosecond}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 5)

	m = typeText(press(m, ":"), "lgtm")
	m, cmd := enter(m)
	msg := cmd()
	m, _ = m.Update(msg)

	if m.(ui.Model).ExitCode() != 0 {
		t.Errorf("exit code = %d, want 0 after :lgtm", m.(ui.Model).ExitCode())
	}
}

// TestHelpListsTheExportCommands pins that the help documents :lgtm and
// the force quit.
func TestHelpListsTheExportCommands(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 80, 40)

	m = press(m, "?")
	for _, want := range []string{":lgtm", ":q!", "Y", "yc"} {
		if !strings.Contains(screen(m), want) {
			t.Errorf("help lacks %q:\n%s", want, screen(m))
		}
	}
}
