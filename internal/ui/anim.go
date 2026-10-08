package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/harmonica"
)

// frameInterval is the cadence of the animation frames, 60 fps.
const frameInterval = 16 * time.Millisecond

// maxAnimFrames caps an animation, so a spring that does not settle
// cannot tick forever.
const maxAnimFrames = 30

// frameMsg ticks the animations: the smooth scroll of the view and the
// slide-in of the open overlay.
type frameMsg struct{}

// spring animates one quantity of the UI: the offset or the cursor of
// the smooth scroll, or the y position of a sliding overlay. The
// harmonica spring (60 Hz, frequency 60, damping 0.8) settles a long
// jump in about nine frames (~150 ms), slightly overshooting, which
// reads as the smooth nvim-like glide.
type spring struct {
	s    harmonica.Spring
	pos  float64
	vel  float64
	done bool
}

func newSpring(pos float64) spring {
	return spring{
		s:   harmonica.NewSpring(harmonica.FPS(60), 60, 0.8),
		pos: pos,
	}
}

// step advances the spring one frame toward target and snaps it once
// it is within half a pixel.
func (s *spring) step(target float64) {
	s.pos, s.vel = s.s.Update(s.pos, s.vel, target)
	if s.pos > target-0.5 && s.pos < target+0.5 {
		s.done = true
		s.pos = target
	}
}

// scrollTween is the active smooth scroll: two springs carry the
// offset and the cursor from their positions to the targets of the
// move. The springs share their parameters, so viewport and cursor
// arrive together.
type scrollTween struct {
	off, cur spring
	off1     int
	cur1     int
	frames   int
}

// stepScroll advances the smooth scroll one frame.
func (m *Model) stepScroll() {
	t := m.smooth
	t.frames++
	t.off.step(float64(t.off1))
	t.cur.step(float64(t.cur1))
	m.offset = int(t.off.pos + 0.5)
	m.cursor = int(t.cur.pos + 0.5)
	if (t.off.done && t.cur.done) || t.frames >= maxAnimFrames {
		m.offset, m.cursor = t.off1, t.cur1
		m.smooth = nil
	}
}

// animate runs the move and smoothly scrolls the view from its old to
// its new position; without animations the move is instant. Jumps to
// another file are not animated: the new file appears at its start.
func (m *Model) animate(move func()) {
	if !m.animOn() {
		move()
		return
	}
	file := m.fileIdx
	off0, cur0 := m.offset, m.cursor
	move()
	if m.fileIdx != file {
		// The move jumped to another file and reset the view; the
		// scroll would glide over the old file.
		m.scrollCursorIntoView()
		return
	}
	m.scrollCursorIntoView()
	off1, cur1 := m.offset, m.cursor
	if off1 == off0 && cur1 == cur0 {
		return
	}
	m.smooth = &scrollTween{
		off:  newSpring(float64(off0)),
		cur:  newSpring(float64(cur0)),
		off1: off1,
		cur1: cur1,
	}
	// The springs start at the old position; the view stays there
	// until the first frame arrives.
	m.offset, m.cursor = off0, cur0
}

// finishAnims completes a running scroll immediately, as if it had
// reached its target; the next key or a resize interrupts the glide.
func (m *Model) finishAnims() {
	if m.smooth != nil {
		m.offset, m.cursor = m.smooth.off1, m.smooth.cur1
		m.smooth = nil
	}
}

// slideTween is the slide-in of the open overlay: the box starts off
// the edge of the screen and glides to its final position.
// overlayY animates the position, the spring settles at the target.
type slideTween struct {
	spring spring
	to     int
}

// startSlide begins the slide-in of an overlay from the edge of the
// screen: from below for the centred and bottom overlays, from above
// (negative) for the drawer. Without animations the overlay appears at
// its final position right away.
func (m *Model) startSlide(from, to int) {
	m.slide = nil
	if !m.animOn() {
		return
	}
	m.slide = &slideTween{spring: newSpring(float64(from)), to: to}
}

// startSlideOverlay begins the slide-in of the overlay that is open:
// the box is measured and its final position is the slide target.
func (m *Model) startSlideOverlay(from int) {
	if _, _, y, ok := m.overlayBox(); ok {
		m.startSlide(from, y)
	} else {
		m.slide = nil
	}
}

// stepSlide advances the slide-in one frame.
func (m *Model) stepSlide() {
	s := m.slide
	s.spring.step(float64(s.to))
	if s.spring.done {
		m.slide = nil
	}
}

// overlayY is the y position of the open overlay: the animated value
// while the box slides to its target, the final position otherwise. A
// layout change (e.g. a resize) moves the box to its new final
// position directly; the stale slide finishes on its own.
func (m Model) overlayY(to int) int {
	if m.slide == nil || m.slide.to != to {
		return to
	}
	return int(m.slide.spring.pos + 0.5)
}

// stepFrames advances the active animations.
func (m *Model) stepFrames() {
	if m.smooth != nil {
		m.stepScroll()
		m.maybeMarkRead()
	}
	if m.slide != nil {
		m.stepSlide()
	}
	if m.fade != nil {
		m.stepFade()
	}
	if m.flash != nil {
		m.flash.frames++
		if m.flash.frames >= flashFrames {
			m.flash = nil
		}
	}
	if m.approved {
		m.shimmer++
	}
}

// frameCmd ticks the animations once.
func (m Model) frameCmd() tea.Cmd {
	return tea.Tick(frameInterval, func(time.Time) tea.Msg { return frameMsg{} })
}

// animCmd is the frame tick while an animation runs, nil otherwise.
func (m Model) animCmd() tea.Cmd {
	if m.smooth != nil || m.slide != nil || m.fade != nil || m.flash != nil || (m.approved && m.animOn()) {
		return m.frameCmd()
	}
	return nil
}

// animOn reports whether the animations run: the Animate option turns
// them on, nil means off.
func (m Model) animOn() bool {
	return m.opts.Animate != nil && *m.opts.Animate
}
