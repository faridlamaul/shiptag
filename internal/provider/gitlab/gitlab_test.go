package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faridlamaul/shiptag/internal/provider"
)

func newP(t *testing.T, kind string, h http.HandlerFunc) provider.Provider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(provider.Config{APIURL: srv.URL, WebURL: "https://gl.example/", Repo: "g/s/p", Token: "tok", TokenKind: kind})
}

func TestLinker(t *testing.T) {
	l := NewLinker("https://gl.example/", "g/s/p")
	if got := l.CommitURL("abc"); got != "https://gl.example/g/s/p/-/commit/abc" {
		t.Errorf("commit %q", got)
	}
	if got := l.CompareURL("v1", "v2"); got != "https://gl.example/g/s/p/-/compare/v1...v2" {
		t.Errorf("compare %q", got)
	}
	if got := l.ChangeRequestURL(5); got != "https://gl.example/g/s/p/-/merge_requests/5" {
		t.Errorf("cr %q", got)
	}
	if got := l.IssueURL(5); got != "https://gl.example/g/s/p/-/issues/5" {
		t.Errorf("issue %q", got)
	}
	n, h, ok := l.ExtractChangeRequest("feat: x (!12)", "")
	if !ok || n != 12 || h != "feat: x" {
		t.Errorf("header %d %q %v", n, h, ok)
	}
	n, h, ok = l.ExtractChangeRequest("Merge branch 'x'", "body\n\nSee merge request g/s/p!34")
	if !ok || n != 34 || h != "Merge branch 'x'" {
		t.Errorf("body %d %q %v", n, h, ok)
	}
	if _, _, ok := l.ExtractChangeRequest("Merge", "See merge request other/p!34"); ok {
		t.Error("foreign repo matched")
	}
	if got := l.Mention("Bob", "bob@x.io"); got != "Bob" {
		t.Errorf("mention %q", got)
	}
}

func TestAuthHeader(t *testing.T) {
	for _, c := range []struct{ kind, hdr string }{{"", "PRIVATE-TOKEN"}, {"job", "JOB-TOKEN"}} {
		p := newP(t, c.kind, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(c.hdr) != "tok" {
				t.Errorf("kind %q missing %s", c.kind, c.hdr)
			}
			if _, ok := r.Header["Authorization"]; ok {
				t.Error("Authorization sent")
			}
			http.NotFound(w, r)
		})
		p.ReleaseExists(context.Background(), "v1")
	}
}

func TestTagSHA(t *testing.T) {
	p := newP(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/projects/g%2Fs%2Fp/repository/tags/v1" {
			w.Write([]byte(`{"commit":{"id":"c1"}}`))
			return
		}
		http.NotFound(w, r)
	})
	if sha, ok, err := p.TagSHA(context.Background(), "v1"); err != nil || !ok || sha != "c1" {
		t.Errorf("v1 %q %v %v", sha, ok, err)
	}
	if _, ok, err := p.TagSHA(context.Background(), "v2"); err != nil || ok {
		t.Errorf("v2 %v %v", ok, err)
	}
}

func TestReleaseExists(t *testing.T) {
	p := newP(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/projects/g%2Fs%2Fp/releases/v1" {
			w.Write([]byte(`{}`))
			return
		}
		http.NotFound(w, r)
	})
	if ok, err := p.ReleaseExists(context.Background(), "v1"); err != nil || !ok {
		t.Errorf("v1 %v %v", ok, err)
	}
	if ok, err := p.ReleaseExists(context.Background(), "v2"); err != nil || ok {
		t.Errorf("v2 %v %v", ok, err)
	}
}

func TestPublish(t *testing.T) {
	calls := 0
	p := newP(t, "", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.EscapedPath() != "/projects/g%2Fs%2Fp/releases" {
			t.Errorf("req %s %s", r.Method, r.URL.EscapedPath())
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		want := map[string]any{"tag_name": "v1", "ref": "sha1", "name": "N", "description": "notes", "tag_message": "notes"}
		for k, v := range want {
			if b[k] != v {
				t.Errorf("body %s = %v", k, b[k])
			}
		}
		w.Write([]byte(`{}`))
	})
	u, err := p.Publish(context.Background(), provider.Release{Tag: "v1", SHA: "sha1", Name: "N", Notes: "notes"})
	if err != nil || u != "https://gl.example/g/s/p/-/releases/v1" {
		t.Errorf("publish %q %v", u, err)
	}
	calls = 0
	_, err = p.Publish(context.Background(), provider.Release{Tag: "v1", Draft: true})
	if err == nil || !strings.Contains(err.Error(), "--draft") || calls != 0 {
		t.Errorf("draft err=%v calls=%d", err, calls)
	}
	if p.Name() != "gitlab" {
		t.Error("name")
	}
}

func TestGitLabPrefixedTagPath(t *testing.T) {
	var got string
	p := newP(t, "", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.EscapedPath()
		http.NotFound(w, r)
	})
	p.TagSHA(context.Background(), "api/v1.0.0")
	if got != "/projects/g%2Fs%2Fp/repository/tags/api%2Fv1.0.0" {
		t.Errorf("path %q", got)
	}
}
