package ui

import (
	"fmt"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/export"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
)

// copyPrompt renders the prompt for all open comments and copies it to
// the clipboard: the template is resolved project-local over global
// over built-in, the clipboard is OSC 52 with a tool fallback.
func (m *Model) copyPrompt() {
	if len(m.state.OpenComments()) == 0 {
		m.fail("no open comments to copy")
		return
	}
	m.copy(export.Data{Comments: export.OpenComments(m.state)})
}

// copyComment renders the prompt for the comment under the cursor and
// copies it to the clipboard.
func (m *Model) copyComment() {
	c, ok := m.commentAtCursor()
	if !ok {
		m.fail("move the cursor to a comment to copy")
		return
	}
	m.copy(export.Data{Comments: []review.Comment{c}})
}

// copy renders the prompt for the comments and writes it to the
// clipboard, reporting success or failure in the status line.
func (m *Model) copy(data export.Data) {
	data.Branch, data.Base, data.Round = m.opts.Branch, m.opts.Base, m.opts.Round
	tmpl, err := export.Resolve(m.dir, m.opts.CfgDir)
	if err != nil {
		m.fail("prompt: " + err.Error())
		return
	}
	prompt, err := export.Render(tmpl, data)
	if err != nil {
		m.fail("prompt: " + err.Error())
		return
	}
	copyText := export.Copy
	if m.opts.Copy != nil {
		copyText = m.opts.Copy
	}
	if err := copyText(prompt); err != nil {
		m.fail("clipboard: " + err.Error())
		return
	}
	if len(data.Comments) == 1 {
		m.message = "comment copied to the clipboard"
	} else {
		m.message = fmt.Sprintf("%d comments copied as prompt to the clipboard", len(data.Comments))
	}
}

// commentAtCursor returns the comment whose note row the cursor is on.
func (m Model) commentAtCursor() (review.Comment, bool) {
	if len(m.files) == 0 {
		return review.Comment{}, false
	}
	r := m.rows()[m.cursor]
	if r.note == nil {
		return review.Comment{}, false
	}
	return r.note.comment, true
}
