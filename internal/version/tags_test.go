package version

import (
	"testing"

	"github.com/Masterminds/semver/v3"
)

func TestSchemeParse(t *testing.T) {
	s := Scheme{Prefix: "api/"}
	tests := []struct {
		name string
		ok   bool
		pre  string
	}{
		{"api/v1.2.3", true, ""},
		{"api/1.2.3", true, ""},
		{"v1.2.3", false, ""},
		{"api/v1.2", false, ""},
		{"api/v01.2.3", false, ""},
		{"api/v1.2.3-rc.1+b5", true, "rc.1"},
	}
	for _, tt := range tests {
		tag, ok := s.Parse(tt.name)
		if ok != tt.ok {
			t.Errorf("Parse(%q) ok = %v, want %v", tt.name, ok, tt.ok)
			continue
		}
		if ok {
			if tag.Name != tt.name {
				t.Errorf("Parse(%q) Name = %q", tt.name, tag.Name)
			}
			if tag.Version.Prerelease() != tt.pre {
				t.Errorf("Parse(%q) prerelease = %q, want %q", tt.name, tag.Version.Prerelease(), tt.pre)
			}
		}
	}
}

func TestSchemeFormat(t *testing.T) {
	v := semver.MustParse("1.3.0")
	if got := (Scheme{Prefix: "api/", VPrefix: true}).Format(v); got != "api/v1.3.0" {
		t.Errorf("got %q", got)
	}
	if got := (Scheme{}).Format(v); got != "1.3.0" {
		t.Errorf("got %q", got)
	}
}

func TestNormalize(t *testing.T) {
	s := Scheme{Prefix: "api/"}
	for _, in := range []string{"v1.2.3", "1.2.3", "api/v1.2.3", "api/1.2.3"} {
		v, err := s.Normalize(in)
		if err != nil || v.String() != "1.2.3" {
			t.Errorf("Normalize(%q) = %v, %v", in, v, err)
		}
	}
	if _, err := s.Normalize("banana"); err == nil {
		t.Error("expected error for banana")
	}
}

func TestBases(t *testing.T) {
	s := Scheme{VPrefix: true}
	var tags []Tag
	for _, n := range []string{"v1.2.0", "v1.3.0-rc.2", "v1.10.0-beta.1", "v1.9.0"} {
		tag, ok := s.Parse(n)
		if !ok {
			t.Fatalf("parse %s", n)
		}
		tags = append(tags, tag)
	}
	stable, latest := Bases(tags)
	if stable == nil || stable.Name != "v1.9.0" {
		t.Errorf("stable = %v", stable)
	}
	if latest == nil || latest.Name != "v1.10.0-beta.1" {
		t.Errorf("latest = %v", latest)
	}
	if s, l := Bases(nil); s != nil || l != nil {
		t.Error("expected nil, nil")
	}
}
