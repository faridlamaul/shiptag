// Package github implements the GitHub release provider and URL linker.
package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/faridlamaul/shiptag/internal/provider"
	"github.com/faridlamaul/shiptag/internal/provider/httpx"
)

var (
	crRe      = regexp.MustCompile(`\s*\(#(\d+)\)\s*$`)
	noreplyRe = regexp.MustCompile(`(?i)^(?:\d+\+)?([A-Za-z0-9-]+)@users\.noreply\.github\.com$`)
)

type linker struct{ web, repo string }

// NewLinker returns a GitHub URL linker.
func NewLinker(webURL, repo string) provider.Linker {
	return linker{web: strings.TrimRight(webURL, "/"), repo: repo}
}

func (l linker) base() string { return l.web + "/" + l.repo }

func (l linker) CommitURL(sha string) string { return l.base() + "/commit/" + sha }
func (l linker) CompareURL(from, to string) string {
	return l.base() + "/compare/" + from + "..." + to
}
func (l linker) ChangeRequestURL(n int) string { return l.base() + "/pull/" + strconv.Itoa(n) }
func (l linker) ChangeRequestRef(n int) string { return "#" + strconv.Itoa(n) }
func (l linker) IssueURL(n int) string         { return l.base() + "/issues/" + strconv.Itoa(n) }

func (l linker) ExtractChangeRequest(header, body string) (int, string, bool) {
	m := crRe.FindStringSubmatchIndex(header)
	if m == nil {
		return 0, header, false
	}
	n, err := strconv.Atoi(header[m[2]:m[3]])
	if err != nil {
		return 0, header, false
	}
	return n, header[:m[0]], true
}

func (l linker) Mention(name, email string) string {
	if m := noreplyRe.FindStringSubmatch(email); m != nil {
		return "@" + m[1]
	}
	return name
}

type gh struct {
	linker
	c *httpx.Client
	o string // owner/name path prefix
}

// New returns a GitHub provider.
func New(cfg provider.Config) provider.Provider {
	h := http.Header{}
	h.Set("Accept", "application/vnd.github+json")
	h.Set("X-GitHub-Api-Version", "2022-11-28")
	if cfg.UserAgent != "" {
		h.Set("User-Agent", cfg.UserAgent)
	}
	if cfg.Token != "" {
		h.Set("Authorization", "Bearer "+cfg.Token)
	}
	owner, name, _ := strings.Cut(cfg.Repo, "/")
	return &gh{
		linker: linker{web: strings.TrimRight(cfg.WebURL, "/"), repo: cfg.Repo},
		c:      &httpx.Client{BaseURL: strings.TrimRight(cfg.APIURL, "/"), Header: h},
		o:      "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name),
	}
}

func (g *gh) Name() string            { return "github" }
func (g *gh) Linker() provider.Linker { return g.linker }

type gitObject struct {
	Object struct{ Type, SHA string } `json:"object"`
}

// escapeRef escapes each segment of a ref while keeping "/" separators.
func escapeRef(ref string) string {
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func (g *gh) TagSHA(ctx context.Context, tag string) (string, bool, error) {
	var ref gitObject
	if err := g.c.Do(ctx, "GET", g.o+"/git/ref/tags/"+escapeRef(tag), nil, &ref); err != nil {
		if httpx.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	obj := ref.Object
	for i := 0; obj.Type == "tag" && i < 5; i++ {
		var t gitObject
		if err := g.c.Do(ctx, "GET", g.o+"/git/tags/"+url.PathEscape(obj.SHA), nil, &t); err != nil {
			return "", false, err
		}
		obj = t.Object
	}
	if obj.SHA == "" {
		return "", false, fmt.Errorf("github: tag %q resolved to empty SHA", tag)
	}
	return obj.SHA, true, nil
}

func (g *gh) ReleaseExists(ctx context.Context, tag string) (bool, error) {
	err := g.c.Do(ctx, "GET", g.o+"/releases/tags/"+escapeRef(tag), nil, nil)
	if err == nil {
		return true, nil
	}
	if !httpx.IsNotFound(err) {
		return false, err
	}
	// Draft releases are not served by releases/tags; scan the list.
	var list []struct {
		TagName string `json:"tag_name"`
	}
	if err := g.c.Do(ctx, "GET", g.o+"/releases?per_page=100", nil, &list); err != nil {
		return false, err
	}
	for _, r := range list {
		if r.TagName == tag {
			return true, nil
		}
	}
	return false, nil
}

func (g *gh) Publish(ctx context.Context, r provider.Release) (string, error) {
	body := map[string]any{
		"tag_name":         r.Tag,
		"target_commitish": r.SHA,
		"name":             r.Name,
		"body":             r.Notes,
		"draft":            r.Draft,
		"prerelease":       r.Prerelease,
	}
	var out struct {
		HTMLURL string `json:"html_url"`
	}
	if err := g.c.Do(ctx, "POST", g.o+"/releases", body, &out); err != nil {
		return "", err
	}
	return out.HTMLURL, nil
}
