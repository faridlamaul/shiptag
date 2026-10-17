// Package config loads and validates the .shiptag.yaml configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

// DefaultFileName is the config file looked up in the repository top level.
const DefaultFileName = ".shiptag.yaml"

// Section describes one group of commit types in the release notes.
type Section struct {
	Title  string   `yaml:"title"`
	Types  []string `yaml:"types"`
	Hidden bool     `yaml:"hidden"`
}

// Config is the shiptag configuration.
type Config struct {
	TagPrefix          string            `yaml:"tag_prefix"`
	VPrefix            bool              `yaml:"v_prefix"`
	InitialVersion     string            `yaml:"initial_version"`
	InitialDevelopment bool              `yaml:"initial_development"`
	Path               string            `yaml:"path"`
	ReleaseRules       map[string]string `yaml:"release_rules"`
	BreakingTitle      string            `yaml:"breaking_title"`
	Sections           []Section         `yaml:"sections"`
	OtherSection       Section           `yaml:"other_section"`
	ExcludeAuthors     []string          `yaml:"exclude_authors"`
	ChangelogFile      string            `yaml:"changelog_file"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		VPrefix:            true,
		InitialVersion:     "0.1.0",
		InitialDevelopment: true,
		ReleaseRules: map[string]string{
			"feat":   "minor",
			"fix":    "patch",
			"perf":   "patch",
			"revert": "patch",
		},
		BreakingTitle: "⚠ BREAKING CHANGES",
		Sections: []Section{
			{Title: "Features", Types: []string{"feat"}},
			{Title: "Bug Fixes", Types: []string{"fix"}},
			{Title: "Performance Improvements", Types: []string{"perf"}},
			{Title: "Reverts", Types: []string{"revert"}},
			{Title: "Code Refactoring", Types: []string{"refactor"}},
			{Title: "Documentation", Types: []string{"docs"}},
			{Title: "Build System", Types: []string{"build"}, Hidden: true},
			{Title: "Continuous Integration", Types: []string{"ci"}, Hidden: true},
			{Title: "Chores", Types: []string{"chore"}, Hidden: true},
			{Title: "Styles", Types: []string{"style"}, Hidden: true},
			{Title: "Tests", Types: []string{"test"}, Hidden: true},
		},
		OtherSection:   Section{Title: "Other Changes", Hidden: true},
		ExcludeAuthors: []string{"[bot]", "dependabot", "renovate"},
	}
}

// Load reads the config at path. When path is empty it reads
// <topLevel>/.shiptag.yaml if present, otherwise returns Default().
func Load(path, topLevel string) (Config, error) {
	explicit := path != ""
	if !explicit {
		path = filepath.Join(topLevel, DefaultFileName)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !explicit && errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	defaults := cfg.ReleaseRules
	cfg.ReleaseRules = nil
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	for k, v := range cfg.ReleaseRules {
		defaults[k] = v
	}
	cfg.ReleaseRules = defaults

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", path, err)
	}
	return cfg, nil
}

// Validate checks the configuration for invalid values.
func (c Config) Validate() error {
	for typ, bump := range c.ReleaseRules {
		switch bump {
		case "major", "minor", "patch", "none":
		default:
			return fmt.Errorf("release_rules: %q has invalid bump %q (want major, minor, patch or none)", typ, bump)
		}
	}
	if _, err := semver.StrictNewVersion(strings.TrimPrefix(c.InitialVersion, "v")); err != nil {
		return fmt.Errorf("initial_version: %q is not a valid semantic version", c.InitialVersion)
	}
	for i, s := range c.Sections {
		if strings.TrimSpace(s.Title) == "" {
			return fmt.Errorf("sections[%d]: title must not be empty", i)
		}
	}
	if strings.IndexFunc(c.TagPrefix, unicode.IsSpace) >= 0 {
		return fmt.Errorf("tag_prefix: %q must not contain whitespace", c.TagPrefix)
	}
	return nil
}
