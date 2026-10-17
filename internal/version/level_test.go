package version

import (
	"testing"

	"github.com/faridlamaul/shiptag/internal/commits"
)

var defaultRules = Rules{Types: map[string]Level{
	"feat": Minor, "fix": Patch, "perf": Patch, "revert": Patch,
}}

func TestParseLevel(t *testing.T) {
	for s, want := range map[string]Level{"none": None, "patch": Patch, "minor": Minor, "major": Major} {
		got, err := ParseLevel(s)
		if err != nil || got != want || got.String() != s {
			t.Errorf("ParseLevel(%q) = %v, %v", s, got, err)
		}
	}
	if _, err := ParseLevel("huge"); err == nil {
		t.Error("expected error")
	}
}

func TestDetermine(t *testing.T) {
	c := func(typ string, breaking bool) commits.Commit {
		return commits.Commit{SHA: typ, Type: typ, Breaking: breaking}
	}
	lvl, reasons := Determine([]commits.Commit{c("docs", false), c("chore", false)}, defaultRules)
	if lvl != None || len(reasons) != 0 {
		t.Errorf("got %v %v", lvl, reasons)
	}
	lvl, reasons = Determine([]commits.Commit{c("fix", false), c("feat", false)}, defaultRules)
	if lvl != Minor || len(reasons) != 1 || reasons[0].Commit.Type != "feat" {
		t.Errorf("got %v %v", lvl, reasons)
	}
	lvl, _ = Determine([]commits.Commit{c("chore", true)}, defaultRules)
	if lvl != Major {
		t.Errorf("breaking: got %v", lvl)
	}
	lvl, _ = Determine([]commits.Commit{c("docs", false)}, Rules{Types: map[string]Level{"docs": Patch}})
	if lvl != Patch {
		t.Errorf("docs rule: got %v", lvl)
	}
}
