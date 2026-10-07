package diff

import (
	"sort"

	zdiff "znkr.io/diff"
)

// Status tells how a file changed between the old and the new version.
// The rename detection needs it to tell an added empty file from a
// deleted one.
type Status int

const (
	StatusModified Status = iota
	StatusAdded
	StatusDeleted
)

// File is one changed file of a review subject, the input of the rename
// detection. Old and New are the contents of the two versions, empty
// for added and deleted files. Submodule marks a gitlink change, whose
// contents are commit hashes; OldMode and NewMode carry the file modes
// of a mode-only change.
type File struct {
	Path      string
	Status    Status
	Old       string
	New       string
	OldPath   string // rename source, set by Renames
	Submodule bool
	OldMode   uint32
	NewMode   uint32
}

// renameSimilarityThreshold is the similarity a deleted/added pair must
// reach to count as a rename: git's default.
const renameSimilarityThreshold = 0.5

// Renames detects renames among the files of a review subject: every
// deleted file is paired with the added file it resembles most, as long
// as the similarity reaches 50%. The renamed entry keeps the new path
// and content and carries the old path and content, so the UI can show
// "old → new" with only the content diff. Every file is used at most
// once; ties are broken by path.
func Renames(files []File) []File {
	var deleted, added []int
	for i, f := range files {
		switch f.Status {
		case StatusDeleted:
			deleted = append(deleted, i)
		case StatusAdded:
			added = append(added, i)
		}
	}
	type pair struct {
		deleted, added int
		sim            float64
	}
	var pairs []pair
	for _, d := range deleted {
		for _, a := range added {
			sim := similarity(files[d].Old, files[a].New)
			if sim >= renameSimilarityThreshold {
				pairs = append(pairs, pair{deleted: d, added: a, sim: sim})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		a, b := pairs[i], pairs[j]
		if a.sim != b.sim {
			return a.sim > b.sim
		}
		if files[a.deleted].Path != files[b.deleted].Path {
			return files[a.deleted].Path < files[b.deleted].Path
		}
		return files[a.added].Path < files[b.added].Path
	})

	used := make([]bool, len(files))
	out := make([]File, 0, len(files))
	for _, p := range pairs {
		if used[p.deleted] || used[p.added] {
			continue
		}
		used[p.deleted], used[p.added] = true, true
		d, a := files[p.deleted], files[p.added]
		out = append(out, File{
			Path:      a.Path,
			Status:    StatusModified,
			Old:       d.Old,
			New:       a.New,
			OldPath:   d.Path,
			Submodule: d.Submodule,
			OldMode:   d.OldMode,
			NewMode:   a.NewMode,
		})
	}
	for i, f := range files {
		if !used[i] {
			out = append(out, f)
		}
	}
	return out
}

// similarity is git's similarity index of two contents: twice the
// number of common lines divided by the total number of lines, 1 for
// identical contents. Two empty contents have no lines to compare and
// score 0.
func similarity(old, new string) float64 {
	ol, nl := splitLines(old), splitLines(new)
	if len(ol)+len(nl) == 0 {
		return 0
	}
	common := 0
	for _, e := range zdiff.Edits(ol, nl) {
		if e.Op == zdiff.Match {
			common++
		}
	}
	return 2 * float64(common) / float64(len(ol)+len(nl))
}
