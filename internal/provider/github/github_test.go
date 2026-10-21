package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faridlamaul/shiptag/internal/provider"
)

func newP(t *testing.T, h http.HandlerFunc) provider.Provider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(provider.Config{APIURL: srv.URL, WebURL: "https://github.com", Repo: "o/r", Token: "tok", UserAgent: "shiptag-test"})
}

func TestLinker(t *testing.T) {
	l := NewLinker("https://gh.example/", "o/r")
	if got := l.CommitURL("abc"); got != "https://gh.example/o/r/commit/abc" {
		t.Errorf("commit %q", got)
	}
	if got := l.CompareURL("v1", "v2"); got != "https://gh.example/o/r/compare/v1...v2" {
		t.Errorf("compare %q", got)
	}
	if got := l.ChangeRequestURL(5); got != "https://gh.example/o/r/pull/5" {
		t.Errorf("cr %q", got)
	}
	if got := l.IssueURL(5); got != "https://gh.example/o/r/issues/5" {
		t.Errorf("issue %q", got)
	}
	n, h, ok := l.ExtractChangeRequest("feat: x (#12)", "")
	if !ok || n != 12 || h != "feat: x" {
		t.Errorf("extract %d %q %v", n, h, ok)
	}
	if _, _, ok := l.ExtractChangeRequest("feat: x", ""); ok {
		t.Error("unexpected match")
	}
	n, h, ok = l.ExtractChangeRequest("Merge pull request #12 from o/branch", "feat: x")
	if !ok || n != 12 || h != "Merge pull request #12 from o/branch" {
		t.Errorf("merge %d %q %v", n, h, ok)
	}
	if got := l.ChangeRequestRef(12); got != "#12" {
		t.Errorf("ref %q", got)
	}
	for _, c := range []struct{ name, email, want string }{
		{"A", "123+alice@users.noreply.github.com", "@alice"},
		{"A", "alice@users.noreply.github.com", "@alice"},
		{"A", "Alice@Users.Noreply.GitHub.com", "@Alice"},
		{"A", "a@x.io", "A"},
	} {
		if got := l.Mention(c.name, c.email); got != c.want {
			t.Errorf("mention %q = %q want %q", c.email, got, c.want)
		}
	}
}

func TestTagSHA(t *testing.T) {
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/repos/o/r/git/ref/tags/light":
			w.Write([]byte(`{"object":{"type":"commit","sha":"c1"}}`))
		case "/repos/o/r/git/ref/tags/ann":
			w.Write([]byte(`{"object":{"type":"tag","sha":"t1"}}`))
		case "/repos/o/r/git/tags/t1":
			w.Write([]byte(`{"object":{"type":"commit","sha":"c2"}}`))
		default:
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	if sha, ok, err := p.TagSHA(ctx, "light"); err != nil || !ok || sha != "c1" {
		t.Errorf("light %q %v %v", sha, ok, err)
	}
	if sha, ok, err := p.TagSHA(ctx, "ann"); err != nil || !ok || sha != "c2" {
		t.Errorf("ann %q %v %v", sha, ok, err)
	}
	if _, ok, err := p.TagSHA(ctx, "none"); err != nil || ok {
		t.Errorf("none %v %v", ok, err)
	}
}

func TestReleaseExists(t *testing.T) {
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/repos/o/r/releases/tags/v1":
			w.Write([]byte(`{"tag_name":"v1"}`))
		case "/repos/o/r/releases":
			w.Write([]byte(`[{"tag_name":"other"}]`))
		default:
			http.NotFound(w, r)
		}
	})
	if ok, err := p.ReleaseExists(context.Background(), "v1"); err != nil || !ok {
		t.Errorf("v1 %v %v", ok, err)
	}
	if ok, err := p.ReleaseExists(context.Background(), "v2"); err != nil || ok {
		t.Errorf("v2 %v %v", ok, err)
	}
}

