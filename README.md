# shiptag

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

shiptag analyzes Conventional Commits in your local git clone, calculates the next semantic version, renders categorized release notes, and publishes the release to **GitHub**, **GitLab**, or **Bitbucket Cloud**. One binary, one command set, and the platform is detected automatically in CI.

## Features

- **Multi-provider:** GitHub (including Enterprise Server), GitLab (including self-managed), and Bitbucket Cloud.
- **Local-git analysis:** tags and commits come from the clone, so results are identical on every platform and dry-run needs no token or network.
- **Bump rules:** `feat` bumps minor, `fix`/`perf`/`revert` bump patch, breaking changes bump major. Docs, chore, and CI-only pushes release nothing by default.
- **Categorized notes with links:** sections per commit type, links to PRs/MRs, issues, commits, and the compare view, plus a contributors list.
- **Dry-run and JSON plan:** see the version, the reasons, and the notes before anything is published.
- **CI outputs:** `version`, `tag`, `released`, and more are written to `$GITHUB_OUTPUT` or `shiptag.env`.
- **Config file:** optional `.shiptag.yaml`; it works with zero configuration.

## Provider support

| | GitHub | GitLab | Bitbucket Cloud |
|---|---|---|---|
| Hosts | github.com, GitHub Enterprise Server | gitlab.com, self-managed | bitbucket.org |
| What "release" means | GitHub Release (draft and prerelease supported) | GitLab Release | Annotated tag whose message carries the notes (Bitbucket has no Releases feature) |
| Change request links | `(#N)` | `(!N)` | `(pull request #N)` |
| `--draft` | Yes | No | No |

## Installation

```bash
# Go
go install github.com/faridlamaul/shiptag/cmd/shiptag@latest

# From source
git clone https://github.com/faridlamaul/shiptag.git
cd shiptag
make build        # writes bin/shiptag
make install      # copies it to ~/.local/bin

# Docker (runs as a non-root user; contains git and ca-certificates only)
docker pull ghcr.io/faridlamaul/shiptag:latest
```

## Quick start

shiptag needs the **full git history**; shallow clones are rejected. Preview a release without a token:

```bash
shiptag tag --dry-run
```

Real output from this repository:

```text
Provider:     github (faridlamaul/shiptag)
Ref:          HEAD → 04b5c26a…
Previous tag: (none)
Next tag:     v0.1.0 (initial)
Reason:       initial ← no previous version tag (initial_version 0.1.0)
Commits:      23 counted, 2 hidden
Actions:
  - would create release v0.1.0 at 04b5c26 (draft=false)
  - would write outputs to stdout

--- Release notes ---
## v0.1.0 (2026-10-17)

### ⚠ BREAKING CHANGES
…
```

Publish it:

```bash
export GITHUB_TOKEN=...   # see "Tokens and permissions"
shiptag tag
```

## Commands and flags

```text
shiptag [command]

  tag           Compute the next version, publish the release, and write CI outputs
  changelog     Print release notes for unreleased commits
  next-version  Print the next tag, or nothing when there is nothing to release
```

Global flags (all commands):

```text
      --api-url string      API base URL (default: derived from the web base URL)
      --base-url string     web base URL for self-hosted instances
      --config string       config file (default: .shiptag.yaml in the repository root)
      --dir string          repository directory (default ".")
      --path string         count only commits touching this path
      --provider string     hosting provider: github, gitlab, or bitbucket (default: detected)
      --ref string          commit to release (default: the CI commit SHA, else HEAD)
      --repo string         repository path, e.g. owner/repo (default: from CI or the origin remote)
      --tag-prefix string   version tag prefix, e.g. api/
      --token string        API token (default: from the provider's environment variables)
```

`shiptag tag` flags:

```text
      --bump string             version bump: auto, major, minor, or patch (default "auto")
      --changelog-file string   prepend the release notes to this file
      --draft                   create a draft release (GitHub only)
      --dry-run                 print the plan without publishing or writing files
      --json                    print the plan as JSON
      --output-file string      append CI outputs to this file (default: detected from CI)
      --prerelease string       prerelease identifier, e.g. rc or beta
      --version string          explicit version, e.g. 1.4.0 or v1.4.0
```

`shiptag changelog` flags:

```text
      --prerelease string   cover commits since the latest tag of any kind
      --version string      version to use as the heading instead of Unreleased
```

`shiptag next-version` flags:

```text
      --bump string         version bump: auto, major, minor, or patch (default "auto")
      --prerelease string   prerelease identifier, e.g. rc or beta
```

Notes:

- Provider detection order: `--provider`, then CI environment variables, then the `origin` remote host.
- `--base-url` sets the web URL of a self-hosted instance; the API URL is derived (`<web>/api/v3` for GitHub Enterprise, `<web>/api/v4` for GitLab). `--api-url` overrides it directly.
- Flags override the config file, which overrides defaults.
- `--json` prints the plan (and, without `--dry-run`, what was done) as one JSON object. Logs go to stderr.

