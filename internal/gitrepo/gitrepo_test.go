package gitrepo

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
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
	_, err := r.ResolveSHA("nope")
	if err == nil || !strings.Contains(err.Error(), `cannot resolve ref "nope"`) {
		t.Fatalf("unknown ref: err = %v", err)
	}
}

func TestResolveSHAEmptyRepository(t *testing.T) {
	f := gitfixture.New(t)
	r := open(t, f)
	for _, ref := range []string{"HEAD", "main"} {
		if _, err := r.ResolveSHA(ref); err == nil || err.Error() != "repository has no commits" {
			t.Errorf("ResolveSHA(%q) err = %v", ref, err)
		}
	}
}

func TestMerges(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("chore: init", "a")
	f.Tag("v1.0.0", false)
	f.Git("checkout", "-q", "-b", "feature")
	s1 := f.Commit("feat: one", "b")
	s2 := f.Commit("fix: two", "c")
	f.Git("checkout", "-q", "main")
	direct := f.Commit("docs: direct", "d")
	m := f.Merge("feature", "Merge branch 'feature' into 'main'\n\nSee merge request g/p!42")
	r := open(t, f)

	got, err := r.Merges("v1.0.0", "HEAD", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("merges = %+v", got)
	}
	if got[0].SHA != m || got[0].Message != "Merge branch 'feature' into 'main'\n\nSee merge request g/p!42" {
		t.Errorf("merge = %+v", got[0])
	}
	cs := append([]string(nil), got[0].Commits...)
	sort.Strings(cs)
	want := []string{s1, s2}
	sort.Strings(want)
	if !reflect.DeepEqual(cs, want) {
		t.Errorf("commits = %v, want %v (direct %s excluded)", cs, want, direct)
	}

	if got, err := r.Merges("HEAD", "HEAD", ""); err != nil || len(got) != 0 {
		t.Errorf("empty range = %+v, %v", got, err)
	}
	if got, err := r.Merges("", "HEAD", ""); err != nil || len(got) != 1 {
		t.Errorf("full history = %+v, %v", got, err)
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
	S := f.Commit("fix: side", "side/s")
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
		if c.Message == "Merge side" {
			t.Fatal("merge commit included")
		}
	}
	// topo-order reversed: B, then main-line C (first parent), then the side commit.
	if want := []string{B, C, S}; !reflect.DeepEqual(shas, want) {
		t.Fatalf("range = %v, want %v", shas, want)
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
	// git's own explanation is kept so ownership or permission problems are
	// not mistaken for a missing repository.
	if !strings.Contains(err.Error(), "fatal: not a git repository") {
		t.Errorf("err lacks git stderr: %v", err)
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

// wantMergeCommits is what `git rev-list M^@ ^M^1 [^from]` reports: the
// commits a merge brought in, limited to the range.
func wantMergeCommits(f *gitfixture.Repo, from, m string) []string {
	f.T.Helper()
	args := []string{"rev-list", m + "^@", "^" + m + "^1"}
	if from != "" {
		args = append(args, "^"+from)
	}
	out := f.Git(args...)
	if out == "" {
		return nil
	}
	l := strings.Split(out, "\n")
	sort.Strings(l)
	return l
}

// checkMerges asserts Merges(from, to) lists exactly the first-parent merges
// in want, oldest first, each with the commits git itself attributes to it.
func checkMerges(t *testing.T, f *gitfixture.Repo, r *Repo, from, to string, want ...string) {
	t.Helper()
	got, err := r.Merges(from, to, "")
	if err != nil {
		t.Fatal(err)
	}
	var shas []string
	for _, m := range got {
		shas = append(shas, m.SHA)
	}
	if !reflect.DeepEqual(shas, want) {
		t.Fatalf("Merges(%q, %q) = %v, want %v", from, to, shas, want)
	}
	for _, m := range got {
		cs := append([]string(nil), m.Commits...)
		sort.Strings(cs)
		if len(cs) == 0 {
			cs = nil
		}
		if w := wantMergeCommits(f, from, m.SHA); !reflect.DeepEqual(cs, w) {
			t.Errorf("merge %s (%q): commits = %v, want %v", m.SHA[:7], m.Message, cs, w)
		}
	}
}

func TestMergesNested(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("chore: init", "a")
	f.Git("checkout", "-q", "-b", "feature")
	f.Commit("feat: f1", "f1")
	f.Git("checkout", "-q", "-b", "sub")
	f.Commit("feat: s1", "s1")
	f.Commit("feat: s2", "s2")
	f.Git("checkout", "-q", "feature")
	f.Commit("feat: f2", "f2")
	f.Merge("sub", "Merge sub into feature")
	f.Git("checkout", "-q", "main")
	f.Commit("docs: direct", "d")
	m := f.Merge("feature", "Merge feature")
	r := open(t, f)
	checkMerges(t, f, r, "", "HEAD", m)
	got, _ := r.Merges("", "HEAD", "")
	if len(got[0].Commits) != 5 { // f1, f2, s1, s2 and the inner merge
		t.Fatalf("commits = %v", got[0].Commits)
	}
}

func TestMergesOctopus(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("chore: init", "a")
	f.Git("checkout", "-q", "-b", "b1")
	f.Commit("feat: b1", "b1")
	f.Git("checkout", "-q", "main")
	f.Git("checkout", "-q", "-b", "b2")
	f.Commit("feat: b2a", "b2")
	f.Commit("feat: b2b", "b2b")
	f.Git("checkout", "-q", "main")
	f.Commit("docs: direct", "d")
	f.Git("-c", "commit.gpgsign=false", "merge", "-q", "--no-ff", "-m", "Octopus", "b1", "b2")
	m := f.Git("rev-parse", "HEAD")
	r := open(t, f)
	checkMerges(t, f, r, "", "HEAD", m)
	got, _ := r.Merges("", "HEAD", "")
	if len(got[0].Commits) != 3 {
		t.Fatalf("commits = %v", got[0].Commits)
	}
}

func TestMergesBranchMergedTwice(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("chore: init", "a")
	f.Git("checkout", "-q", "-b", "feature")
	f.Commit("feat: one", "b")
	f.Git("checkout", "-q", "main")
	m1 := f.Merge("feature", "Merge feature 1")
	f.Git("checkout", "-q", "feature")
	two := f.Commit("feat: two", "c")
	f.Git("checkout", "-q", "main")
	f.Commit("docs: direct", "d")
	m2 := f.Merge("feature", "Merge feature 2")
	r := open(t, f)
	checkMerges(t, f, r, "", "HEAD", m1, m2)
	got, _ := r.Merges("", "HEAD", "")
	if !reflect.DeepEqual(got[1].Commits, []string{two}) {
		t.Fatalf("second merge commits = %v, want [%s]", got[1].Commits, two)
	}
}

func TestMergesFromRange(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("chore: init", "a")
	f.Git("checkout", "-q", "-b", "feature")
	early := f.Commit("feat: early", "e")
	f.Git("checkout", "-q", "main")
	m1 := f.Merge("feature", "Merge feature early")
	f.Git("checkout", "-q", "-b", "other")
	f.Commit("feat: other", "o")
	f.Git("checkout", "-q", "feature")
	late := f.Commit("feat: late", "l")
	f.Git("checkout", "-q", "main")
	f.Commit("fix: main", "m")
	f.Tag("v1.0.0", false)
	f.Git("checkout", "-q", "feature")
	later := f.Commit("feat: later", "l2")
	f.Git("checkout", "-q", "main")
	m2 := f.Merge("feature", "Merge feature late")
	m3 := f.Merge("other", "Merge other")
	r := open(t, f)
	checkMerges(t, f, r, "v1.0.0", "HEAD", m2, m3)
	checkMerges(t, f, r, "", "HEAD", m1, m2, m3)
	got, _ := r.Merges("v1.0.0", "HEAD", "")
	cs := append([]string(nil), got[0].Commits...)
	sort.Strings(cs)
	want := []string{late, later}
	sort.Strings(want)
	if !reflect.DeepEqual(cs, want) {
		t.Fatalf("commits = %v, want %v (early %s excluded)", cs, want, early)
	}
}

func TestLogPathRelativeToRepoRoot(t *testing.T) {
	f := gitfixture.New(t)
	api := f.Commit("feat: api", "services/api/main.go")
	f.Commit("feat: web", "services/web/main.go")
	f.Commit("feat: root api", "api/x")
	r, err := Open(filepath.Join(f.Dir, "services"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"services/api", "./services/api", "/services/api"} {
		got, err := r.Log("", "HEAD", p)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].SHA != api {
			t.Errorf("Log(path=%q) = %+v, want only %s", p, got, api)
		}
	}
	for _, p := range []string{"", "."} {
		got, err := r.Log("", "HEAD", p)
		if err != nil || len(got) != 3 {
			t.Errorf("Log(path=%q) = %d commits, %v; want 3", p, len(got), err)
		}
	}
}

func TestTagsPrefixNotOption(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("feat: a", "a")
	f.Tag("v1.0.0", false)
	r := open(t, f)
	for _, merged := range []string{"", "HEAD"} {
		got, err := r.Tags("-n9", merged)
		if err != nil || len(got) != 0 {
			t.Errorf("Tags(%q, %q) = %v, %v; want no tags, no error", "-n9", merged, got, err)
		}
		got, err = r.Tags("v", merged)
		if err != nil || !reflect.DeepEqual(got, []string{"v1.0.0"}) {
			t.Errorf("Tags(v, %q) = %v, %v", merged, got, err)
		}
	}
}

// A from that is not on the first-parent chain (a tag cut on a branch that is
// merged later) bounds the merge's commits like it bounds Log.
func TestMergesFromOffFirstParent(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("chore: init", "a")
	f.Git("checkout", "-q", "-b", "release")
	f.Commit("fix: tagged", "t")
	f.Tag("v1.0.0", false)
	after := f.Commit("fix: after tag", "u")
	f.Git("checkout", "-q", "main")
	f.Commit("docs: direct", "d")
	m := f.Merge("release", "Merge release")
	r := open(t, f)
	checkMerges(t, f, r, "v1.0.0", "HEAD", m)
	got, _ := r.Merges("v1.0.0", "HEAD", "")
	if !reflect.DeepEqual(got[0].Commits, []string{after}) {
		t.Fatalf("commits = %v, want [%s]", got[0].Commits, after)
	}
}
