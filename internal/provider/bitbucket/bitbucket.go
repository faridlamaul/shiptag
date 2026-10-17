// Package bitbucket implements the Bitbucket Cloud provider and URL linker.
package bitbucket

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/faridlamaul/shiptag/internal/commits"
	"github.com/faridlamaul/shiptag/internal/provider"
	"github.com/faridlamaul/shiptag/internal/provider/httpx"
)

var (
	prRe     = regexp.MustCompile(`\s*\(pull request #(\d+)\)`)
	squashRe = regexp.MustCompile(`^Merged in .+ \(pull request #(\d+)\)$`)
)

type linker struct{ web, repo string }

// NewLinker returns a Bitbucket Cloud URL linker.
func NewLinker(webURL, repo string) provider.Linker {
	return linker{web: strings.TrimRight(webURL, "/"), repo: repo}
}

func (l linker) base() string { return l.web + "/" + l.repo }

func (l linker) CommitURL(sha string) string { return l.base() + "/commits/" + sha }
func (l linker) CompareURL(from, to string) string {
	return l.base() + "/branches/compare/" + to + "%0D" + from
}
func (l linker) ChangeRequestURL(n int) string { return l.base() + "/pull-requests/" + strconv.Itoa(n) }
func (l linker) ChangeRequestRef(n int) string { return "#" + strconv.Itoa(n) }
func (l linker) IssueURL(n int) string         { return l.base() + "/issues/" + strconv.Itoa(n) }
func (l linker) Mention(name, _ string) string { return name }

// ExtractChangeRequest recognizes "(pull request #N)" in the header or body.
// For Bitbucket's squash/merge title "Merged in <branch> (pull request #N)"
// the real (conventional) header is the first non-empty body line, which is
// returned as the cleaned header; commits.Parse then drops that line from the
// body.
func (l linker) ExtractChangeRequest(header, body string) (int, string, bool) {
	if m := squashRe.FindStringSubmatch(header); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			if first := commits.FirstLine(body); first != "" {
				return n, first, true
			}
		}
	}
	if m := prRe.FindStringSubmatchIndex(header); m != nil {
		if n, err := strconv.Atoi(header[m[2]:m[3]]); err == nil {
			return n, strings.TrimSpace(header[:m[0]] + header[m[1]:]), true
		}
	}
	if m := prRe.FindStringSubmatch(body); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, header, true
		}
	}
	return 0, header, false
}

type bb struct {
	linker
	c    *httpx.Client
	repo string // escaped /repositories/{ws}/{repo} prefix
}

// New returns a Bitbucket Cloud provider.
func New(cfg provider.Config) provider.Provider {
	h := http.Header{}
	if cfg.UserAgent != "" {
		h.Set("User-Agent", cfg.UserAgent)
	}
	if cfg.Token != "" {
		h.Set("Authorization", "Bearer "+cfg.Token)
	}
	ws, name, _ := strings.Cut(cfg.Repo, "/")
	return &bb{
		linker: linker{web: strings.TrimRight(cfg.WebURL, "/"), repo: cfg.Repo},
		c:      &httpx.Client{BaseURL: strings.TrimRight(cfg.APIURL, "/"), Header: h},
		repo:   "/repositories/" + url.PathEscape(ws) + "/" + url.PathEscape(name),
	}
}

func (b *bb) Name() string            { return "bitbucket" }
func (b *bb) Linker() provider.Linker { return b.linker }

func (b *bb) TagSHA(ctx context.Context, tag string) (string, bool, error) {
	var out struct {
		Target struct {
			Hash string `json:"hash"`
		} `json:"target"`
	}
	if err := b.c.Do(ctx, "GET", b.repo+"/refs/tags/"+url.PathEscape(tag), nil, &out); err != nil {
		if httpx.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if out.Target.Hash == "" {
		return "", false, errors.New("bitbucket: tag response has no target hash")
	}
	return out.Target.Hash, true, nil
}

// ReleaseExists reports tag existence: Bitbucket Cloud has no release objects.
func (b *bb) ReleaseExists(ctx context.Context, tag string) (bool, error) {
	_, found, err := b.TagSHA(ctx, tag)
	return found, err
}

type tagTarget struct {
	Hash string `json:"hash"`
}

type tagRequest struct {
	Name    string    `json:"name"`
	Target  tagTarget `json:"target"`
	Message string    `json:"message"`
}

func (b *bb) Publish(ctx context.Context, r provider.Release) (string, error) {
	if r.Draft {
		return "", errors.New("bitbucket does not support draft releases; remove --draft")
	}
	req := tagRequest{Name: r.Tag, Target: tagTarget{Hash: r.SHA}, Message: r.Notes}
	if err := b.c.Do(ctx, "POST", b.repo+"/refs/tags", req, nil); err != nil {
		return "", err
	}
	return b.base() + "/src/" + url.PathEscape(r.Tag), nil
}
