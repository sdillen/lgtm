package diff_test

import (
	"fmt"
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
)

// genGo returns n lines of Go-like code with a few keywords per line, so
// the diff has realistic token content.
func genGo(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "line %d := value + %d // note %d\n", i, i%7, i)
	}
	return sb.String()
}

// changeLines returns a copy of s with the lines at the given indexes
// rewritten, as an agent would touch a few places in a big file.
func changeLines(s string, lines map[int]string) string {
	parts := strings.SplitAfter(s, "\n")
	for i, repl := range lines {
		parts[i] = repl
	}
	return strings.Join(parts, "")
}

// manyChanges returns two versions of an n-line file where every 7th
// line is modified: hundreds of paired lines, each with a token diff.
func manyChanges(n int) (old, new string) {
	var before, after strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&before, "line %d with some content\n", i)
		if i%7 == 0 {
			fmt.Fprintf(&after, "line %d with some changed content\n", i)
		} else {
			fmt.Fprintf(&after, "line %d with some content\n", i)
		}
	}
	return before.String(), after.String()
}

// reindented is s with every 3rd line indented by a tab, a
// whitespace-only change that ignorews hides.
func reindented(s string) string {
	parts := strings.SplitAfter(s, "\n")
	for i := 0; i < len(parts); i += 3 {
		parts[i] = "\t" + parts[i]
	}
	return strings.Join(parts, "")
}

// minified is one very long line of dense code, like a bundled or
// minified asset.
func minified() (old, new string) {
	body := strings.Repeat("return a+b*c-d/e;", 2000)
	old = "function f(){" + body + "}"
	new = "function f(){const x=1;" + body + "}"
	return old, new
}

func BenchmarkDiffNew(b *testing.B) {
	old10k := genGo(10000)
	new10k := changeLines(old10k, map[int]string{
		5:    "line 5 := changed + 1 // note 5\n",
		999:  "line 999 := changed + 2 // note 999\n",
		4999: "line 4999 := changed + 3 // note 4999\n",
		9999: "line 9999 := changed + 4 // note 9999\n",
	})
	old50k := genGo(50000)
	new50k := changeLines(old50k, map[int]string{
		5: "line 5 := changed + 1 // note 5\n", 25000: "line 25000 := changed + 2\n",
		49999: "line 49999 := changed + 3\n",
	})
	minOld, minNew := minified()
	manyOld, manyNew := manyChanges(10000)

	cases := []struct {
		name     string
		old, new string
	}{
		{name: "10kLines", old: old10k, new: new10k},
		{name: "50kLines", old: old50k, new: new50k},
		{name: "10kLinesManyChanges", old: manyOld, new: manyNew},
		{name: "Minified", old: minOld, new: minNew},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				diff.New("bench.go", c.old, c.new)
			}
		})
	}
}

func BenchmarkWithIgnoreWS(b *testing.B) {
	old := genGo(10000)
	f := diff.New("bench.go", old, reindented(old))
	b.ReportAllocs()
	for b.Loop() {
		f.WithIgnoreWS(true)
	}
}
