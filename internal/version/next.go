package version

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Options controls Next.
type Options struct {
	Bump               string // "auto" | "major" | "minor" | "patch"
	Explicit           string
	Prerelease         string
	InitialVersion     string
	InitialDevelopment bool
	Scheme             Scheme
}

// Result is the outcome of Next.
type Result struct {
	Release bool // false = nothing to release
	Version *semver.Version
	Tag     string
	Bump    string // level name, "initial", or "explicit"
}

// Next computes the next version. stable is the stable base (nil for a first
// release), computed is Determine's level, and all is every local tag with the
// prefix (used for prerelease numbering).
func Next(stable *Tag, computed Level, all []Tag, o Options) (Result, error) {
	if o.Explicit != "" {
		return nextExplicit(stable, all, o)
	}

	var level Level
	bumpName := ""
	if o.Bump == "" || o.Bump == "auto" {
		level = computed
		if stable == nil {
			return nextInitial(all, o)
		}
		if level == None {
			return Result{}, nil
		}
		if level == Major && o.InitialDevelopment && stable.Version.Major() == 0 {
			level = Minor
		}
		bumpName = level.String()
	} else {
		l, err := ParseLevel(o.Bump)
		if err != nil || l == None {
			return Result{}, fmt.Errorf("invalid bump %q: want auto, major, minor or patch", o.Bump)
		}
		level = l
		bumpName = l.String()
	}

	base := semver.New(0, 0, 0, "", "")
	if stable != nil {
		base = stable.Version
	}
	var core semver.Version
	switch level {
	case Major:
		core = base.IncMajor()
	case Minor:
		core = base.IncMinor()
	default:
		core = base.IncPatch()
	}
	return finish(&core, bumpName, all, o)
}

func nextExplicit(stable *Tag, all []Tag, o Options) (Result, error) {
	v, err := o.Scheme.Normalize(o.Explicit)
	if err != nil {
		return Result{}, err
	}
	for _, t := range all {
		if t.Version.Equal(v) {
			return Result{}, fmt.Errorf("version %s already exists as tag %s", v, t.Name)
		}
	}
	if stable != nil {
		if stable.Version.Equal(v) {
			return Result{}, fmt.Errorf("version %s already exists as tag %s", v, stable.Name)
		}
		if !v.GreaterThan(stable.Version) {
			return Result{}, fmt.Errorf("version %s must be greater than current stable version %s", v, stable.Version)
		}
	}
	return Result{Release: true, Version: v, Tag: o.Scheme.Format(v), Bump: "explicit"}, nil
}

func nextInitial(all []Tag, o Options) (Result, error) {
	init := o.InitialVersion
	if init == "" {
		init = "0.1.0"
	}
	v, err := o.Scheme.Normalize(init)
	if err != nil {
		return Result{}, fmt.Errorf("invalid initial version: %w", err)
	}
	return finish(v, "initial", all, o)
}

// finish applies prerelease numbering (if requested), refuses versions that
// already exist as a tag, and builds the result.
func finish(core *semver.Version, bumpName string, all []Tag, o Options) (Result, error) {
	v := core
	if o.Prerelease != "" {
		n := 1
		prefix := o.Prerelease + "."
		for _, t := range all {
			if t.Version.Major() != core.Major() || t.Version.Minor() != core.Minor() || t.Version.Patch() != core.Patch() {
				continue
			}
			num, ok := strings.CutPrefix(t.Version.Prerelease(), prefix)
			if !ok {
				continue
			}
			k, err := strconv.Atoi(num)
			if err != nil || k < 0 || strconv.Itoa(k) != num {
				continue
			}
			if k+1 > n {
				n = k + 1
			}
		}
		pv, err := semver.NewVersion(fmt.Sprintf("%d.%d.%d-%s.%d", core.Major(), core.Minor(), core.Patch(), o.Prerelease, n))
		if err != nil {
			return Result{}, fmt.Errorf("invalid prerelease identifier %q: %w", o.Prerelease, err)
		}
		v = pv
	}
	// The stable base is the highest reachable stable tag, so a collision
	// here is a tag on another branch; publishing it would fail later.
	for _, t := range all {
		if t.Version.Equal(v) {
			return Result{}, fmt.Errorf("tag %s already exists (not reachable from the target commit)", t.Name)
		}
	}
	return Result{Release: true, Version: v, Tag: o.Scheme.Format(v), Bump: bumpName}, nil
}
