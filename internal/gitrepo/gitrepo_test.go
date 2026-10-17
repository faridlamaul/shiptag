package gitrepo

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/faridlamaul/shiptag/internal/testutil/gitfixture"
)

func open(t *testing.T, f *gitfixture.Repo) *Repo {
	t.Helper()
	r, err := Open(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestResolveSHA(t *testing.T) {
	f := gitfixture.New(t)
	sha := f.Commit("feat: a", "a")
	r := open(t, f)
	for _, ref := range []string{"main", "HEAD"} {
		got, err := r.ResolveSHA(ref)
		if err != nil || got != sha || len(got) != 40 {
			t.Fatalf("ResolveSHA(%q) = %q, %v; want %q", ref, got, err, sha)
		}
	}
	if _, err := r.ResolveSHA("nope"); err == nil {
		t.Fatal("expected error for unknown ref")
	}
}

func TestTagsMergedInto(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("feat: a", "a")
	f.Tag("v1.0.0", false)
	f.Tag("api/v2.0.0", true)
	sha := f.Git("rev-parse", "HEAD")
	f.Git("checkout", "-q", "-b", "side")
	f.Commit("feat: b", "b")
	f.Tag("v9.0.0", false)
	f.Git("checkout", "-q", "main")
	r := open(t, f)

	got, err := r.Tags("", sha)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if want := []string{"api/v2.0.0", "v1.0.0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
	all, _ := r.Tags("", "")
	if len(all) != 3 {
		t.Fatalf("all = %v", all)
	}
	api, _ := r.Tags("api/", "")
	if !reflect.DeepEqual(api, []string{"api/v2.0.0"}) {
		t.Fatalf("api = %v", api)
	}
}

func TestLogRangeAndPath(t *testing.T) {
	f := gitfixture.New(t)
	A := f.Commit("feat: a\n\nbody line", "api/x")
	f.Tag("v1", false)
	B := f.Commit("fix: b", "web/y")
	f.Git("checkout", "-q", "-b", "side")
	f.Commit("fix: side", "side/s")
	f.Git("checkout", "-q", "main")
	C := f.Commit("feat: c", "api/z")
	f.Git("-c", "commit.gpgsign=false", "merge", "--no-ff", "-q", "-m", "Merge side", "side")
	r := open(t, f)

	got, err := r.Log("v1", "HEAD", "")
	if err != nil {
		t.Fatal(err)
	}
	var shas []string
	for _, c := range got {
		shas = append(shas, c.SHA)
	}
	if len(got) != 3 || shas[0] != B || (shas[1] != C && shas[2] != C) {
		t.Fatalf("range = %v (B=%s C=%s)", shas, B, C)
	}
	for _, c := range got {
		if c.Message == "Merge side" {
			t.Fatal("merge commit included")
		}
	}

	got, err = r.Log("", "HEAD", "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SHA != A || got[1].SHA != C {
		t.Fatalf("path = %+v", got)
	}
	if got[0].Message != "feat: a\n\nbody line" || got[0].AuthorEmail != "test@example.com" || got[0].AuthorName != "Test" {
		t.Fatalf("fields = %+v", got[0])
	}
}

func TestDetachedHeadAnnotatedTags(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("feat: a", "a")
	f.Tag("v1.0.0", true)
	f.Git("checkout", "-q", "--detach")
	f.Commit("fix: b", "b")
	r := open(t, f)
	tags, err := r.Tags("", "HEAD")
	if err != nil || !reflect.DeepEqual(tags, []string{"v1.0.0"}) {
		t.Fatalf("tags = %v, %v", tags, err)
	}
	cs, err := r.Log("v1.0.0", "HEAD", "")
	if err != nil || len(cs) != 1 {
		t.Fatalf("log = %v, %v", cs, err)
	}
}

func TestLogIgnoresHostileGlobalConfig(t *testing.T) {
	f := gitfixture.New(t)
	sha := f.Commit("feat: hello", "a")
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	body := "[log]\n\tshowSignature = true\n[color]\n\tui = always\n[format]\n\tpretty = oneline\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	r := open(t, f)
	cs, err := r.Log("", "HEAD", "")
	if err != nil || len(cs) != 1 || cs[0].SHA != sha || cs[0].Message != "feat: hello" {
		t.Fatalf("log = %+v, %v", cs, err)
	}
}

func TestIsShallow(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("feat: a", "a")
	f.Commit("feat: b", "b")
	r := open(t, f)
	if s, err := r.IsShallow(); err != nil || s {
		t.Fatalf("shallow = %v, %v", s, err)
	}
	dst := filepath.Join(t.TempDir(), "clone")
	f.Git("clone", "-q", "--depth", "1", "file://"+f.Dir, dst)
	r2, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := r2.IsShallow(); err != nil || !s {
		t.Fatalf("shallow = %v, %v", s, err)
	}
}

func TestOpenNotRepository(t *testing.T) {
	_, err := Open(t.TempDir())
	if !errors.Is(err, ErrNotRepository) {
		t.Fatalf("err = %v", err)
	}
}

func TestRemoteURLAndTopLevel(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("feat: a", "a")
	r := open(t, f)
	if u, err := r.RemoteURL("origin"); err != nil || u != "" {
		t.Fatalf("url = %q, %v", u, err)
	}
	f.Git("remote", "add", "origin", "https://github.com/o/r.git")
	if u, _ := r.RemoteURL("origin"); u != "https://github.com/o/r.git" {
		t.Fatalf("url = %q", u)
	}
	top, err := r.TopLevel()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(f.Dir)
	got, _ := filepath.EvalSymlinks(top)
	if got != want {
		t.Fatalf("top = %q want %q", got, want)
	}
}
