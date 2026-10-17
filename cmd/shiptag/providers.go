package main

import (
	"fmt"

	"github.com/faridlamaul/shiptag/internal/provider"
	"github.com/faridlamaul/shiptag/internal/provider/bitbucket"
	"github.com/faridlamaul/shiptag/internal/provider/github"
	"github.com/faridlamaul/shiptag/internal/provider/gitlab"
)

// newProvider builds the publishing client for the detected provider.
func newProvider(info provider.Info, userAgent string) (provider.Provider, error) {
	cfg := provider.Config{
		APIURL:    info.APIURL,
		WebURL:    info.WebURL,
		Repo:      info.Repo,
		Token:     info.Token,
		TokenKind: info.TokenKind,
		UserAgent: userAgent,
	}
	switch info.Name {
	case "github":
		return github.New(cfg), nil
	case "gitlab":
		return gitlab.New(cfg), nil
	case "bitbucket":
		return bitbucket.New(cfg), nil
	}
	return nil, fmt.Errorf("unknown provider %q", info.Name)
}

// newLinker builds the URL linker for the detected provider. It needs no
// token and makes no network calls, so dry-runs use it too.
func newLinker(info provider.Info) provider.Linker {
	switch info.Name {
	case "gitlab":
		return gitlab.NewLinker(info.WebURL, info.Repo)
	case "bitbucket":
		return bitbucket.NewLinker(info.WebURL, info.Repo)
	}
	return github.NewLinker(info.WebURL, info.Repo)
}