## Versioning rules

Each counted commit maps to a level; the highest wins.

| Commit | Bump |
|---|---|
| `!` after the type/scope, or a `BREAKING CHANGE:` footer | major |
| `feat` | minor |
| `fix`, `perf`, `revert` | patch |
| Every other type, and non-conventional commits | none |

- **Nothing to release:** if the highest level is none, shiptag prints `nothing to release`, writes `released=false`, and exits 0.
- **0.x rule:** while the stable base is `0.y.z`, a major bump becomes minor (`initial_development: true`). Use `--bump major` or `--version 1.0.0` to graduate to 1.0.0.
- **First release:** with no reachable stable tag, the version is `initial_version` (default `0.1.0`), regardless of commit types. `--version` and `--bump` still win.
- **Overrides:** `--bump` forces the level (without the 0.x rule). `--version` sets the version; it must be valid SemVer, not an existing tag, and greater than the stable base.
- **Prereleases:** `--prerelease rc` computes the stable core from commits since the last stable tag, then numbers `-rc.N` after the highest existing `rc` tag of that core (any branch). Prerelease notes cover commits since the latest tag of any kind; the later stable release covers everything since the last stable tag. Releases are flagged as prerelease on GitHub.
- **Tag selection:** only tags reachable from the release commit count. Tags match `<tag_prefix>v?MAJOR.MINOR.PATCH[-PRERELEASE]`.
- **Monorepo:** combine `--tag-prefix` and `--path` so each package has its own tags and history:

  ```bash
  shiptag tag --tag-prefix api/ --path services/api
  ```

- **Idempotent:** re-running on the same commit prints `already released` and exits 0. If the tag exists at a different commit, shiptag fails.

## Release notes format

```markdown
## [v1.3.0](<compare-url>) (2026-10-17)

### ⚠ BREAKING CHANGES

- **api:** <breaking note> ([#42](<pr-url>)) ([a1b2c3d](<commit-url>))

### Features

- **cli:** add --path flag ([#40](<pr-url>)) ([d4e5f6a](<commit-url>)), closes [#38](<issue-url>)

### Contributors

Alice, @bob, Carol
```

- Default sections: Features, Bug Fixes, Performance Improvements, Reverts, Code Refactoring, Documentation. Build, CI, chore, style, test, and non-conventional commits are hidden by default; titles, order, and visibility are configurable.
- If every commit is hidden, the body is `_No user-facing changes._`
- Contributors come from commit authors and `Co-authored-by` trailers. Bots are excluded (`exclude_authors`). On GitHub, noreply emails render as `@login`.

## CI setup

All platforms need a full clone, and shiptag needs the platform's variables. See "Tokens and permissions" for credentials.

### GitHub Actions

```yaml
name: Release
on:
  push:
    branches: [main]

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Release
        id: shiptag
        run: |
          go install github.com/faridlamaul/shiptag/cmd/shiptag@latest
          shiptag tag
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Use the version
        if: steps.shiptag.outputs.released == 'true'
        run: echo "Released ${{ steps.shiptag.outputs.version }}"
```

shiptag appends `released`, `version`, `previous_version`, `tag`, `sha`, `bump`, and `url` to `$GITHUB_OUTPUT`.

To use the Docker image instead of `go install`, mount the workspace, run as the runner's user so the output file is writable, and pass the CI variables through:

```yaml
      - name: Release
        id: shiptag
        run: |
          docker run --rm --user "$(id -u):$(id -g)" \
            -v "$PWD:/workspace" -w /workspace \
            -e GITHUB_ACTIONS -e GITHUB_REPOSITORY -e GITHUB_SHA \
            -e GITHUB_TOKEN -e GITHUB_API_URL -e GITHUB_SERVER_URL \
            ghcr.io/faridlamaul/shiptag:latest tag --output-file shiptag.env
          cat shiptag.env >> "$GITHUB_OUTPUT"
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### GitLab CI

```yaml
variables:
  GIT_DEPTH: 0

release:
  stage: deploy
  image:
    name: ghcr.io/faridlamaul/shiptag:latest
    entrypoint: [""]
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  script:
    - shiptag tag
  artifacts:
    reports:
      dotenv: shiptag.env
```

`shiptag.env` exposes `version`, `tag`, and the other outputs to later jobs via `needs`. The built-in `CI_JOB_TOKEN` is used automatically; it works when the job's user has at least the Developer role. For protected tags or stricter setups, set a masked `GITLAB_TOKEN` variable with the `api` scope. When running the image directly with `docker run`, pass `GITLAB_CI`, `CI_PROJECT_PATH`, `CI_COMMIT_SHA`, `CI_API_V4_URL`, `CI_SERVER_URL`, `CI_JOB_TOKEN`, and `GITLAB_TOKEN`.

### Bitbucket Pipelines

```yaml
clone:
  depth: full

