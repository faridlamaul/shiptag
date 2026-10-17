package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/faridlamaul/shiptag/internal/commits"
	"github.com/faridlamaul/shiptag/internal/config"
	"github.com/faridlamaul/shiptag/internal/gitrepo"
	"github.com/faridlamaul/shiptag/internal/notes"
	"github.com/faridlamaul/shiptag/internal/provider"
	ver "github.com/faridlamaul/shiptag/internal/version"
	"github.com/spf13/cobra"
)

const shallowHelp = "shallow clone detected: shiptag needs the full history to select tags and read commits; " +
	"use a full clone (GitHub Actions: actions/checkout with fetch-depth: 0; " +
	"GitLab CI: variables GIT_DEPTH: 0; Bitbucket Pipelines: clone: depth: full) " +
	"or run git fetch --unshallow --tags"

// globals holds the flags shared by every command.
type globals struct {
	provider, repo, ref, baseURL, apiURL, token, config, dir, tagPrefix, path string
}

// request selects what analyze computes.
type request struct {
	bump, explicit, prerelease string
	needVersion                bool // false for changelog without --version
}

// analysis is the result of reading the repository: everything a command
// needs to plan, print, or publish a release.
type analysis struct {
	cfg    config.Config
	info   provider.Info
	linker provider.Linker
	scheme ver.Scheme
	ref    string
	sha    string

	stable    *ver.Tag // stable base, nil for a first release
	notesBase *ver.Tag // stable base, or latest tag of any kind for prereleases

	bumpCommits  []commits.Commit
	notesCommits []commits.Commit
	level        ver.Level
	reasons      []ver.Reason

	release bool // false = nothing to release
	result  ver.Result
	date    time.Time
}

// previousTag is the notes base tag name, or "".
func (a *analysis) previousTag() string {
	if a.notesBase == nil {
		return ""
	}
	return a.notesBase.Name
}

// previousVersion is the notes base version, or "".
func (a *analysis) previousVersion() string {
	if a.notesBase == nil {
		return ""
	}
	return a.notesBase.Version.String()
}

// render renders the release notes for the notes range.
func (a *analysis) render(heading, compareTo string) (string, int, int) {
	sections := make([]notes.Section, len(a.cfg.Sections))
	for i, s := range a.cfg.Sections {
		sections[i] = notes.Section{Title: s.Title, Types: s.Types, Hidden: s.Hidden}
	}
	o := a.cfg.OtherSection
	return notes.Render(a.notesCommits, notes.Options{
		Heading:        heading,
		PreviousTag:    a.previousTag(),
		CompareTo:      compareTo,
		Date:           a.date,
		BreakingTitle:  a.cfg.BreakingTitle,
		Sections:       sections,
		Other:          notes.Section{Title: o.Title, Types: o.Types, Hidden: o.Hidden},
		ExcludeAuthors: a.cfg.ExcludeAuthors,
		Linker:         a.linker,
	})
}

