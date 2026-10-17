package provider

import (
	"fmt"
	"strings"
)

// Info is the resolved provider identity and connection settings.
type Info struct {
	Name, Repo, WebURL, APIURL, DefaultRef, Token, TokenKind string
	InCI                                                     bool
}

// DetectInput carries flag values, the git remote URL, and an injected env lookup.
type DetectInput struct {
	Provider, Repo, BaseURL, APIURL, Token, RemoteURL string
	Getenv                                            func(string) string
}

const (
	githubWeb    = "https://github.com"
	gitlabWeb    = "https://gitlab.com"
	bitbucketWeb = "https://bitbucket.org"
	bitbucketAPI = "https://api.bitbucket.org/2.0"
)

// Detect resolves provider, repository, URLs, ref, and token.
func Detect(in DetectInput) (Info, error) {
	getenv := in.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	var remoteHost, remotePath string
	if in.RemoteURL != "" {
		if h, p, err := ParseRemote(in.RemoteURL); err == nil {
			remoteHost, remotePath = h, p
		}
	}

	ci := ""
	switch {
	case getenv("GITHUB_ACTIONS") == "true":
		ci = "github"
	case getenv("GITLAB_CI") == "true":
		ci = "gitlab"
	case getenv("BITBUCKET_BUILD_NUMBER") != "":
		ci = "bitbucket"
	}

	name := strings.ToLower(strings.TrimSpace(in.Provider))
	switch name {
	case "":
		switch {
		case ci != "":
			name = ci
		case strings.Contains(remoteHost, "github"):
			name = "github"
		case strings.Contains(remoteHost, "gitlab"):
			name = "gitlab"
		case strings.Contains(remoteHost, "bitbucket.org"):
			name = "bitbucket"
		default:
			return Info{}, fmt.Errorf("cannot detect provider; pass --provider github|gitlab|bitbucket")
		}
	case "github", "gitlab", "bitbucket":
	default:
		return Info{}, fmt.Errorf("unknown provider %q; use github, gitlab, or bitbucket", in.Provider)
	}

	inCI := ci == name
	ienv := func(k string) string {
		if inCI {
			return getenv(k)
		}
		return ""
	}

	var envRepo, envSHA, envWeb, envAPI, publicWeb string
	switch name {
	case "github":
		envRepo, envSHA, envWeb, envAPI = ienv("GITHUB_REPOSITORY"), ienv("GITHUB_SHA"), ienv("GITHUB_SERVER_URL"), ienv("GITHUB_API_URL")
		publicWeb = githubWeb
	case "gitlab":
		envRepo, envSHA, envWeb, envAPI = ienv("CI_PROJECT_PATH"), ienv("CI_COMMIT_SHA"), ienv("CI_SERVER_URL"), ienv("CI_API_V4_URL")
		publicWeb = gitlabWeb
	case "bitbucket":
		envRepo, envSHA = ienv("BITBUCKET_REPO_FULL_NAME"), ienv("BITBUCKET_COMMIT")
		publicWeb = bitbucketWeb
	}

	repo := firstNonEmpty(in.Repo, envRepo, remotePath)
	if repo == "" {
		return Info{}, fmt.Errorf("cannot determine repository; pass --repo <path>")
	}

	web := strings.TrimRight(in.BaseURL, "/")
	if web == "" {
		web = strings.TrimRight(envWeb, "/")
	}
	if web == "" && remoteHost != "" && "https://"+remoteHost != publicWeb && remoteHost != strings.TrimPrefix(publicWeb, "https://") {
		web = "https://" + remoteHost
	}
	if web == "" {
		web = publicWeb
	}

	api := strings.TrimRight(in.APIURL, "/")
	if api == "" && in.BaseURL == "" {
		api = strings.TrimRight(envAPI, "/")
	}
	if api == "" {
		switch name {
		case "github":
			if web == githubWeb {
				api = "https://api.github.com"
			} else {
				api = web + "/api/v3"
			}
		case "gitlab":
			api = web + "/api/v4"
		case "bitbucket":
			api = bitbucketAPI
		}
	}

	ref := "HEAD"
	if inCI && envSHA != "" {
		ref = envSHA
	}

	token, kind := in.Token, ""
	if token == "" {
		switch name {
		case "github":
			token = firstNonEmpty(getenv("GITHUB_TOKEN"), getenv("GH_TOKEN"))
		case "gitlab":
			if token = getenv("GITLAB_TOKEN"); token == "" {
				if token = getenv("CI_JOB_TOKEN"); token != "" {
					kind = "job"
				}
			}
		case "bitbucket":
			token = getenv("BITBUCKET_TOKEN")
		}
	}

	return Info{Name: name, Repo: repo, WebURL: web, APIURL: api, DefaultRef: ref, Token: token, TokenKind: kind, InCI: inCI}, nil
}

// TokenHelp returns the hint shown when no token is available.
func TokenHelp(name string) string {
	switch name {
	case "github":
		return "set GITHUB_TOKEN or GH_TOKEN, or pass --token"
	case "gitlab":
		return "set GITLAB_TOKEN (or run in GitLab CI with CI_JOB_TOKEN), or pass --token"
	case "bitbucket":
		return "set BITBUCKET_TOKEN, or pass --token"
	}
	return "pass --token"
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
