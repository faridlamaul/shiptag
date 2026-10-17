package githubclient

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/faridlamaul/shiptag/pkg/changelog"
	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

// Client wraps the GitHub API client for release automation.
type Client struct {
	gh    *github.Client
	Owner string
	Repo  string
}

// NewClient initializes a GitHub client using token authentication.
func NewClient(ctx context.Context, token, repoSlug string) (*Client, error) {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
		if token == "" {
			token = os.Getenv("GH_TOKEN")
		}
	}
	if token == "" {
		return nil, fmt.Errorf("GitHub token is required (set GITHUB_TOKEN or pass --token)")
	}

	if repoSlug == "" {
		repoSlug = os.Getenv("GITHUB_REPOSITORY")
	}
	if repoSlug == "" {
		return nil, fmt.Errorf("target repository is required (set GITHUB_REPOSITORY or pass --repo owner/repo)")
	}

	parts := strings.Split(repoSlug, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository slug %q (must be in 'owner/repo' format)", repoSlug)
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	ghClient := github.NewClient(tc)

	return &Client{
		gh:    ghClient,
		Owner: parts[0],
		Repo:  parts[1],
	}, nil
}

// ListTags fetches all tag names from the repository.
func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	opt := &github.ListOptions{PerPage: 100}
	var allTags []string

	for {
		tags, resp, err := c.gh.Repositories.ListTags(ctx, c.Owner, c.Repo, opt)
		if err != nil {
			return nil, fmt.Errorf("failed to list tags: %w", err)
		}
		for _, t := range tags {
			if t.Name != nil {
				allTags = append(allTags, *t.Name)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allTags, nil
}

// GetCommitsSinceTag fetches commit history between baseTag and targetRef.
// If baseTag is empty, fetches the last 50 commits on targetRef.
func (c *Client) GetCommitsSinceTag(ctx context.Context, baseTag, targetRef string) ([]changelog.CommitInfo, error) {
	if targetRef == "" {
		targetRef = "main"
	}

	var commits []changelog.CommitInfo

	if baseTag != "" {
		comp, _, err := c.gh.Repositories.CompareCommits(ctx, c.Owner, c.Repo, baseTag, targetRef, &github.ListOptions{PerPage: 100})
		if err != nil {
			return nil, fmt.Errorf("failed to compare %s...%s: %w", baseTag, targetRef, err)
		}
		for _, rc := range comp.Commits {
			commits = append(commits, parseRepositoryCommit(rc))
		}
	} else {
		// No previous tag: list commits up to targetRef
		listOpt := &github.CommitsListOptions{
			SHA:         targetRef,
			ListOptions: github.ListOptions{PerPage: 50},
		}
		rcs, _, err := c.gh.Repositories.ListCommits(ctx, c.Owner, c.Repo, listOpt)
		if err != nil {
			return nil, fmt.Errorf("failed to list commits on %s: %w", targetRef, err)
		}
		// Reverse to chronological order (oldest to newest)
		for i := len(rcs) - 1; i >= 0; i-- {
			commits = append(commits, parseRepositoryCommit(rcs[i]))
		}
	}

	return commits, nil
}

func parseRepositoryCommit(rc *github.RepositoryCommit) changelog.CommitInfo {
	sha := ""
	if rc.SHA != nil {
		sha = *rc.SHA
	}

	title := ""
	body := ""
	authorName := ""
	authorLogin := ""

	if rc.Commit != nil {
		if rc.Commit.Message != nil {
			lines := strings.SplitN(*rc.Commit.Message, "\n", 2)
			title = lines[0]
			if len(lines) > 1 {
				body = lines[1]
			}
		}
		if rc.Commit.Author != nil && rc.Commit.Author.Name != nil {
			authorName = *rc.Commit.Author.Name
		}
	}

	if rc.Author != nil && rc.Author.Login != nil {
		authorLogin = *rc.Author.Login
	}

	return changelog.CommitInfo{
		SHA:         sha,
		Title:       title,
		Body:        body,
		AuthorName:  authorName,
		AuthorLogin: authorLogin,
	}
}

// CreateTag creates a lightweight git reference tag (refs/tags/<tagName>).
func (c *Client) CreateTag(ctx context.Context, tagName, sha string) error {
	refName := "refs/tags/" + tagName
	ref := &github.Reference{
		Ref:    &refName,
		Object: &github.GitObject{SHA: &sha},
	}
	_, _, err := c.gh.Git.CreateRef(ctx, c.Owner, c.Repo, ref)
	if err != nil {
		return fmt.Errorf("failed to create git tag %s: %w", tagName, err)
	}
	return nil
}

// CreateRelease creates a GitHub release with release notes.
func (c *Client) CreateRelease(ctx context.Context, tagName, targetCommitish, title, notes string, isDraft, isPrerelease bool) (*github.RepositoryRelease, error) {
	relReq := &github.RepositoryRelease{
		TagName:         &tagName,
		TargetCommitish: &targetCommitish,
		Name:            &title,
		Body:            &notes,
		Draft:           &isDraft,
		Prerelease:      &isPrerelease,
	}

	rel, _, err := c.gh.Repositories.CreateRelease(ctx, c.Owner, c.Repo, relReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create release %s: %w", tagName, err)
	}
	return rel, nil
}

// CreateBranch creates a git branch from a commit SHA.
func (c *Client) CreateBranch(ctx context.Context, branchName, sha string) error {
	refName := "refs/heads/" + branchName
	ref := &github.Reference{
		Ref:    &refName,
		Object: &github.GitObject{SHA: &sha},
	}
	_, _, err := c.gh.Git.CreateRef(ctx, c.Owner, c.Repo, ref)
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}
	return nil
}