// analyze runs the shared pipeline: open the repository, load config,
// detect the provider, select base tags, read and parse commits, and compute
// the next version. It never touches the network.
func analyze(cmd *cobra.Command, g *globals, req request, getenv func(string) string, logger *log.Logger) (*analysis, error) {
	repo, err := gitrepo.Open(g.dir)
	if err != nil {
		return nil, err
	}
	shallow, err := repo.IsShallow()
	if err != nil {
		return nil, err
	}
	if shallow {
		return nil, errors.New(shallowHelp)
	}
	top, err := repo.TopLevel()
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(g.config, top)
	if err != nil {
		return nil, err
	}
	flags := cmd.Flags()
	if flags.Changed("tag-prefix") {
		cfg.TagPrefix = g.tagPrefix
	}
	if flags.Changed("path") {
		cfg.Path = g.path
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	remote, err := repo.RemoteURL("origin")
	if err != nil {
		return nil, err
	}
	info, err := provider.Detect(provider.DetectInput{
		Provider: g.provider, Repo: g.repo, BaseURL: g.baseURL, APIURL: g.apiURL,
		Token: g.token, RemoteURL: remote, Getenv: getenv,
	})
	if err != nil {
		return nil, err
	}

	a := &analysis{
		cfg:    cfg,
		info:   info,
		linker: newLinker(info),
		scheme: ver.Scheme{Prefix: cfg.TagPrefix, VPrefix: cfg.VPrefix},
		ref:    g.ref,
	}
	if a.ref == "" {
		a.ref = info.DefaultRef
	}
	if a.sha, err = repo.ResolveSHA(a.ref); err != nil {
		return nil, err
	}
	if a.date, err = runDate(getenv); err != nil {
		return nil, err
	}

	reachable, err := parsedTags(repo, a.scheme, a.sha)
	if err != nil {
		return nil, err
	}
	all, err := parsedTags(repo, a.scheme, "")
	if err != nil {
		return nil, err
	}
	stable, latest := ver.Bases(reachable)
	a.stable, a.notesBase = stable, stable
	if req.prerelease != "" {
		a.notesBase = latest
	}

	if cfg.Path != "" && cfg.TagPrefix == "" {
		logger.Printf("warning: path %q is set without tag_prefix; tags of other packages are counted too", cfg.Path)
	}

	extract := a.linker.ExtractChangeRequest
	if a.bumpCommits, err = readCommits(repo, a.stable, a.sha, cfg.Path, extract); err != nil {
		return nil, err
	}
	a.notesCommits = a.bumpCommits
	if a.notesBase != a.stable {
		if a.notesCommits, err = readCommits(repo, a.notesBase, a.sha, cfg.Path, extract); err != nil {
			return nil, err
		}
	}

	rules, err := releaseRules(cfg.ReleaseRules)
	if err != nil {
		return nil, err
	}
	a.level, a.reasons = ver.Determine(a.bumpCommits, rules)

	if !req.needVersion || len(a.bumpCommits) == 0 {
		return a, nil
	}
	a.result, err = ver.Next(a.stable, a.level, all, ver.Options{
		Bump:               req.bump,
		Explicit:           req.explicit,
		Prerelease:         req.prerelease,
		InitialVersion:     cfg.InitialVersion,
		InitialDevelopment: cfg.InitialDevelopment,
		Scheme:             a.scheme,
	})
	if err != nil {
		return nil, err
	}
	a.release = a.result.Release
	return a, nil
}

func parsedTags(repo *gitrepo.Repo, s ver.Scheme, mergedInto string) ([]ver.Tag, error) {
	names, err := repo.Tags(s.Prefix, mergedInto)
	if err != nil {
		return nil, err
	}
	var tags []ver.Tag
	for _, n := range names {
		if t, ok := s.Parse(n); ok {
			tags = append(tags, t)
		}
	}
	return tags, nil
}

func readCommits(repo *gitrepo.Repo, base *ver.Tag, sha, path string, extract commits.CRExtractor) ([]commits.Commit, error) {
	from := ""
	if base != nil {
		from = base.Name
	}
	raws, err := repo.Log(from, sha, path)
	if err != nil {
		return nil, err
	}
	cs := make([]commits.Commit, len(raws))
	for i, r := range raws {
		cs[i] = commits.Parse(r, extract)
	}
	return cs, nil
}

func releaseRules(m map[string]string) (ver.Rules, error) {
	r := ver.Rules{Types: make(map[string]ver.Level, len(m))}
	for typ, lvl := range m {
		l, err := ver.ParseLevel(lvl)
		if err != nil {
			return r, fmt.Errorf("release_rules.%s: %w", typ, err)
		}
		r.Types[typ] = l
	}
	return r, nil
}

// runDate is SOURCE_DATE_EPOCH when set, otherwise the current UTC time.
func runDate(getenv func(string) string) (time.Time, error) {
	s := strings.TrimSpace(getenv("SOURCE_DATE_EPOCH"))
	if s == "" {
		return time.Now().UTC(), nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid SOURCE_DATE_EPOCH %q: want unix seconds", s)
	}
	return time.Unix(n, 0).UTC(), nil
}

func short(sha string, n int) string {
	if len(sha) > n {
		return sha[:n]
	}
	return sha
}
