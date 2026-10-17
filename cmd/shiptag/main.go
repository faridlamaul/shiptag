// Command shiptag computes semantic versions and release notes from the local
// git clone and publishes releases to GitHub, GitLab, or Bitbucket Cloud.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strings"

	"github.com/faridlamaul/shiptag/internal/output"
	"github.com/faridlamaul/shiptag/internal/provider"
	"github.com/spf13/cobra"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run executes the CLI with injected arguments, environment, and writers and
// returns the process exit code.
func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	g := &globals{}
	var secrets []string // values redacted from error messages
	logger := log.New(stderr, "", 0)

	analyzeFor := func(cmd *cobra.Command, req request) (*analysis, error) {
		a, err := analyze(cmd, g, req, getenv, logger)
		if a != nil && a.info.Token != "" {
			secrets = append(secrets, a.info.Token)
		}
		return a, err
	}

	root := &cobra.Command{
		Use:   "shiptag",
		Short: "Semantic versioning and release automation for GitHub, GitLab, and Bitbucket",
		Long: `shiptag analyzes Conventional Commits in the local git clone, calculates the
next semantic version, renders release notes, and publishes the release to
GitHub, GitLab, or Bitbucket Cloud.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	pf := root.PersistentFlags()
	pf.StringVar(&g.provider, "provider", "", "hosting provider: github, gitlab, or bitbucket (default: detected)")
	pf.StringVar(&g.repo, "repo", "", "repository path, e.g. owner/repo (default: from CI or the origin remote)")
	pf.StringVar(&g.ref, "ref", "", "commit to release (default: the CI commit SHA, else HEAD)")
	pf.StringVar(&g.baseURL, "base-url", "", "web base URL for self-hosted instances")
	pf.StringVar(&g.apiURL, "api-url", "", "API base URL (default: derived from the web base URL)")
	pf.StringVar(&g.token, "token", "", "API token (default: from the provider's environment variables)")
	pf.StringVar(&g.config, "config", "", "config file (default: .shiptag.yaml in the repository root)")
	pf.StringVar(&g.dir, "dir", ".", "repository directory")
	pf.StringVar(&g.tagPrefix, "tag-prefix", "", "version tag prefix, e.g. api/")
	pf.StringVar(&g.path, "path", "", "count only commits touching this path")

	root.AddCommand(
		tagCmd(analyzeFor, getenv, stdout, logger),
		changelogCmd(analyzeFor, stdout),
		nextVersionCmd(analyzeFor, stdout, logger),
	)

	if err := root.Execute(); err != nil {
		msg := err.Error()
		for _, s := range append(secrets, g.token) {
			if s != "" {
				msg = strings.ReplaceAll(msg, s, "***")
			}
		}
		fmt.Fprintf(stderr, "error: %s\n", msg)
		return 1
	}
	return 0
}

type analyzer func(cmd *cobra.Command, req request) (*analysis, error)

func validBump(b string) error {
	switch b {
	case "auto", "major", "minor", "patch":
		return nil
	}
	return fmt.Errorf("invalid --bump %q: want auto, major, minor, or patch", b)
}

func tagCmd(analyzeFor analyzer, getenv func(string) string, stdout io.Writer, logger *log.Logger) *cobra.Command {
	var (
		bump, explicit, prerelease, changelogFile, outputFile string
		draft, dryRun, asJSON                                 bool
	)
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Compute the next version, publish the release, and write CI outputs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validBump(bump); err != nil {
				return err
			}
			req := request{bump: bump, explicit: explicit, prerelease: prerelease, needVersion: true}
			a, err := analyzeFor(cmd, req)
			if err != nil {
				return err
			}
			if draft && a.info.Name != "github" {
				return fmt.Errorf("--draft is only supported for GitHub; %s has no draft releases", a.info.Name)
			}
			if cmd.Flags().Changed("changelog-file") {
				a.cfg.ChangelogFile = changelogFile
			}
			dest := output.Destination(a.info.Name, a.info.InCI, outputFile, getenv)
			// With --json the stdout KEY=VALUE fallback is suppressed.
			writeOutputs := !(asJSON && dest == "")
			destName := dest
			if destName == "" {
				destName = "stdout"
			}
			p := newPlan(a, req, dryRun)
			emit := func() error {
				if asJSON {
					return p.writeJSON(stdout)
				}
				if dryRun {
					return p.writeHuman(stdout)
				}
				return nil
			}
			vals := output.Values{PreviousVersion: a.previousVersion()}

			if !a.release {
				logger.Println("nothing to release")
				if !dryRun && writeOutputs {
					if err := output.Write(dest, vals, stdout); err != nil {
						return fmt.Errorf("write outputs: %w", err)
					}
					p.Actions = append(p.Actions, "wrote outputs to "+destName)
				}
				if asJSON {
					return p.writeJSON(stdout)
				}
				return nil
			}

			tag := a.result.Tag
			p.Notes, p.Commits.Counted, p.Commits.Hidden = a.render(tag, tag)
			vals.Version, vals.Tag, vals.SHA, vals.Bump = p.Version, tag, a.sha, p.Bump

			if dryRun {
				p.Actions = append(p.Actions, publishAction(a.info.Name, tag, a.sha, draft, true))
				if writeOutputs {
					p.Actions = append(p.Actions, "would write outputs to "+destName)
				}
				if a.cfg.ChangelogFile != "" {
					p.Actions = append(p.Actions, "would prepend "+a.cfg.ChangelogFile)
				}
				return emit()
			}

			if a.info.Token == "" {
				return errors.New("missing token: " + provider.TokenHelp(a.info.Name))
			}
			prov, err := newProvider(a.info, "shiptag/"+version)
			if err != nil {
				return err
			}
			ctx := context.Background()
			remoteSHA, found, err := prov.TagSHA(ctx, tag)
			if err != nil {
				return err
			}
			if found && remoteSHA != a.sha {
				return fmt.Errorf("tag %s already points to %s, expected %s", tag, remoteSHA, a.sha)
			}
			// A GitHub draft release creates no tag, so a rerun with --draft
			// must look for the draft itself before publishing another one.
			if found || draft {
				exists, err := prov.ReleaseExists(ctx, tag)
				if err != nil {
					return err
				}
				if exists {
					logger.Println("already released")
					p.Actions = append(p.Actions, fmt.Sprintf("release %s already exists at %s", tag, short(a.sha, 7)))
					if writeOutputs {
						if err := output.Write(dest, vals, stdout); err != nil {
							return fmt.Errorf("write outputs: %w", err)
						}
						p.Actions = append(p.Actions, "wrote outputs to "+destName)
					}
					if err := updateChangelog(a.cfg.ChangelogFile, tag, p); err != nil {
						return err
					}
					return emit()
				}
			}

			url, err := prov.Publish(ctx, provider.Release{
				Tag: tag, SHA: a.sha, Name: tag, Notes: p.Notes,
				Draft: draft, Prerelease: a.result.Version.Prerelease() != "",
			})
			if err != nil {
				return err
			}
			logger.Printf("released %s: %s", tag, url)
			p.Released, p.URL = true, url
			p.Actions = append(p.Actions, publishAction(a.info.Name, tag, a.sha, draft, false))
			vals.Released, vals.URL = true, url
			if writeOutputs {
				if err := output.Write(dest, vals, stdout); err != nil {
					return fmt.Errorf("write outputs: %w", err)
				}
				p.Actions = append(p.Actions, "wrote outputs to "+destName)
			}
			if err := updateChangelog(a.cfg.ChangelogFile, tag, p); err != nil {
				return err
			}
			return emit()
		},
	}
	f := cmd.Flags()
	f.StringVar(&bump, "bump", "auto", "version bump: auto, major, minor, or patch")
	f.StringVar(&explicit, "version", "", "explicit version, e.g. 1.4.0 or v1.4.0")
	f.StringVar(&prerelease, "prerelease", "", "prerelease identifier, e.g. rc or beta")
	f.BoolVar(&draft, "draft", false, "create a draft release (GitHub only)")
	f.BoolVar(&dryRun, "dry-run", false, "print the plan without publishing or writing files")
	f.BoolVar(&asJSON, "json", false, "print the plan as JSON")
	f.StringVar(&changelogFile, "changelog-file", "", "prepend the release notes to this file")
	f.StringVar(&outputFile, "output-file", "", "append CI outputs to this file (default: detected from CI)")
	return cmd
}

func changelogCmd(analyzeFor analyzer, stdout io.Writer) *cobra.Command {
	var explicit, prerelease string
	cmd := &cobra.Command{
		Use:   "changelog",
		Short: "Print release notes for unreleased commits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a, err := analyzeFor(cmd, request{prerelease: prerelease})
			if err != nil {
				return err
			}
			heading, compareTo := "Unreleased", short(a.sha, 7)
			if explicit != "" {
				v, err := a.scheme.Normalize(explicit)
				if err != nil {
					return err
				}
				heading = a.scheme.Format(v)
				compareTo = heading
			}
			md, _, _ := a.render(heading, compareTo)
			_, err = io.WriteString(stdout, md)
			return err
		},
	}
	cmd.Flags().StringVar(&explicit, "version", "", "version to use as the heading instead of Unreleased")
	cmd.Flags().StringVar(&prerelease, "prerelease", "", "cover commits since the latest tag of any kind")
	return cmd
}

func nextVersionCmd(analyzeFor analyzer, stdout io.Writer, logger *log.Logger) *cobra.Command {
	var bump, prerelease string
	cmd := &cobra.Command{
		Use:   "next-version",
		Short: "Print the next tag, or nothing when there is nothing to release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validBump(bump); err != nil {
				return err
			}
			a, err := analyzeFor(cmd, request{bump: bump, prerelease: prerelease, needVersion: true})
			if err != nil {
				return err
			}
			if !a.release {
				logger.Println("nothing to release")
				return nil
			}
			_, err = fmt.Fprintln(stdout, a.result.Tag)
			return err
		},
	}
	cmd.Flags().StringVar(&bump, "bump", "auto", "version bump: auto, major, minor, or patch")
	cmd.Flags().StringVar(&prerelease, "prerelease", "", "prerelease identifier, e.g. rc or beta")
	return cmd
}

// updateChangelog prepends the plan's notes to path unless the file already
// has a heading for tag, so retries after a partial run do not duplicate it.
func updateChangelog(path, tag string, p *Plan) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("update changelog: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ["+tag+"]") || strings.HasPrefix(line, "## "+tag+" ") {
			p.Actions = append(p.Actions, path+" already contains "+tag)
			return nil
		}
	}
	if err := output.PrependChangelog(path, p.Notes); err != nil {
		return fmt.Errorf("update changelog: %w", err)
	}
	p.Actions = append(p.Actions, "prepended "+path)
	return nil
}
