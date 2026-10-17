package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/faridlamaul/shiptag/internal/testutil/gitfixture"
)

var update = flag.Bool("update", false, "update golden files")

// epoch is 2026-10-17T00:00:00Z.
const epoch = "1792195200"

// runCLI invokes run with an injected environment. SOURCE_DATE_EPOCH is
// always set so rendered dates are stable.
func runCLI(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	all := map[string]string{"SOURCE_DATE_EPOCH": epoch}
	for k, v := range env {
		all[k] = v
	}
	var out, errb bytes.Buffer
	code := run(args, func(k string) string { return all[k] }, &out, &errb)
	return code, out.String(), errb.String()
}

// normalize replaces a commit SHA and its short forms with placeholders so
// golden files do not depend on commit timestamps.
func normalize(s string, shas map[string]string) string {
	for name, sha := range shas {
		s = strings.ReplaceAll(s, sha, "<"+name+">")
		s = strings.ReplaceAll(s, sha[:8], "<"+name+"8>")
		s = strings.ReplaceAll(s, sha[:7], "<"+name+"7>")
	}
	return s
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// githubRepo builds v1.2.0 followed by a feature and a hidden chore.
func githubRepo(t *testing.T) (*gitfixture.Repo, string, string) {
	t.Helper()
	f := gitfixture.New(t)
	f.Commit("chore: init", "README.md")
	f.Tag("v1.2.0", true)
	feat := f.Commit("feat(cli): add --path flag (#40)", "cli.go")
	head := f.Commit("chore: x", "x.txt")
	return f, feat, head
}

func TestTagDryRunGitHub(t *testing.T) {
	f, feat, head := githubRepo(t)
	code, out, errOut := runCLI(t, nil, "tag", "--provider", "github", "--repo", "o/r", "--dry-run", "--dir", f.Dir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	golden(t, "dryrun_github.golden", normalize(out, map[string]string{"feat": feat, "head": head}))
}

func TestTagDryRunJSON(t *testing.T) {
	f, _, head := githubRepo(t)
	code, out, errOut := runCLI(t, nil, "tag", "--provider", "github", "--repo", "o/r", "--dry-run", "--json", "--dir", f.Dir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	checks := map[string]any{
		"provider": "github", "repository": "o/r", "ref": "HEAD", "sha": head,
		"previous_tag": "v1.2.0", "next_tag": "v1.3.0", "version": "1.3.0",
		"bump": "minor", "dry_run": true, "released": false,
	}
	for k, want := range checks {
		if p[k] != want {
			t.Errorf("%s = %v, want %v", k, p[k], want)
		}
	}
	reasons, _ := p["reasons"].([]any)
	if len(reasons) != 1 {
		t.Fatalf("reasons = %v", p["reasons"])
	}
	r0 := reasons[0].(map[string]any)
	if r0["type"] != "feat" || r0["scope"] != "cli" || r0["subject"] != "add --path flag" || r0["bump"] != "minor" {
		t.Errorf("reasons[0] = %v", r0)
	}
	if c := p["commits"].(map[string]any); c["counted"] != 1.0 || c["hidden"] != 1.0 {
		t.Errorf("commits = %v", c)
	}
	if _, ok := p["url"]; ok {
		t.Errorf("url present in dry-run plan")
	}
	for _, k := range []string{"actions", "notes"} {
		if _, ok := p[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	if strings.Contains(out, "released=") {
		t.Errorf("stdout carries KEY=VALUE outputs alongside JSON")
	}
}

func TestTagDryRunGitLabEnv(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("chore: init", "a")
	f.Tag("v1.0.0", false)
	sha := f.Commit("feat: y (!7)", "b")
	env := map[string]string{
		"GITLAB_CI": "true", "CI_PROJECT_PATH": "g/p",
		"CI_SERVER_URL": "https://gl.x", "CI_COMMIT_SHA": sha,
	}
	code, out, errOut := runCLI(t, env, "tag", "--dry-run", "--dir", f.Dir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{
		"Provider:     gitlab (g/p)",
		"Ref:          " + sha + " → " + sha[:8] + "…",
		"would create release v1.1.0 at " + sha[:7] + " (tag created by release)",
		"would write outputs to shiptag.env",
		"https://gl.x/g/p/-/merge_requests/7",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q\n%s", want, out)
		}
	}
	if _, err := os.Stat("shiptag.env"); err == nil {
		t.Errorf("dry-run wrote shiptag.env")
	}
}

func TestNothingToRelease(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("feat: a", "a")
	f.Tag("v1.0.0", false)
	f.Commit("chore: b", "b")
	base := []string{"--provider", "github", "--repo", "o/r", "--dir", f.Dir}

	code, out, errOut := runCLI(t, nil, append([]string{"tag"}, base...)...)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(errOut, "nothing to release") {
		t.Errorf("stderr = %q", errOut)
	}
	if !strings.Contains(out, "released=false\n") || !strings.Contains(out, "previous_version=1.0.0\n") {
		t.Errorf("stdout = %q", out)
	}

	code, out, errOut = runCLI(t, nil, append([]string{"next-version"}, base...)...)
	if code != 0 || out != "" {
		t.Errorf("next-version: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestNextVersionPrerelease(t *testing.T) {
	f := gitfixture.New(t)
	f.Commit("feat: a", "a")
	f.Tag("v1.2.0", false)
	f.Commit("feat: rc", "b")
	f.Tag("v1.3.0-rc.1", false)
	f.Commit("feat: z", "c")
	code, out, errOut := runCLI(t, nil, "next-version", "--prerelease", "rc", "--provider", "github", "--repo", "o/r", "--dir", f.Dir)
	if code != 0 || out != "v1.3.0-rc.2\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestChangelogUnreleased(t *testing.T) {
	f, feat, head := githubRepo(t)
	code, out, errOut := runCLI(t, nil, "changelog", "--provider", "github", "--repo", "o/r", "--dir", f.Dir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	want := "## [Unreleased](https://github.com/o/r/compare/v1.2.0..." + head[:7] + ") (2026-10-17)\n"
	if !strings.HasPrefix(out, want) {
		t.Errorf("stdout = %q, want prefix %q", out, want)
	}
	if !strings.Contains(out, feat[:7]) {
		t.Errorf("notes miss feature commit:\n%s", out)
	}

	code, out, _ = runCLI(t, nil, "changelog", "--version", "1.3.0", "--provider", "github", "--repo", "o/r", "--dir", f.Dir)
	if code != 0 || !strings.HasPrefix(out, "## [v1.3.0](https://github.com/o/r/compare/v1.2.0...v1.3.0) (2026-10-17)\n") {
		t.Errorf("--version: exit %d, stdout %q", code, out)
	}
}

// fakeGitHub serves the GitHub endpoints used by publishing.
type fakeGitHub struct {
	mu       sync.Mutex
	tagSHA   string // "" = tag missing
	release  bool
	requests []string
	posts    []map[string]any
}

func (g *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, r.Method+" "+r.URL.Path)
	switch {
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/o/r/git/ref/tags/"):
		if g.tagSHA == "" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"object":{"type":"commit","sha":"`+g.tagSHA+`"}}`)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/o/r/releases/tags/"):
		if !g.release {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{}`)
	case r.Method == "GET" && r.URL.Path == "/repos/o/r/releases":
		io.WriteString(w, `[]`)
	case r.Method == "POST" && r.URL.Path == "/repos/o/r/releases":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		g.posts = append(g.posts, body)
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"html_url":"https://github.com/o/r/releases/tag/v1.3.0"}`)
	default:
		http.Error(w, "unexpected", http.StatusTeapot)
	}
}

func ciEnv(api, outFile, token string) map[string]string {
	return map[string]string{
		"GITHUB_ACTIONS": "true", "GITHUB_REPOSITORY": "o/r",
		"GITHUB_API_URL": api, "GITHUB_OUTPUT": outFile, "GITHUB_TOKEN": token,
	}
}

func TestTagPublishGitHub(t *testing.T) {
	f, _, head := githubRepo(t)
	fake := &fakeGitHub{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	outFile := filepath.Join(t.TempDir(), "github_output")
	t.Chdir(t.TempDir())

	code, out, errOut := runCLI(t, ciEnv(srv.URL, outFile, "tok"), "tag", "--dir", f.Dir, "--changelog-file", "CHANGELOG.md")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty (outputs go to GITHUB_OUTPUT)", out)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"released=true\n", "tag=v1.3.0\n", "version=1.3.0\n", "previous_version=1.2.0\n", "bump=minor\n", "sha=" + head + "\n", "url=https://github.com/o/r/releases/tag/v1.3.0\n"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("outputs missing %q:\n%s", want, data)
		}
	}
	if len(fake.posts) != 1 {
		t.Fatalf("posts = %v", fake.posts)
	}
	p := fake.posts[0]
	if p["tag_name"] != "v1.3.0" || p["target_commitish"] != head || p["name"] != "v1.3.0" || p["draft"] != false || p["prerelease"] != false {
		t.Errorf("release body = %v", p)
	}
	if body, _ := p["body"].(string); !strings.HasPrefix(body, "## [v1.3.0](") {
		t.Errorf("release notes = %q", body)
	}
	cl, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatalf("changelog not created: %v", err)
	}
	if !strings.HasPrefix(string(cl), "# Changelog\n") || !strings.Contains(string(cl), "## [v1.3.0](") {
		t.Errorf("changelog = %q", cl)
	}
}

func TestTagIdempotent(t *testing.T) {
	f, _, head := githubRepo(t)
	outFile := filepath.Join(t.TempDir(), "github_output")

	t.Run("same sha", func(t *testing.T) {
		fake := &fakeGitHub{tagSHA: head, release: true}
		srv := httptest.NewServer(fake)
		defer srv.Close()
		code, _, errOut := runCLI(t, ciEnv(srv.URL, outFile, "tok"), "tag", "--dir", f.Dir)
		if code != 0 || !strings.Contains(errOut, "already released") {
			t.Fatalf("exit %d, stderr %q", code, errOut)
		}
		if len(fake.posts) != 0 {
			t.Errorf("unexpected POST: %v", fake.posts)
		}
		data, _ := os.ReadFile(outFile)
		if !strings.Contains(string(data), "released=false\n") {
			t.Errorf("outputs = %q", data)
		}
	})

	t.Run("different sha", func(t *testing.T) {
		fake := &fakeGitHub{tagSHA: strings.Repeat("d", 40)}
		srv := httptest.NewServer(fake)
		defer srv.Close()
		code, _, errOut := runCLI(t, ciEnv(srv.URL, outFile, "tok"), "tag", "--dir", f.Dir)
		if code != 1 || !strings.Contains(errOut, "already points to") {
			t.Fatalf("exit %d, stderr %q", code, errOut)
		}
		if len(fake.posts) != 0 {
			t.Errorf("unexpected POST: %v", fake.posts)
		}
	})
}

func TestMissingTokenOutsideDryRun(t *testing.T) {
	f, _, _ := githubRepo(t)
	code, out, errOut := runCLI(t, nil, "tag", "--provider", "github", "--repo", "o/r", "--dir", f.Dir)
	if code != 1 || !strings.Contains(errOut, "GITHUB_TOKEN") || !strings.HasPrefix(errOut, "error: ") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestShallowCloneFails(t *testing.T) {
	f, _, _ := githubRepo(t)
	dst := filepath.Join(t.TempDir(), "clone")
	f.Git("clone", "-q", "--depth", "1", "file://"+f.Dir, dst)
	code, _, errOut := runCLI(t, nil, "tag", "--provider", "github", "--repo", "o/r", "--dry-run", "--dir", dst)
	if code != 1 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	for _, want := range []string{"fetch-depth: 0", "GIT_DEPTH: 0", "clone: depth: full"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q: %s", want, errOut)
		}
	}
}

func TestTokenNeverPrinted(t *testing.T) {
	f, _, _ := githubRepo(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		http.Error(w, "boom "+r.Header.Get("Authorization"), http.StatusInternalServerError)
	}))
	defer srv.Close()
	env := map[string]string{"GITHUB_TOKEN": "supersecret"}
	code, out, errOut := runCLI(t, env, "tag", "--provider", "github", "--repo", "o/r", "--api-url", srv.URL, "--dir", f.Dir)
	if code != 1 {
		t.Errorf("exit %d", code)
	}
	if strings.Contains(out+errOut, "supersecret") {
		t.Errorf("token leaked to output")
	}
}

func TestVersionShorthandRemoved(t *testing.T) {
	f, _, _ := githubRepo(t)
	code, _, errOut := runCLI(t, nil, "tag", "-v", "1.5.0", "--provider", "github", "--repo", "o/r", "--dry-run", "--dir", f.Dir)
	if code != 1 || !strings.Contains(errOut, "error: ") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}
