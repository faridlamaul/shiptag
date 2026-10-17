package notes

import (
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/faridlamaul/shiptag/internal/commits"
)

var update = flag.Bool("update", false, "update golden files")

type fakeLinker struct{}

func (fakeLinker) CommitURL(sha string) string { return "https://h/r/commit/" + sha }
func (fakeLinker) CompareURL(from, to string) string {
	return "https://h/r/compare/" + from + "..." + to
}
func (fakeLinker) ChangeRequestURL(n int) string                        { return "https://h/r/pull/" + itoa(n) }
func (fakeLinker) IssueURL(n int) string                                { return "https://h/r/issues/" + itoa(n) }
func (fakeLinker) ExtractChangeRequest(h, b string) (int, string, bool) { return 0, h, false }
func (fakeLinker) Mention(name, email string) string {
	if strings.HasSuffix(email, "@noreply") {
		return "@" + strings.TrimSuffix(email, "@noreply")
	}
	return name
}

func itoa(n int) string {
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}

func opts() Options {
	return Options{
		Heading:       "v1.3.0",
		PreviousTag:   "v1.2.0",
		CompareTo:     "v1.3.0",
		Date:          time.Date(2026, 10, 17, 23, 0, 0, 0, time.UTC),
		BreakingTitle: "⚠ BREAKING CHANGES",
		Sections: []Section{
			{Title: "Features", Types: []string{"feat"}},
			{Title: "Bug Fixes", Types: []string{"fix"}},
			{Title: "Chores", Types: []string{"chore"}, Hidden: true},
		},
		Other:          Section{Title: "Other Changes", Hidden: true},
		ExcludeAuthors: []string{"[bot]", "dependabot"},
		Linker:         fakeLinker{},
	}
}

func p(n, e string) commits.Person { return commits.Person{Name: n, Email: e} }

func sample() []commits.Commit {
	return []commits.Commit{
		{SHA: "aaaaaaa1111111", Type: "feat", Scope: "api", Subject: "drop v1", Breaking: true, BreakingNote: "v1 endpoints removed\nuse v2", ChangeRequest: 42, Author: p("Zed", "zed@x")},
		{SHA: "bbbbbbb2222222", Type: "refactor", Subject: "rename core", Breaking: true, Author: p("alice", "alice@x")},
		{SHA: "ccccccc3333333", Type: "feat", Scope: "cli", Subject: "add --path flag", Author: p("Bob", "bob@noreply")},
		{SHA: "ddddddd4444444", Type: "feat", Subject: "add --dry-run", Author: p("Bob Again", "BOB@noreply")},
		{SHA: "eeeeeee5555555", Type: "fix", Scope: "cli", Subject: "handle empty tag", ChangeRequest: 40, Issues: []int{38}, Author: p("Carol", "carol@x"), CoAuthors: []commits.Person{p("Dave", "dave@x")}},
		{SHA: "fffffff6666666", Type: "chore", Subject: "tidy", Author: p("Erin", "erin@x")},
		{SHA: "1111111aaaaaaa", Subject: "random message", Author: p("Frank", "frank@x")},
		{SHA: "2222222bbbbbbb", Type: "fix", Subject: "bump dep", Author: p("dependabot[bot]", "d@x")},
	}
}

func TestRenderGolden(t *testing.T) {
	got, counted, hidden := Render(sample(), opts())
	if counted != 6 || hidden != 2 {
		t.Fatalf("counted=%d hidden=%d", counted, hidden)
	}
	const path = "testdata/basic.golden.md"
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderNoPrevious(t *testing.T) {
	o := opts()
	o.Heading, o.PreviousTag = "v0.1.0", ""
	got, _, _ := Render(sample(), o)
	if !strings.HasPrefix(got, "## v0.1.0 (2026-10-17)\n\n") {
		t.Fatalf("got %q", got)
	}
}

func TestRenderAllHidden(t *testing.T) {
	cs := []commits.Commit{
		{SHA: "aaaaaaa1", Type: "chore", Subject: "a", Author: p("A", "a@x")},
		{SHA: "bbbbbbb2", Type: "chore", Subject: "b", Author: p("B", "b@x")},
	}
	got, counted, hidden := Render(cs, opts())
	if !strings.Contains(got, "_No user-facing changes._") || counted != 0 || hidden != 2 {
		t.Fatalf("%q %d %d", got, counted, hidden)
	}
	if !strings.Contains(got, "### Contributors\n\nA, B\n") {
		t.Fatalf("contributors missing: %q", got)
	}
}

func TestRenderCounts(t *testing.T) {
	cs := sample()
	_, c, h := Render(cs, opts())
	if c+h != len(cs) {
		t.Fatalf("%d+%d != %d", c, h, len(cs))
	}
}

func TestRenderEscapesSubjects(t *testing.T) {
	cs := []commits.Commit{
		{SHA: "aaaaaaa1", Type: "feat", Subject: "use [x](y) and <b>", Author: p("A", "a@x")},
		{SHA: "bbbbbbb2", Type: "feat", Subject: "#1 fan", Author: p("A", "a@x")},
	}
	got, _, _ := Render(cs, opts())
	for _, w := range []string{`use \[x\](y) and &lt;b&gt;`, `- \#1 fan`} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in %s", w, got)
		}
	}
}

func TestRenderEmaillessContributors(t *testing.T) {
	cs := []commits.Commit{{
		SHA: "aaaaaaa1", Type: "feat", Subject: "x",
		Author:    p("Zoe", "zoe@x"),
		CoAuthors: []commits.Person{p("Ann", ""), p("Ben", ""), p("ann", "")},
	}}
	got, _, _ := Render(cs, opts())
	if !strings.Contains(got, "### Contributors\n\nAnn, Ben, Zoe\n") {
		t.Fatalf("got %q", got)
	}
}
