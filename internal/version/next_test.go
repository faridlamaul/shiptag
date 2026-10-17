package version

import (
	"strings"
	"testing"
)

func TestNext(t *testing.T) {
	sch := Scheme{VPrefix: true}
	tag := func(n string) Tag {
		tg, ok := sch.Parse(n)
		if !ok {
			t.Fatalf("parse %s", n)
		}
		return tg
	}
	tagp := func(n string) *Tag { tg := tag(n); return &tg }
	base := Options{Bump: "auto", InitialVersion: "0.1.0", InitialDevelopment: true, Scheme: sch}
	with := func(f func(o *Options)) Options { o := base; f(&o); return o }

	tests := []struct {
		name     string
		stable   *Tag
		computed Level
		all      []Tag
		opts     Options
		release  bool
		tag      string
		bump     string
		errSub   string
	}{
		{"minor", tagp("v1.2.0"), Minor, nil, base, true, "v1.3.0", "minor", ""},
		{"none", tagp("v1.2.0"), None, nil, base, false, "", "", ""},
		{"0.x major", tagp("v0.4.0"), Major, nil, base, true, "v0.5.0", "minor", ""},
		{"0.x off", tagp("v0.4.0"), Major, nil, with(func(o *Options) { o.InitialDevelopment = false }), true, "v1.0.0", "major", ""},
		{"bump override major", tagp("v0.4.0"), Patch, nil, with(func(o *Options) { o.Bump = "major" }), true, "v1.0.0", "major", ""},
		{"first release", nil, None, nil, base, true, "v0.1.0", "initial", ""},
		{"first release bump", nil, Minor, nil, with(func(o *Options) { o.Bump = "patch" }), true, "v0.0.1", "patch", ""},
		{"explicit", tagp("v1.2.0"), None, nil, with(func(o *Options) { o.Explicit = "2.0.0" }), true, "v2.0.0", "explicit", ""},
		{"explicit lower", tagp("v1.2.0"), None, nil, with(func(o *Options) { o.Explicit = "1.1.0" }), false, "", "", "greater"},
		{"explicit equal", tagp("v1.2.0"), None, nil, with(func(o *Options) { o.Explicit = "v1.2.0" }), false, "", "", "already exists"},
		{"explicit exists", tagp("v1.2.0"), None, []Tag{tag("v3.0.0")}, with(func(o *Options) { o.Explicit = "3.0.0" }), false, "", "", "already exists"},
		{"prerelease numbering", tagp("v1.2.0"), Minor, []Tag{tag("v1.3.0-rc.1"), tag("v1.3.0-rc.2")}, with(func(o *Options) { o.Prerelease = "rc" }), true, "v1.3.0-rc.3", "minor", ""},
		{"prerelease new id", tagp("v1.2.0"), Minor, []Tag{tag("v1.3.0-rc.1")}, with(func(o *Options) { o.Prerelease = "beta" }), true, "v1.3.0-beta.1", "minor", ""},
		{"prerelease none", tagp("v1.2.0"), None, nil, with(func(o *Options) { o.Prerelease = "rc" }), false, "", "", ""},
		{"unknown bump", tagp("v1.2.0"), Minor, nil, with(func(o *Options) { o.Bump = "huge" }), false, "", "", "bump"},
		{"explicit beats prerelease", tagp("v1.2.0"), Minor, nil, with(func(o *Options) { o.Explicit = "2.0.0"; o.Prerelease = "rc" }), true, "v2.0.0", "explicit", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Next(tt.stable, tt.computed, tt.all, tt.opts)
			if tt.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSub) {
					t.Fatalf("err = %v, want containing %q", err, tt.errSub)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Release != tt.release || got.Tag != tt.tag || got.Bump != tt.bump {
				t.Errorf("got %+v, want release=%v tag=%q bump=%q", got, tt.release, tt.tag, tt.bump)
			}
			if got.Release && got.Version == nil {
				t.Error("nil Version on release")
			}
		})
	}
}