pipelines:
  branches:
    main:
      - step:
          name: Release
          image: ghcr.io/faridlamaul/shiptag:latest
          script:
            - shiptag tag
          artifacts:
            - shiptag.env
```

Add `BITBUCKET_TOKEN` as a secured repository variable (a repository access token with `repository:write`). When running the image directly with `docker run`, pass `BITBUCKET_BUILD_NUMBER`, `BITBUCKET_REPO_FULL_NAME`, `BITBUCKET_COMMIT`, and `BITBUCKET_TOKEN`.

### Running the image elsewhere

The image's entrypoint is `shiptag`, and it runs as a non-root user (uid 10001) with `safe.directory '*'` configured, so mounted workspaces owned by any user work:

```bash
docker run --rm -v "$PWD:/workspace" -w /workspace ghcr.io/faridlamaul/shiptag:latest tag --dry-run
```

## Tokens and permissions

| Provider | Token (checked in order) | Required permission |
|---|---|---|
| GitHub | `--token`, `GITHUB_TOKEN`, `GH_TOKEN` | Workflow `permissions: contents: write` |
| GitLab | `--token`, `GITLAB_TOKEN` (`PRIVATE-TOKEN` header), then `CI_JOB_TOKEN` (`JOB-TOKEN` header) | Developer role; PAT or project token needs the `api` scope |
| Bitbucket Cloud | `--token`, `BITBUCKET_TOKEN` (Bearer) | Repository access token with `repository:write` |

`--dry-run` needs no token. Without a token on a real run, shiptag names the expected variables for the detected provider.

## Configuration

Optional file `.shiptag.yaml` at the repository root (or `--config <path>`). Unknown fields and invalid values are errors that name the field.

```yaml
tag_prefix: ""              # e.g. "api/"
v_prefix: true              # new tags are v1.2.3
initial_version: 0.1.0
initial_development: true   # 0.x: breaking changes bump minor
path: ""                    # count only commits touching this path

release_rules:              # type -> major | minor | patch | none
  feat: minor
  fix: patch
  perf: patch
  revert: patch

breaking_title: "⚠ BREAKING CHANGES"
sections:                   # replaces the default list when set
  - { title: Features, types: [feat] }
  - { title: Bug Fixes, types: [fix] }
  - { title: Chores, types: [chore], hidden: true }
other_section: { title: Other Changes, hidden: true }

exclude_authors: ["[bot]", dependabot, renovate]
changelog_file: ""          # e.g. CHANGELOG.md
```

`release_rules` merges over the defaults; `sections` replaces them. With `changelog_file` (or `--changelog-file`), the notes are prepended to that file (created with a `# Changelog` heading if missing). The file is not committed; do that in your pipeline.

## Outputs

| Key | Meaning |
|---|---|
| `released` | `true` if a release was published, otherwise `false` |
| `version` | New version without prefix, e.g. `1.3.0` |
| `previous_version` | Previous stable version |
| `tag` | New tag, e.g. `v1.3.0` |
| `sha` | Commit that was released |
| `bump` | `major`, `minor`, `patch`, or `initial` |
| `url` | URL of the release |

| Environment | Destination |
|---|---|
| GitHub Actions | appended to `$GITHUB_OUTPUT` |
| GitLab CI | `shiptag.env` (use `artifacts:reports:dotenv`) |
| Bitbucket Pipelines | `shiptag.env` (share via `artifacts`) |
| Elsewhere | `KEY=VALUE` lines on stdout (suppressed with `--json`) |

`--output-file` overrides the destination.

## Exit codes

- `0`: released, nothing to release, already released, or dry-run.
- `1`: any error, including a shallow clone, a missing `git` binary, a missing token, or a tag that already points to another commit.

## Migrating from the previous CLI

- **Default trigger:** only `feat`, `fix`, `perf`, `revert`, and breaking commits create a release. Docs, chore, and CI-only commits no longer cause a patch release; use `release_rules` to change this.
- **`--ref` default:** the CI commit SHA, else `HEAD` (previously the default branch). Branch names are resolved to a full SHA.
- **Full clone required:** tags and commits are read from the local clone; shallow clones fail.
- **Notes format:** release notes use the new categorized layout described above.
- **Outputs:** written to `$GITHUB_OUTPUT` or `shiptag.env` instead of only stdout.
- **`-v` removed:** use `--version`.
- **`pkg/` removed:** code now lives under `internal/`; shiptag is a CLI, not a library.
- **Renamed:** the module is `github.com/faridlamaul/shiptag` and the image is `ghcr.io/faridlamaul/shiptag`.

## License

MIT License © 2026 Ahmad Lamaul Farid.
