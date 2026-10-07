package review

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
)

// markdown renders the open comments as review.md for the agent: grouped
// by file in path order, each comment with its location, category,
// quoted code and text, in line order.
func (s *State) markdown() string {
	open := s.OpenComments()
	// The stable sort keeps comments on the same lines in the order
	// they were added.
	slices.SortStableFunc(open, func(a, b Comment) int {
		return cmp.Or(
			strings.Compare(a.Path, b.Path),
			cmp.Compare(a.Start, b.Start),
			cmp.Compare(a.End, b.End),
		)
	})

	var b strings.Builder
	b.WriteString("# Review\n")
	if len(open) == 0 {
		b.WriteString("\nNo open comments.\n")
		return b.String()
	}
	for i, c := range open {
		if i == 0 || c.Path != open[i-1].Path {
			fmt.Fprintf(&b, "\n## %s\n", c.Path)
		}
		fmt.Fprintf(&b, "\n### %s:%s (%s)\n\n", c.Path, lineRange(c.Start, c.End), c.Category)
		fence := codeFence(c.Quote)
		fmt.Fprintf(&b, "%s%s\n%s\n%s\n\n", fence, languageTag(c.Path), c.Quote, fence)
		b.WriteString(strings.TrimRight(c.Text, "\n"))
		b.WriteByte('\n')
	}
	return b.String()
}

// lineRange formats a line range as "start-end", a single line as
// "start".
func lineRange(start, end int) string {
	if start == end {
		return fmt.Sprint(start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

// codeFence returns a backtick fence longer than any backtick run in the
// quote, so the quoted code cannot close the block early.
func codeFence(quote string) string {
	return strings.Repeat("`", CodeFenceLen(quote))
}

// CodeFenceLen is the length of a backtick fence that no backtick run
// in the quote can close early; the markdown outputs of review.md and
// the prompt share this format.
func CodeFenceLen(quote string) int {
	longest, run := 0, 0
	for _, r := range quote {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return max(3, longest+1)
}

// languageTag names the language of a file for the code block, "" if it
// is unknown.
func languageTag(path string) string {
	l := lexers.Match(path)
	if l == nil {
		return ""
	}
	if aliases := l.Config().Aliases; len(aliases) > 0 {
		return aliases[0]
	}
	return strings.ToLower(l.Config().Name)
}
