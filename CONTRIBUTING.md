# Contributing to shiptag

Thanks for helping improve shiptag. Bug reports, fixes, and features are all welcome. Everyone taking part is expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Before you start

- **Questions:** ask in [Discussions](https://github.com/faridlamaul/shiptag/discussions/categories/q-a) rather than opening an issue.
- **Bugs:** open an issue with the bug report form. A `shiptag tag --dry-run` output and the CI platform make most bugs reproducible.
- **Features and larger changes:** open a feature request first so we can agree on the approach before you write code.
- **Security issues:** do not open a public issue; follow [SECURITY.md](SECURITY.md).

## Development setup

You need Go 1.26 or newer and git.

```bash
git clone https://github.com/faridlamaul/shiptag.git
cd shiptag
make build                  # bin/shiptag
make fmt-check vet test     # what CI runs on every pull request
```

| Path | Contents |
|---|---|
| `cmd/shiptag` | CLI wiring and end-to-end tests |
| `internal/commits` | Conventional Commit parsing |
| `internal/version` | Tag selection and bump rules |
| `internal/gitrepo` | Reading tags and commits with the git CLI |
| `internal/notes` | Release notes rendering |
| `internal/provider` | GitHub, GitLab, and Bitbucket publishers and CI detection |
| `internal/config`, `internal/output` | `.shiptag.yaml` and CI outputs |

## Tests

- Add or update tests with every change. Tests that need a repository create one with `internal/testutil/gitfixture`, which isolates them from your global git config.
- Provider tests use `httptest` servers; never call a real API.
- Golden files are regenerated with `go test ./internal/notes -update` and `go test ./cmd/shiptag -update`. Review the diff before committing it.

## Pull requests

Pull requests are squash-merged, and **the PR title becomes the commit message** that shiptag reads to decide the next version. The title must be a [Conventional Commit](https://www.conventionalcommits.org/):

| Title | Effect |
|---|---|
| `feat(cli): add --path flag` | minor release |
| `fix(gitlab): escape subgroup paths` | patch release |
| `feat!: drop the -v shorthand` | major release (minor while shiptag is 0.x) |
| `docs: …`, `chore: …`, `ci: …`, `test: …`, `refactor: …` | no release |

A breaking change also needs a `BREAKING CHANGE:` paragraph in the PR description explaining what users must change.

Every pull request must pass the `Test`, `Build`, `Docker`, and `PR title` checks. Keep each pull request focused on one change, and update the README when behavior or flags change.

## Releases

Maintainers do not tag releases by hand. Every merge to `main` runs shiptag on this repository, which tags the version, publishes the release notes, attaches the binaries, and pushes the container image.
