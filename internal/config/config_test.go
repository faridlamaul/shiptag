package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if !c.VPrefix || c.InitialVersion != "0.1.0" || !c.InitialDevelopment {
		t.Errorf("unexpected scalar defaults: %+v", c)
	}
	wantRules := map[string]string{"feat": "minor", "fix": "patch", "perf": "patch", "revert": "patch"}
	if !reflect.DeepEqual(c.ReleaseRules, wantRules) {
		t.Errorf("ReleaseRules = %v", c.ReleaseRules)
	}
	if c.BreakingTitle != "⚠ BREAKING CHANGES" {
		t.Errorf("BreakingTitle = %q", c.BreakingTitle)
	}
	if !reflect.DeepEqual(c.ExcludeAuthors, []string{"[bot]", "dependabot", "renovate"}) {
		t.Errorf("ExcludeAuthors = %v", c.ExcludeAuthors)
	}
	if want := (Section{Title: "Other Changes", Hidden: true}); !reflect.DeepEqual(c.OtherSection, want) {
		t.Errorf("OtherSection = %+v", c.OtherSection)
	}
	want := []Section{
		{"Features", []string{"feat"}, false},
		{"Bug Fixes", []string{"fix"}, false},
		{"Performance Improvements", []string{"perf"}, false},
		{"Reverts", []string{"revert"}, false},
		{"Code Refactoring", []string{"refactor"}, false},
		{"Documentation", []string{"docs"}, false},
		{"Build System", []string{"build"}, true},
		{"Continuous Integration", []string{"ci"}, true},
		{"Chores", []string{"chore"}, true},
		{"Styles", []string{"style"}, true},
		{"Tests", []string{"test"}, true},
	}
	if !reflect.DeepEqual(c.Sections, want) {
		t.Errorf("Sections = %+v", c.Sections)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("default invalid: %v", err)
	}
}

func write(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".shiptag.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p, dir
}

func TestLoadMissingDefaultFile(t *testing.T) {
	c, err := Load("", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, Default()) {
		t.Errorf("got %+v", c)
	}
}

func TestLoadDefaultFileFromTopLevel(t *testing.T) {
	_, dir := write(t, "tag_prefix: app-\n")
	c, err := Load("", dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.TagPrefix != "app-" {
		t.Errorf("TagPrefix = %q", c.TagPrefix)
	}
}

func TestLoadMergesReleaseRulesReplacesSections(t *testing.T) {
	p, dir := write(t, "release_rules: {docs: patch}\nsections:\n  - {title: Feat, types: [feat]}\n")
	c, err := Load(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReleaseRules["docs"] != "patch" || c.ReleaseRules["feat"] != "minor" {
		t.Errorf("rules = %v", c.ReleaseRules)
	}
	if len(c.Sections) != 1 {
		t.Errorf("sections = %+v", c.Sections)
	}
	if !c.VPrefix || !c.InitialDevelopment {
		t.Errorf("omitted booleans lost defaults: %+v", c)
	}
}

func TestLoadExplicitMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load("nope.yaml", dir); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	p, dir := write(t, "tag_prefx: x\n")
	_, err := Load(p, dir)
	if err == nil || !strings.Contains(err.Error(), "tag_prefx") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name, field string
		mut         func(*Config)
	}{
		{"rule", "release_rules", func(c *Config) { c.ReleaseRules["feat"] = "huge" }},
		{"version", "initial_version", func(c *Config) { c.InitialVersion = "one" }},
		{"section", "title", func(c *Config) { c.Sections[0].Title = "" }},
		{"prefix", "tag_prefix", func(c *Config) { c.TagPrefix = "a b" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default()
			tt.mut(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("err = %v, want mention of %q", err, tt.field)
			}
		})
	}
}

func TestLoadValidates(t *testing.T) {
	p, dir := write(t, "initial_version: one\n")
	if _, err := Load(p, dir); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadAcceptsNoneRule(t *testing.T) {
	p, dir := write(t, "release_rules: {revert: none}\n")
	c, err := Load(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReleaseRules["revert"] != "none" {
		t.Errorf("revert = %q", c.ReleaseRules["revert"])
	}
}
