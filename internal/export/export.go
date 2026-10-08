// Package export turns the open comments of a review into a finished
// prompt for the agent and copies it to the clipboard. The prompt is
// rendered from a text/template: a project-local template in .lgtm/
// wins over a global one in the config directory, which wins over the
// built-in default. The clipboard is written via OSC 52, with
// wl-copy/xclip/pbcopy as fallback.
package export

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"text/template"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
)

// categoryPriority orders the categories for the prompt: must first, so
// the agent fixes the important points before the rest.
var categoryPriority = map[review.Category]int{
	review.Must:     0,
	review.Should:   1,
	review.Nit:      2,
	review.Question: 3,
}

// Data is the template data of a prompt.
type Data struct {
	// Branch is the branch under review.
	Branch string
	// Base is the base the review started from.
	Base string
	// Round is the review round; 0 until rounds are tracked.
	Round int
	// Comments are the open comments. The prompt contract sorts them
	// by file, with the must comments first in each file.
	Comments []review.Comment
}

// FileGroup groups the comments of one file.
type FileGroup struct {
	Path     string
	Comments []review.Comment
}

// OpenComments returns the open comments of the state in the order the
// prompt data promises: by file, with the must comments first in each
// file. Custom templates that iterate .Comments directly can rely on it.
func OpenComments(s *review.State) []review.Comment {
	comments := s.OpenComments()
	sortByPathThenPriority(comments)
	return comments
}

// GroupByFile groups the comments by file: the groups in path order,
// within each group the comments by category priority (must first),
// then by position.
func GroupByFile(comments []review.Comment) []FileGroup {
	sorted := append([]review.Comment(nil), comments...)
	sortByPathThenPriority(sorted)
	var groups []FileGroup
	for i := 0; i < len(sorted); {
		j := i + 1
		for j < len(sorted) && sorted[j].Path == sorted[i].Path {
			j++
		}
		groups = append(groups, FileGroup{Path: sorted[i].Path, Comments: sorted[i:j]})
		i = j
	}
	return groups
}

// SortByCategory sorts the comments by category priority (must first),
// then by path and position, stably for equal keys.
func SortByCategory(comments []review.Comment) []review.Comment {
	sorted := append([]review.Comment(nil), comments...)
	slices.SortStableFunc(sorted, func(a, b review.Comment) int {
		return comparePriorityPathPosition(a, b)
	})
	return sorted
}

// sortByPathThenPriority sorts by path, then category priority, then
// position, stably for equal keys. This is the order the prompt data
// promises: grouped by file, must first in each file.
func sortByPathThenPriority(comments []review.Comment) {
	slices.SortStableFunc(comments, func(a, b review.Comment) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return comparePriorityPosition(a, b)
	})
}

// comparePriorityPathPosition orders by category priority, then path,
// then position.
func comparePriorityPathPosition(a, b review.Comment) int {
	if c := comparePriorityPosition(a, b); c != 0 {
		return c
	}
	return strings.Compare(a.Path, b.Path)
}

// comparePriorityPosition orders by category priority, then start, then
// end line. Unknown categories sort after the known ones, so a future
// category cannot jump ahead of must.
func comparePriorityPosition(a, b review.Comment) int {
	pa, pb := priority(a.Category), priority(b.Category)
	if pa != pb {
		return pa - pb
	}
	if a.Start != b.Start {
		return a.Start - b.Start
	}
	return a.End - b.End
}

// priority ranks a category for the prompt; unknown categories go last.
func priority(c review.Category) int {
	if p, ok := categoryPriority[c]; ok {
		return p
	}
	return len(categoryPriority)
}

// Quote wraps code in a backtick fence that no backtick run inside the
// code can close early, like the fence of review.md.
func Quote(code string) string {
	fence := strings.Repeat("`", review.CodeFenceLen(code))
	return fence + "\n" + code + "\n" + fence
}

// DefaultTemplate is the built-in prompt template: a header with
// branch, base and round, then the open comments grouped by file with
// the must comments first. Each comment names its anchor, questions are
// marked as answer-only. Global comments form the first group.
const DefaultTemplate = `# Review {{.Branch}} (Basis: {{.Base}}{{if .Round}}, Runde {{.Round}}{{end}})

Bitte arbeite die folgenden Punkte des Reviews ab:
{{- range groupByFile .Comments}}

## {{if .Path}}{{.Path}}{{else}}global{{end}}
{{- range .Comments}}

### {{.Category}}{{if eq .Category "question"}} – nur beantworten, nichts ändern{{end}} – {{location .}}
{{- if .Quote}}

{{quote .Quote}}
{{- end}}

{{.Text}}
{{- end}}
{{- end}}
`

// funcs are the helpers available in every prompt template.
var funcs = template.FuncMap{
	"groupByFile":    GroupByFile,
	"sortByCategory": SortByCategory,
	"quote":          Quote,
	"location":       func(c review.Comment) string { return c.Location() },
}

// Render executes the template text with the data and returns the
// rendered prompt.
func Render(text string, data Data) (string, error) {
	tmpl, err := template.New("prompt").Funcs(funcs).Parse(text)
	if err != nil {
		return "", fmt.Errorf("prompt template: %w", err)
	}
	var b bytes.Buffer
	if err := tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return b.String(), nil
}