func TestDraftReleaseExists(t *testing.T) {
	var listed bool
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/repos/o/r/releases":
			listed = r.URL.RawQuery == "per_page=100&page=1"
			w.Write([]byte(`[{"tag_name":"v1","draft":true}]`))
		default:
			http.NotFound(w, r)
		}
	})
	if ok, err := p.ReleaseExists(context.Background(), "v1"); err != nil || !ok {
		t.Errorf("draft %v %v", ok, err)
	}
	if !listed {
		t.Error("list fallback not used")
	}
}

func TestPublish(t *testing.T) {
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/repos/o/r/releases" {
			t.Errorf("req %s %s", r.Method, r.URL.Path)
		}
		for k, v := range map[string]string{
			"Authorization":        "Bearer tok",
			"Accept":               "application/vnd.github+json",
			"X-GitHub-Api-Version": "2022-11-28",
			"User-Agent":           "shiptag-test",
		} {
			if got := r.Header.Get(k); got != v {
				t.Errorf("header %s = %q", k, got)
			}
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		want := map[string]any{"tag_name": "v1", "target_commitish": "sha1", "name": "N", "body": "notes", "draft": true, "prerelease": true}
		for k, v := range want {
			if b[k] != v {
				t.Errorf("body %s = %v", k, b[k])
			}
		}
		w.Write([]byte(`{"html_url":"https://github.com/o/r/releases/tag/v1"}`))
	})
	u, err := p.Publish(context.Background(), provider.Release{Tag: "v1", SHA: "sha1", Name: "N", Notes: "notes", Draft: true, Prerelease: true})
	if err != nil || u != "https://github.com/o/r/releases/tag/v1" {
		t.Errorf("publish %q %v", u, err)
	}
	if p.Name() != "github" {
		t.Error("name")
	}
}

func TestNoAuthHeaderWithoutToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Authorization"]; ok {
			t.Error("auth header sent")
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	p := New(provider.Config{APIURL: srv.URL, Repo: "o/r"})
	p.TagSHA(context.Background(), "v1")
}

func TestGitHubPrefixedTagPath(t *testing.T) {
	var paths []string
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		http.NotFound(w, r)
	})
	p.TagSHA(context.Background(), "api/v1.0.0")
	p.ReleaseExists(context.Background(), "api/v1.0.0")
	if paths[0] != "/repos/o/r/git/ref/tags/api/v1.0.0" {
		t.Errorf("ref path %q", paths[0])
	}
	if paths[1] != "/repos/o/r/releases/tags/api/v1.0.0" {
		t.Errorf("release path %q", paths[1])
	}
}

// fullPage returns a JSON list of 100 releases named prefix0..prefix99.
func fullPage(prefix string) string {
	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf(`{"tag_name":"%s%d"}`, prefix, i)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func TestDraftReleaseOnSecondPage(t *testing.T) {
	var pages []string
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/repos/o/r/releases" {
			http.NotFound(w, r)
			return
		}
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		switch page {
		case "1":
			w.Write([]byte(fullPage("old")))
		case "2":
			w.Write([]byte(`[{"tag_name":"v1","draft":true}]`))
		default:
			t.Errorf("unexpected page %q", page)
			w.Write([]byte(`[]`))
		}
	})
	if ok, err := p.ReleaseExists(context.Background(), "v1"); err != nil || !ok {
		t.Errorf("draft %v %v", ok, err)
	}
	if ok, err := p.ReleaseExists(context.Background(), "v9"); err != nil || ok {
		t.Errorf("missing %v %v", ok, err)
	}
	if got := strings.Join(pages, ","); got != "1,2,1,2" {
		t.Errorf("pages %s", got)
	}
}

func TestDraftScanPageCap(t *testing.T) {
	var n int
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/repos/o/r/releases" {
			http.NotFound(w, r)
			return
		}
		n++
		w.Write([]byte(fullPage("p" + r.URL.Query().Get("page") + "-")))
	})
	if ok, err := p.ReleaseExists(context.Background(), "v1"); err != nil || ok {
		t.Errorf("v1 %v %v", ok, err)
	}
	if n != maxReleasePages {
		t.Errorf("listed %d pages, want %d", n, maxReleasePages)
	}
}
