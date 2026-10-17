package bitbucket

import (
	"context"
	"encoding/json"
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
	return New(provider.Config{APIURL: srv.URL, WebURL: "https://bb.example/", Repo: "ws/r", Token: "tok"})
}

func TestLinker(t *testing.T) {
	l := NewLinker("https://bb.example/", "ws/r")
	if got := l.CommitURL("abc"); got != "https://bb.example/ws/r/commits/abc" {
		t.Errorf("commit %q", got)
	}
	if got := l.CompareURL("v1", "v2"); got != "https://bb.example/ws/r/branches/compare/v2%0Dv1" {
		t.Errorf("compare %q", got)
	}
	if got := l.ChangeRequestURL(5); got != "https://bb.example/ws/r/pull-requests/5" {
		t.Errorf("cr %q", got)
	}
	if got := l.IssueURL(5); got != "https://bb.example/ws/r/issues/5" {
		t.Errorf("issue %q", got)
	}
	n, h, ok := l.ExtractChangeRequest("Merged in feat/x (pull request #9)", "")
	if !ok || n != 9 || h != "Merged in feat/x" {
		t.Errorf("header %d %q %v", n, h, ok)
	}
	n, h, ok = l.ExtractChangeRequest("Merge", "stuff (pull request #9)")
	if !ok || n != 9 || h != "Merge" {
		t.Errorf("body %d %q %v", n, h, ok)
	}
	if _, _, ok := l.ExtractChangeRequest("plain", "none"); ok {
		t.Error("unexpected match")
	}
	n, h, ok = l.ExtractChangeRequest("Merged in feature/x (pull request #8)", "\nfeat: add y\n\nApproved-by: Bob")
	if !ok || n != 8 || h != "feat: add y" {
		t.Errorf("squash %d %q %v", n, h, ok)
	}
	if got := l.ChangeRequestRef(8); got != "#8" {
		t.Errorf("ref %q", got)
	}
	if got := l.Mention("Bob", "bob@x.io"); got != "Bob" {
		t.Errorf("mention %q", got)
	}
}

func TestTagSHA(t *testing.T) {
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("auth header")
		}
		if r.URL.EscapedPath() == "/repositories/ws/r/refs/tags/v1" {
			w.Write([]byte(`{"target":{"hash":"c1"}}`))
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
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/repositories/ws/r/refs/tags/v1" {
			w.Write([]byte(`{"target":{"hash":"c1"}}`))
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
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.EscapedPath() != "/repositories/ws/r/refs/tags" {
			t.Errorf("req %s %s", r.Method, r.URL.EscapedPath())
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("auth header")
		}
		var raw json.RawMessage
		json.NewDecoder(r.Body).Decode(&raw)
		if string(raw) != `{"name":"v1","target":{"hash":"sha1"},"message":"notes"}` {
			t.Errorf("body %s", raw)
		}
		w.Write([]byte(`{}`))
	})
	u, err := p.Publish(context.Background(), provider.Release{Tag: "v1", SHA: "sha1", Name: "N", Notes: "notes"})
	if err != nil || u != "https://bb.example/ws/r/src/v1" {
		t.Errorf("publish %q %v", u, err)
	}
	calls = 0
	_, err = p.Publish(context.Background(), provider.Release{Tag: "v1", Draft: true})
	if err == nil || !strings.Contains(err.Error(), "--draft") || calls != 0 {
		t.Errorf("draft err=%v calls=%d", err, calls)
	}
	if p.Name() != "bitbucket" {
		t.Error("name")
	}
}

func TestBitbucketPrefixedTagPath(t *testing.T) {
	var got string
	p := newP(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.EscapedPath()
		http.NotFound(w, r)
	})
	p.TagSHA(context.Background(), "api/v1.0.0")
	if got != "/repositories/ws/r/refs/tags/api%2Fv1.0.0" {
		t.Errorf("path %q", got)
	}
}
