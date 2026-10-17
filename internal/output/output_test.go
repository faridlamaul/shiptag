package output

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDestination(t *testing.T) {
	env := func(k string) string {
		if k == "GITHUB_OUTPUT" {
			return "/tmp/o"
		}
		return ""
	}
	empty := func(string) string { return "" }
	tests := []struct {
		name     string
		provider string
		inCI     bool
		override string
		getenv   func(string) string
		want     string
	}{
		{"github ci", "github", true, "", env, "/tmp/o"},
		{"github ci no env", "github", true, "", empty, ""},
		{"gitlab ci", "gitlab", true, "", env, "shiptag.env"},
		{"bitbucket ci", "bitbucket", true, "", env, "shiptag.env"},
		{"not ci", "github", false, "", env, ""},
		{"override wins", "github", true, "x.env", env, "x.env"},
		{"override wins outside ci", "gitlab", false, "x.env", env, "x.env"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Destination(tt.provider, tt.inCI, tt.override, tt.getenv); got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestLines(t *testing.T) {
	v := Values{Released: true, Version: "1.2.0", PreviousVersion: "1.1.0", Tag: "v1.2.0", SHA: "abc", Bump: "minor", URL: "u"}
	want := []string{"released=true", "version=1.2.0", "previous_version=1.1.0", "tag=v1.2.0", "sha=abc", "bump=minor", "url=u"}
	if got := v.Lines(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
	got := Values{Version: "a\r\nb"}.Lines()
	want = []string{"released=false", "version=a  b", "previous_version=", "tag=", "sha=", "bump=", "url="}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestWriteAppends(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(p, []byte("keep=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := Values{Released: true, Version: "1.0.0", Tag: "v1.0.0"}
	if err := Write(p, v, nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	want := "keep=1\nreleased=true\nversion=1.0.0\nprevious_version=\ntag=v1.0.0\nsha=\nbump=\nurl=\n"
	if string(b) != want {
		t.Errorf("got %q", b)
	}
	var buf bytes.Buffer
	if err := Write("", v, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "released=true\nversion=1.0.0\nprevious_version=\ntag=v1.0.0\nsha=\nbump=\nurl=\n" {
		t.Errorf("stdout got %q", buf.String())
	}
}

func TestPrependChangelog(t *testing.T) {
	tests := []struct {
		name     string
		existing *string
		notes    string
		want     string
	}{
		{"missing", nil, "## v2\n- x\n", "# Changelog\n\n## v2\n- x\n"},
		{"missing no trailing newline", nil, "## v2", "# Changelog\n\n## v2\n"},
		{"heading", ptr("# Changelog\n\n## v1\n"), "## v2\n", "# Changelog\n\n## v2\n\n## v1\n"},
		{"heading only", ptr("# Changelog\n"), "## v2\n", "# Changelog\n\n## v2\n"},
		{"crlf", ptr("# Changelog\r\n\r\n## v1\r\n"), "## v2\n- x\n", "# Changelog\r\n\r\n## v2\r\n- x\r\n\r\n## v1\r\n"},
		{"no heading", ptr("## v1\n"), "## v2\n", "## v2\n\n## v1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "CHANGELOG.md")
			if tt.existing != nil {
				if err := os.WriteFile(p, []byte(*tt.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := PrependChangelog(p, tt.notes); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(p)
			if string(b) != tt.want {
				t.Errorf("got %q want %q", b, tt.want)
			}
			if _, err := os.Stat(p + ".tmp"); err == nil {
				t.Error("tmp file left behind")
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestPrependChangelogPreservesMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(p, []byte("# Changelog\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PrependChangelog(p, "## v2\n"); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
}

func TestPrependChangelogReadErrorReturnsEarly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := PrependChangelog(p, "## v2\n"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPrependChangelogCleansTmpOnRenameFailure(t *testing.T) {
	p := filepath.Join(t.TempDir(), "CHANGELOG.md")
	orig := "# Changelog\n\n## v1\n"
	if err := os.WriteFile(p, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	old := rename
	rename = func(string, string) error { return errors.New("boom") }
	t.Cleanup(func() { rename = old })
	if err := PrependChangelog(p, "## v2\n"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(p + ".tmp"); err == nil {
		t.Error("tmp file left behind")
	}
	if b, _ := os.ReadFile(p); string(b) != orig {
		t.Errorf("original changed: %q", b)
	}
}
