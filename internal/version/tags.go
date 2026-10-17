package version

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var strictVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$`)

// Scheme describes how version tags are named.
type Scheme struct {
	Prefix  string
	VPrefix bool
}

// Tag is a parsed version tag.
type Tag struct {
	Name    string
	Version *semver.Version
}

func parseStrict(s string) (*semver.Version, bool) {
	if !strictVersion.MatchString(s) {
		return nil, false
	}
	v, err := semver.StrictNewVersion(strings.TrimPrefix(s, "v"))
	if err != nil {
		return nil, false
	}
	return v, true
}

// Parse recognizes a version tag: Prefix, optional "v", strict SemVer 2.0.
func (s Scheme) Parse(name string) (Tag, bool) {
	rest, ok := strings.CutPrefix(name, s.Prefix)
	if !ok {
		return Tag{}, false
	}
	v, ok := parseStrict(rest)
	if !ok {
		return Tag{}, false
	}
	return Tag{Name: name, Version: v}, true
}

// Format renders a version as a tag name.
func (s Scheme) Format(v *semver.Version) string {
	p := s.Prefix
	if s.VPrefix {
		p += "v"
	}
	return p + v.String()
}

// Normalize accepts a version with or without Prefix and "v".
func (s Scheme) Normalize(input string) (*semver.Version, error) {
	rest := strings.TrimPrefix(input, s.Prefix)
	v, ok := parseStrict(rest)
	if !ok {
		return nil, fmt.Errorf("invalid version %q", input)
	}
	return v, nil
}

// Bases returns the highest stable tag and the highest tag of any kind.
func Bases(tags []Tag) (stable, latest *Tag) {
	for i := range tags {
		t := &tags[i]
		if latest == nil || t.Version.GreaterThan(latest.Version) {
			latest = t
		}
		if t.Version.Prerelease() == "" && (stable == nil || t.Version.GreaterThan(stable.Version)) {
			stable = t
		}
	}
	return stable, latest
}
