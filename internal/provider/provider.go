// Package provider defines the hosting-provider abstractions and CI/remote detection.
package provider

import "context"

// Linker builds provider-specific URLs and parses change-request references.
type Linker interface {
	CommitURL(sha string) string
	CompareURL(from, to string) string
	ChangeRequestURL(n int) string
	IssueURL(n int) string
	ExtractChangeRequest(header, body string) (n int, cleanedHeader string, ok bool)
	Mention(name, email string) string // "@login" or name
}

// Release is a release to publish.
type Release struct {
	Tag, SHA, Name, Notes string
	Draft, Prerelease     bool
}

// Provider publishes releases to a hosting service.
type Provider interface {
	Name() string
	Linker() Linker
	TagSHA(ctx context.Context, tag string) (sha string, found bool, err error) // peels annotated tags
	ReleaseExists(ctx context.Context, tag string) (bool, error)
	Publish(ctx context.Context, r Release) (url string, err error)
}

// Config configures a provider implementation.
type Config struct {
	APIURL, WebURL, Repo, Token, TokenKind, UserAgent string // TokenKind: "" or "job" (GitLab CI_JOB_TOKEN)
}
