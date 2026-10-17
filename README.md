# shiptag

[![Release](https://github.com/faridlamaul/shiptag/actions/workflows/release.yml/badge.svg)](https://github.com/faridlamaul/shiptag/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

shiptag reads Conventional Commits from your git history, calculates the next semantic version, writes categorized release notes, and publishes the release to **GitHub**, **GitLab**, or **Bitbucket Cloud**. The platform is detected automatically in CI.

- **One tool, three platforms:** GitHub (and Enterprise Server), GitLab (and self-managed), Bitbucket Cloud.
- **Local analysis:** tags and commits come from the clone, so results are the same everywhere and `--dry-run` needs no token.
- **Safe to re-run:** a commit that is already released is never released twice.
- **CI-ready outputs:** `version`, `tag`, `released`, and more for the next steps of your pipeline.

## Platforms

| | GitHub | GitLab | Bitbucket Cloud |
|---|---|---|---|
| Publishes | GitHub Release (draft, prerelease) | GitLab Release | Annotated tag carrying the notes |
| Token | `GITHUB_TOKEN` or `GH_TOKEN` | `GITLAB_TOKEN`, else the built-in `CI_JOB_TOKEN` | `BITBUCKET_TOKEN` |
| Permission | `contents: write` | Developer role (`api` scope for personal or project tokens) | Repository access token with `repository:write` |
| PR/MR links from | `(#N)`, `Merge pull request #N` | `(!N)`, `See merge request …!N` | `(pull request #N)` |

`--token` overrides the environment on every platform.

## Installation

```bash
docker pull ghcr.io/faridlamaul/shiptag:latest                 # Docker
go install github.com/faridlamaul/shiptag/cmd/shiptag@latest  # Go 1.26+
```

Prebuilt binaries for Linux, macOS, and Windows (amd64 and arm64) are attached to every [GitHub Release](https://github.com/faridlamaul/shiptag/releases) together with `checksums.txt`. To build from source, run `make build`.

Image tags follow the release: `X.Y.Z`, `X.Y`, `X`, and `latest` (the newest stable release). Each image is built for `linux/amd64` and `linux/arm64` with provenance and an SBOM.

## Quick start

shiptag needs the full git history; shallow clones are rejected.

```bash
shiptag tag --dry-run        # preview the version, the reasons, and the notes
GITHUB_TOKEN=... shiptag tag # publish
```

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
…
```

With Docker, mount the repository and keep file ownership:

```bash
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/workspace" -w /workspace \
  ghcr.io/faridlamaul/shiptag:latest tag --dry-run
```

## CI setup

Every pipeline needs a **full clone** and a **token** (see [Platforms](#platforms)). Pin the image to a release tag (for example `ghcr.io/faridlamaul/shiptag:0.1`) instead of `latest` once you depend on it.

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

      - id: shiptag
        uses: faridlamaul/shiptag@v0

      - if: steps.shiptag.outputs.released == 'true'
        run: echo "Released ${{ steps.shiptag.outputs.tag }}"
```

| Input | Default | Description |
|---|---|---|
| `command` | `tag` | `tag`, `changelog`, or `next-version` |
| `args` | | Extra flags separated by spaces, e.g. `--dry-run` or `--tag-prefix api/ --path services/api` |
| `token` | `github.token` | Token used to publish the release |

The step exposes every [output](#outputs-and-exit-codes). `@v0` follows the newest `0.x` release; pin `@v0.3.0` or a commit SHA for a fixed version. The action builds shiptag from source on Linux runners, which adds about 30 seconds; for a faster step, run the image directly with `uses: docker://ghcr.io/faridlamaul/shiptag:<version>`, `with: { args: tag }`, and `GITHUB_TOKEN` in `env`.

### GitLab CI

```yaml
release:
  image:
    name: ghcr.io/faridlamaul/shiptag:latest
    entrypoint: [""]
  variables:
    GIT_DEPTH: 0
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  script:
    - shiptag tag
  artifacts:
    reports:
      dotenv: shiptag.env
```

The built-in `CI_JOB_TOKEN` is enough when the pipeline user has the Developer role. For protected tags, set a masked `GITLAB_TOKEN` variable instead.

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

Add `BITBUCKET_TOKEN` as a secured repository variable.

## Usage

| Command | Purpose |
|---|---|
| `shiptag tag` | Compute the next version, publish the release, write CI outputs |
| `shiptag changelog` | Print release notes for unreleased commits |
| `shiptag next-version` | Print the next tag, or nothing when there is nothing to release |

| Flag | Applies to | Description |
|---|---|---|
| `--dry-run` | `tag` | Print the plan; publish and write nothing |
| `--json` | `tag` | Print the plan (or the result) as JSON |
| `--bump auto\|major\|minor\|patch` | `tag`, `next-version` | Force the bump level (default `auto`) |
| `--version <x.y.z>` | `tag`, `changelog` | Use an explicit version |
| `--prerelease <id>` | all | Create `-<id>.N` prereleases, e.g. `rc` |
| `--draft` | `tag` | Create a draft release (GitHub only) |
| `--changelog-file <path>` | `tag` | Prepend the notes to a file, e.g. `CHANGELOG.md` |
| `--output-file <path>` | `tag` | Write CI outputs to this file |
| `--tag-prefix <prefix>` | all | Tag prefix for monorepos, e.g. `api/` |
| `--path <dir>` | all | Count only commits touching this path |
| `--provider`, `--repo`, `--ref` | all | Override detection: platform, `owner/repo`, commit to release |
| `--base-url`, `--api-url` | all | Self-hosted web and API URLs |
| `--token`, `--config`, `--dir` | all | Token, config file, repository directory |

Run `shiptag <command> --help` for defaults. Flags override `.shiptag.yaml`, which overrides the defaults.

## How versions are calculated

| Commit | Bump |
|---|---|
| `type!:` or a `BREAKING CHANGE:` footer | major |
| `feat` | minor |
| `fix`, `perf`, `revert` | patch |
| anything else | none: nothing to release |

- **First release:** `initial_version` (default `0.1.0`).
- **0.x:** while the version is `0.y.z`, a breaking change bumps minor. Graduate with `--bump major` or `--version 1.0.0`.
- **Prereleases:** `--prerelease rc` gives `v1.3.0-rc.1`, `-rc.2`, …; the stable `v1.3.0` later covers every commit since the last stable tag.
- **Monorepos:** `--tag-prefix api/ --path services/api` gives each package its own tags and history.
- **Re-runs:** an already tagged commit prints `nothing to release` (or `already released` when the tag exists only on the platform) and exits 0. A tag pointing to a different commit is an error.

Only tags reachable from the released commit count. Merge commits are left out of the notes, but their PR/MR number is linked to the commits they merged.

## Release notes

```markdown
## [v1.3.0](https://github.com/owner/repo/compare/v1.2.0...v1.3.0) (2026-10-17)

### ⚠️ Breaking Changes

- **api:** move config to shiptag.yaml ([#42](…)) ([`a1b2c3d`](…))

  The old config file is no longer read; rename it to shiptag.yaml.

### ✨ Features

- **cli:** add --path flag ([#40](…)) ([`d4e5f6a`](…)), closes [#38](…)

### ❤️ Contributors

Alice, @bob, Carol
```

A breaking change keeps its commit subject as the list item, and the text of its `BREAKING CHANGE:` footer appears as a paragraph below it. The notes are plain Markdown, so they render the same on GitHub, GitLab, and Bitbucket.

Visible by default: Features, Bug Fixes, Performance Improvements, Reverts, Code Refactoring, Documentation. Build, CI, chore, style, test, and non-conventional commits are hidden. Bots are left out of the contributors. Every heading can be renamed in the configuration, for example to drop the emoji.

## Configuration

`.shiptag.yaml` at the repository root is optional. All keys and their defaults:

```yaml
tag_prefix: ""              # e.g. "api/"
v_prefix: true              # tags look like v1.2.3
initial_version: 0.1.0
initial_development: true   # 0.x: breaking changes bump minor
path: ""                    # count only commits touching this path

release_rules:              # merged over the defaults; major | minor | patch | none
  feat: minor
  fix: patch
  perf: patch
  revert: patch

breaking_title: "⚠️ Breaking Changes"
sections:                   # replaces the default list when set
  - { title: ✨ Features, types: [feat] }
  - { title: 🐛 Bug Fixes, types: [fix] }
  - { title: 🔧 Chores, types: [chore], hidden: true }
other_section: { title: 📝 Other Changes, hidden: true }
contributors_title: "❤️ Contributors"

exclude_authors: ["[bot]", dependabot, renovate]
changelog_file: ""          # e.g. CHANGELOG.md (prepended, not committed)
```

Unknown keys and invalid values are reported with the key name.

## Outputs and exit codes

| Key | Value |
|---|---|
| `released` | `true` when a release was published |
| `version`, `tag` | New version (`1.3.0`) and tag (`v1.3.0`) |
| `previous_version` | Version the notes start from; empty for a first release |
| `sha`, `url` | Released commit and release URL |
| `bump` | `major`, `minor`, `patch`, `initial`, or `explicit` |

Outputs go to `$GITHUB_OUTPUT` on GitHub, `shiptag.env` on GitLab and Bitbucket, and stdout elsewhere (`--output-file` overrides this).

shiptag exits `0` when it released, when there is nothing to release, or when the release already exists, and `1` on any error (for example a shallow clone or a missing token).

## Contributing

Contributions are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md). The PR title must be a Conventional Commit, because it becomes the commit that shiptag releases from. To report a vulnerability, follow [SECURITY.md](SECURITY.md).

## License

MIT License © 2026 Ahmad Lamaul Farid.
