package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoRetriesOnce(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return
		}
		_, _ = io.WriteString(w, `{"a":1}`)
	}))
	defer srv.Close()
	var sleeps []time.Duration
	c := &Client{BaseURL: srv.URL, Sleep: func(d time.Duration) { sleeps = append(sleeps, d) }}
	var out struct{ A int }
	if err := c.Do(context.Background(), "GET", "/x", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.A != 1 || len(sleeps) != 1 || sleeps[0] != 2*time.Second {
		t.Fatalf("out=%v sleeps=%v", out, sleeps)
	}
}

func TestDoRetryAfterCapAndDefault(t *testing.T) {
	for hdr, want := range map[string]time.Duration{"999": 30 * time.Second, "": time.Second, "abc": time.Second} {
		var n int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&n, 1) == 1 {
				if hdr != "" {
					w.Header().Set("Retry-After", hdr)
				}
				w.WriteHeader(503)
				return
			}
		}))
		var got time.Duration
		c := &Client{BaseURL: srv.URL, Sleep: func(d time.Duration) { got = d }}
		if err := c.Do(context.Background(), "GET", "/", nil, nil); err != nil {
			t.Fatal(err)
		}
		srv.Close()
		if got != want {
			t.Fatalf("hdr %q: got %v want %v", hdr, got, want)
		}
	}
}

func TestDoGivesUpAfterOneRetry(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Sleep: func(time.Duration) {}}
	err := c.Do(context.Background(), "GET", "/", nil, nil)
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 503 || n != 2 {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

func TestDoNoRetryOn400(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(400)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Sleep: func(time.Duration) { t.Fatal("slept") }}
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err == nil || n != 1 {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

func TestDoErrorRedactsAndTruncates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, strings.Repeat("z", 2000))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Header: http.Header{"Authorization": {"Bearer sekret"}}, Sleep: func(time.Duration) {}}
	err := c.Do(context.Background(), "POST", "/x?token=abc", map[string]string{"k": "v"}, nil)
	if err == nil {
		t.Fatal("want error")
	}
	s := err.Error()
	for _, bad := range []string{"abc", "sekret"} {
		if strings.Contains(s, bad) {
			t.Fatalf("error leaks %q", bad)
		}
	}
	if !strings.Contains(s, "POST /x") || !strings.Contains(s, "500") {
		t.Fatalf("error = %.80s", s)
	}
	if n := strings.Count(s, "z"); n != 512 {
		t.Fatalf("body bytes = %d", n)
	}
}

func TestDoHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Header.Clone() }))
	defer srv.Close()
	h := http.Header{"User-Agent": {"shiptag/test"}}
	c := &Client{BaseURL: srv.URL, Header: h}
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got.Get("Content-Type") != "" || got.Get("Accept") != "application/json" || got.Get("User-Agent") != "shiptag/test" {
		t.Fatalf("get headers %v", got)
	}
	if err := c.Do(context.Background(), "POST", "/", map[string]int{"a": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if got.Get("Content-Type") != "application/json" {
		t.Fatalf("post headers %v", got)
	}
	if h.Get("Accept") != "" || h.Get("Content-Type") != "" {
		t.Fatal("client Header mutated")
	}
	c.Header.Set("Accept", "application/vnd.github+json")
	_ = c.Do(context.Background(), "GET", "/", nil, nil)
	if got.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("accept %v", got)
	}
}

func TestDoEmptyBodyWithOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	var out map[string]any
	if err := (&Client{BaseURL: srv.URL}).Do(context.Background(), "GET", "/", nil, &out); err != nil {
		t.Fatal(err)
	}
}

func TestIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer srv.Close()
	err := (&Client{BaseURL: srv.URL}).Do(context.Background(), "GET", "/", nil, nil)
	if !IsNotFound(err) || IsNotFound(errors.New("x")) || IsNotFound(nil) {
		t.Fatalf("err=%v", err)
	}
}

func TestDoCancelDuringRetryWait(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(503)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	err := (&Client{BaseURL: srv.URL}).Do(ctx, "GET", "/", nil, nil)
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("err=%v after %v", err, time.Since(start))
	}
}

func TestDoPostRetriesOn429ResendingBody(t *testing.T) {
	var bodies []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]string
		_ = json.NewDecoder(r.Body).Decode(&m)
		bodies = append(bodies, m)
		if len(bodies) == 1 {
			w.WriteHeader(429)
		}
	}))
	defer srv.Close()
	sent := map[string]string{"k": "v"}
	c := &Client{BaseURL: srv.URL, Sleep: func(time.Duration) {}}
	if err := c.Do(context.Background(), "POST", "/", sent, nil); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0]["k"] != "v" || bodies[1]["k"] != "v" {
		t.Fatalf("bodies=%v", bodies)
	}
}

func TestDoTransportFailureRedacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := srv.URL
	srv.Close()
	err := (&Client{BaseURL: base}).Do(context.Background(), "GET", "/x?token=abc", nil, nil)
	if err == nil || strings.Contains(err.Error(), "abc") || !strings.Contains(err.Error(), "GET") || !strings.Contains(err.Error(), "/x") {
		t.Fatalf("err=%v", err)
	}
}

func TestDoRetryByMethod(t *testing.T) {
	tests := []struct {
		method string
		status int
		want   int32 // requests sent
	}{
		{"POST", 502, 1}, // may have been processed: never replay
		{"POST", 503, 1},
		{"PATCH", 500, 1},
		{"POST", 429, 2}, // 429 means not processed
		{"GET", 503, 2},
		{"HEAD", 502, 2},
		{"PUT", 500, 2},
		{"DELETE", 503, 2},
		{"OPTIONS", 503, 2},
	}
	for _, tc := range tests {
		var n int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&n, 1)
			w.WriteHeader(tc.status)
		}))
		c := &Client{BaseURL: srv.URL, Sleep: func(time.Duration) {}}
		err := c.Do(context.Background(), tc.method, "/", nil, nil)
		srv.Close()
		var se *StatusError
		if !errors.As(err, &se) || se.Status != tc.status || n != tc.want {
			t.Errorf("%s %d: err=%v n=%d want n=%d", tc.method, tc.status, err, n, tc.want)
		}
	}
}

func TestDoOversizedBody(t *testing.T) {
	for _, status := range []int{200, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `"`+strings.Repeat("z", maxResponse)+`"`)
		}))
		var out string
		err := (&Client{BaseURL: srv.URL}).Do(context.Background(), "POST", "/x?token=abc", nil, &out)
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "too large") || strings.Contains(err.Error(), "abc") || strings.Contains(err.Error(), "zzz") {
			t.Fatalf("status %d: err=%.120v", status, err)
		}
	}
}
