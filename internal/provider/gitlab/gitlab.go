// Package gitlab implements the GitLab release provider and URL linker.
package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/faridlamaul/shiptag/internal/provider"
	"github.com/faridlamaul/shiptag/internal/provider/httpx"
)

var (
	headerRe = regexp.MustCompile(`\s*\(!(\d+)\)\s*$`)
	bodyRe   = regexp.MustCompile(`See merge request (\S+?)!(\d+)`)
)

type linker struct{ web, repo string }

// NewLinker returns a GitLab URL linker.
func NewLinker(webURL, repo string) provider.Linker {
	return linker{web: strings.TrimRight(webURL, "/"), repo: repo}
}

func (l linker) base() string { return l.web + "/" + l.repo + "/-" }

func (l linker) CommitURL(sha string) string { return l.base() + "/commit/" + sha }
func (l linker) CompareURL(from, to string) string {
	return l.base() + "/compare/" + from + "..." + to
}
func (l linker) ChangeRequestURL(n int) string {
	return l.base() + "/merge_requests/" + strconv.Itoa(n)
}
func (l linker) IssueURL(n int) string         { return l.base() + "/issues/" + strconv.Itoa(n) }
func (l linker) Mention(name, _ string) string { return name }

func (l linker) ExtractChangeRequest(header, body string) (int, string, bool) {
	if m := headerRe.FindStringSubmatchIndex(header); m != nil {
		if n, err := strconv.Atoi(header[m[2]:m[3]]); err == nil {
			return n, header[:m[0]], true
		}
	}
	for _, m := range bodyRe.FindAllStringSubmatch(body, -1) {
		if m[1] != l.repo {
			continue
		}
		if n, err := strconv.Atoi(m[2]); err == nil {
			return n, header, true
		}
	}
	return 0, header, false
}

type gl struct {
	linker
	c       *httpx.Client
	project string // escaped project path segment
}

// New returns a GitLab provider.
func New(cfg provider.Config) provider.Provider {
	h := http.Header{}
	if cfg.UserAgent != "" {
		h.Set("User-Agent", cfg.UserAgent)
	}
	if cfg.Token != "" {
		if cfg.TokenKind == "job" {
			h.Set("JOB-TOKEN", cfg.Token)
		} else {
			h.Set("PRIVATE-TOKEN", cfg.Token)
		}
	}
	return &gl{
		linker:  linker{web: strings.TrimRight(cfg.WebURL, "/"), repo: cfg.Repo},
		c:       &httpx.Client{BaseURL: strings.TrimRight(cfg.APIURL, "/"), Header: h},
		project: "/projects/" + url.PathEscape(cfg.Repo),
	}
}

func (g *gl) Name() string            { return "gitlab" }
func (g *gl) Linker() provider.Linker { return g.linker }

func (g *gl) TagSHA(ctx context.Context, tag string) (string, bool, error) {
	var out struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := g.c.Do(ctx, "GET", g.project+"/repository/tags/"+url.PathEscape(tag), nil, &out); err != nil {
		if httpx.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if out.Commit.ID == "" {
		return "", false, errors.New("gitlab: tag response has no commit id")
	}
	return out.Commit.ID, true, nil
}

func (g *gl) ReleaseExists(ctx context.Context, tag string) (bool, error) {
	err := g.c.Do(ctx, "GET", g.project+"/releases/"+url.PathEscape(tag), nil, nil)
	if err == nil {
		return true, nil
	}
	if httpx.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

func (g *gl) Publish(ctx context.Context, r provider.Release) (string, error) {
	if r.Draft {
		return "", errors.New("gitlab does not support draft releases; remove --draft")
	}
	body := map[string]any{
		"tag_name":    r.Tag,
		"ref":         r.SHA,
		"name":        r.Name,
		"description": r.Notes,
		"tag_message": r.Notes,
	}
	if err := g.c.Do(ctx, "POST", g.project+"/releases", body, nil); err != nil {
		return "", err
	}
	return g.base() + "/releases/" + url.PathEscape(r.Tag), nil
}
