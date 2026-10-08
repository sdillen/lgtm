package main

import (
	"time"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/source"
)

// rounds are the review rounds of a run: the snapshot of the current
// round is retaken on R, and the interdiff compares with the snapshot
// of the previous round.
type rounds struct {
	repo  *lockedSource
	subj  subject
	state *review.State
}

// begin resumes the current round when the reviewed state still is its
// snapshot, and starts the next round otherwise: the reviewed state is
// snapshotted, the comments move from the snapshot to their code in
// the reviewed state, and files that changed since they were read
// become unread again. files are the review files of the reviewed
// state.
func (r rounds) begin(files []diff.ReviewFile) error {
	n := r.state.Round()
	if n > 0 {
		since, err := r.repo.since(r.subj, r.state.Snapshot(n))
		// A snapshot that is gone, e.g. its ref was deleted, cannot
		// be resumed; the next round starts afresh.
		if err == nil && len(since.Files) == 0 {
			return nil
		}
		if err == nil {
			prev, next := roundFiles(files, reviewFiles(since))
			r.state.Relocate(prev, next)
		}
	}
	for _, f := range files {
		r.state.ResetChanged(f.Path, f.NewContent())
	}
	ref := r.state.SnapshotRef(n + 1)
	if err := r.repo.snapshot(r.subj, ref); err != nil {
		return err
	}
	r.state.StartRound(ref, time.Now())
	return nil
}

// snapshot retakes the snapshot of the current round.
func (r rounds) snapshot() error {
	return r.repo.snapshot(r.subj, r.state.Snapshot(r.state.Round()))
}

// interdiff resolves the review files changed since the snapshot of the
// previous round; nil when the review is in its first round.
func (r rounds) interdiff() func() ([]diff.ReviewFile, error) {
	prev := r.state.Snapshot(r.state.Round() - 1)
	if prev == "" {
		return nil
	}
	return func() ([]diff.ReviewFile, error) {
		changes, err := r.repo.since(r.subj, prev)
		if err != nil {
			return nil, err
		}
		return reviewFiles(changes), nil
	}
}

// roundFiles pairs the review files as they were at the snapshot (prev)
// with the review files now (next), for the relocation of the anchors
// made at the snapshot. since are the files changed since the snapshot:
// their old content is the snapshot's. The old side is the base in
// both, so deleted-line anchors keep their place.
func roundFiles(files, since []diff.ReviewFile) (prev, next []review.File) {
	changed := make(map[string]diff.ReviewFile, len(since))
	for _, f := range since {
		changed[f.Path] = f
	}
	for _, f := range files {
		old, cur := f.Contents()
		n := review.File{Path: f.Path, OldPath: f.OldPath, Old: old, New: cur}
		p := n
		if g, ok := changed[f.Path]; ok {
			p.New, _ = g.Contents()
			if g.OldPath != "" {
				// Renamed since the snapshot: the comments carry
				// the path of the snapshot.
				p.Path, n.OldPath = g.OldPath, g.OldPath
			}
		}
		prev = append(prev, p)
		next = append(next, n)
	}
	return prev, next
}

// snapshot records the new side of the review subject under ref: the
// working tree for the default subject, the commit otherwise.
func (l *lockedSource) snapshot(subj subject, ref string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch subj.kind {
	case subjectRange:
		return l.src.SnapshotRev(ref, subj.b)
	case subjectCommit:
		return l.src.SnapshotRev(ref, subj.rev)
	}
	return l.src.Snapshot(ref)
}

// since resolves the changes of the new side of the review subject
// since the snapshot ref.
func (l *lockedSource) since(subj subject, ref string) (source.Changes, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch subj.kind {
	case subjectRange:
		return l.src.Range(ref, subj.b)
	case subjectCommit:
		return l.src.Range(ref, subj.rev)
	}
	return l.src.Since(ref)
}
