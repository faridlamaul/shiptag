// Package httpx is the shared JSON HTTP client used by the provider implementations.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxBody       = 512
	maxRetryAfter = 30 * time.Second
	defaultRetry  = time.Second
)

// Client is a small JSON-over-HTTP client with a single retry on 429/5xx.
type Client struct {
	BaseURL string
	Header  http.Header
	HTTP    *http.Client        // nil -> 30s timeout client
	Sleep   func(time.Duration) // nil -> time.Sleep
}

// StatusError describes a non-2xx response. Path excludes the query string.
type StatusError struct {
	Method, Path string
	Status       int
	Body         string
}

func (e *StatusError) Error() string {
	s := fmt.Sprintf("%s %s: HTTP %d", e.Method, e.Path, e.Status)
	if e.Body != "" {
		s += ": " + e.Body
	}
	return s
}

// IsNotFound reports whether err is a *StatusError with status 404.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == http.StatusNotFound
}

// Do sends a JSON body (if non-nil) and decodes a 2xx JSON response into out (if non-nil).
// path is appended to BaseURL verbatim.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s %s: encode body: %w", method, stripQuery(path), err)
		}
		payload = b
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	safePath := stripQuery(path)

	for attempt := 0; ; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
		if err != nil {
			return fmt.Errorf("%s %s: build request failed", method, safePath)
		}
		req.Header = c.Header.Clone()
		if req.Header == nil {
			req.Header = http.Header{}
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if req.Header.Get("Accept") == "" {
			req.Header.Set("Accept", "application/json")
		}
		resp, err := hc.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("%s %s: %w", method, safePath, ctx.Err())
			}
			// url.Error embeds the full URL (may carry a query); report only the cause.
			return fmt.Errorf("%s %s: request failed: %s", method, safePath, rootMessage(err))
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if readErr != nil {
				return fmt.Errorf("%s %s: read response: %s", method, safePath, rootMessage(readErr))
			}
			if out == nil || len(bytes.TrimSpace(data)) == 0 {
				return nil
			}
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("%s %s: decode response: %w", method, safePath, err)
			}
			return nil
		}
		if attempt == 0 && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
			sleep(retryDelay(resp.Header.Get("Retry-After")))
			continue
		}
		if len(data) > maxBody {
			data = data[:maxBody]
		}
		return &StatusError{Method: method, Path: safePath, Status: resp.StatusCode, Body: string(data)}
	}
}

func stripQuery(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[:i]
	}
	return p
}

func retryDelay(h string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || n < 0 {
		return defaultRetry
	}
	d := time.Duration(n) * time.Second
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}

func rootMessage(err error) string {
	for {
		u := errors.Unwrap(err)
		if u == nil {
			return err.Error()
		}
		err = u
	}
}
