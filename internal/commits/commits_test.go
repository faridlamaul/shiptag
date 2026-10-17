package commits

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cr42 := func(header, body string) (int, string, bool) { return 42, "feat: x", true }
	tests := []struct {
		name    string
		msg     string
		extract CRExtractor
		check   func(t *testing.T, c Commit)
	}{
		{"scoped feat", "feat(cli): add --path flag\n\nbody", nil, func(t *testing.T, c Commit) {
			eq(t, "Type", c.Type, "feat")
			eq(t, "Scope", c.Scope, "cli")
			eq(t, "Subject", c.Subject, "add --path flag")
			eq(t, "Breaking", c.Breaking, false)
		}},
		{"bang", "fix!: drop v1 api", nil, func(t *testing.T, c Commit) {
			eq(t, "Type", c.Type, "fix")
			eq(t, "Breaking", c.Breaking, true)
			eq(t, "BreakingNote", c.BreakingNote, "")
		}},
		{"multiline breaking footer", "feat: x\n\nBREAKING CHANGE: config moved\nto new file\n\nRefs: #1", nil, func(t *testing.T, c Commit) {
			eq(t, "Breaking", c.Breaking, true)
			eq(t, "BreakingNote", c.BreakingNote, "config moved\nto new file")
		}},
		{"hyphen breaking footer", "refactor: y\n\nBREAKING-CHANGE: z", nil, func(t *testing.T, c Commit) {
			eq(t, "Breaking", c.Breaking, true)
			eq(t, "BreakingNote", c.BreakingNote, "z")
		}},
		{"revert", "Revert \"feat: x\"\n\nThis reverts commit abc.", nil, func(t *testing.T, c Commit) {
			eq(t, "Type", c.Type, "revert")
			eq(t, "Subject", c.Subject, "feat: x")
		}},
		{"other", "Update readme", nil, func(t *testing.T, c Commit) {
			eq(t, "Type", c.Type, "other")
			eq(t, "Subject", c.Subject, "Update readme")
		}},
		{"mixed case type", "Feat-Ure(a b): s", nil, func(t *testing.T, c Commit) {
			eq(t, "Type", c.Type, "feat-ure")
			eq(t, "Scope", c.Scope, "a b")
		}},
		{"issues", "fix: a\n\nCloses #12, fixes #13\nresolves #12\ncloses other/repo#9", nil, func(t *testing.T, c Commit) {
			eq(t, "Issues", c.Issues, []int{12, 13})
		}},
		{"co-authors", "fix: a\n\nCo-authored-by: Bob <bob@x.io>\nco-authored-by: Ann <ann@x.io>", nil, func(t *testing.T, c Commit) {
			eq(t, "CoAuthors", c.CoAuthors, []Person{{"Bob", "bob@x.io"}, {"Ann", "ann@x.io"}})
		}},
		{"change request", "feat: x (#42)", cr42, func(t *testing.T, c Commit) {
			eq(t, "ChangeRequest", c.ChangeRequest, 42)
			eq(t, "Subject", c.Subject, "x")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, Parse(Raw{SHA: "s", AuthorName: "N", AuthorEmail: "n@x.io", Message: tt.msg}, tt.extract))
		})
	}
}

func TestParseHeaderFromBody(t *testing.T) {
	// An extractor that promotes the first body line to the header, as the
	// Bitbucket linker does for "Merged in <branch> (pull request #N)".
	promote := func(header, body string) (int, string, bool) {
		if strings.HasPrefix(header, "Merged in ") {
			return 8, FirstLine(body), true
		}
		return 0, header, false
	}
	c := Parse(Raw{Message: "Merged in feature/x (pull request #8)\n\nfeat(api)!: add y\n\ncloses #3"}, promote)
	eq(t, "Type", c.Type, "feat")
	eq(t, "Scope", c.Scope, "api")
	eq(t, "Subject", c.Subject, "add y")
	eq(t, "Breaking", c.Breaking, true)
	eq(t, "ChangeRequest", c.ChangeRequest, 8)
	eq(t, "Issues", c.Issues, []int{3})

	// The promoted line is removed from the body, so it is not parsed twice.
	c = Parse(Raw{Message: "Merged in x (pull request #8)\n\nBREAKING CHANGE: promoted"}, promote)
	eq(t, "Type", c.Type, "other")
	eq(t, "Breaking", c.Breaking, false)

	eq(t, "FirstLine", FirstLine("\n  \n  a b \nc"), "a b")
	eq(t, "FirstLine empty", FirstLine("\n\n"), "")
}

func TestParseAuthor(t *testing.T) {
	c := Parse(Raw{SHA: "abc", AuthorName: "N", AuthorEmail: "n@x.io", Message: "fix: a"}, nil)
	eq(t, "SHA", c.SHA, "abc")
	eq(t, "Author", c.Author, Person{"N", "n@x.io"})
}

func TestParseMessyMessages(t *testing.T) {
	c := Parse(Raw{Message: "fix: a\r\n\r\nBREAKING CHANGE: b\r\n"}, nil)
	eq(t, "Subject", c.Subject, "a")
	eq(t, "BreakingNote", c.BreakingNote, "b")

	eq(t, "spaced", Parse(Raw{Message: "feat:  spaced  "}, nil).Subject, "spaced")
	eq(t, "unicode", Parse(Raw{Message: "feat: ünïcode ✓"}, nil).Subject, "ünïcode ✓")

	c = Parse(Raw{Message: ""}, nil)
	eq(t, "Type", c.Type, "other")
	eq(t, "Subject", c.Subject, "")
}

func eq(t *testing.T, name string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", name, got, want)
	}
}
