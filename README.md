# ⚡ shiptag

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A lightweight, high-performance Semantic Versioning and release CLI tool built specifically for **GitHub** and **GitHub Actions**.

It analyzes Conventional Commits, calculates the next semantic version tag, generates structured Markdown release notes, and publishes GitHub Releases automatically.

---

## ✨ Features

- **Automated SemVer Bumping:** Automatically determines the next version (`major`, `minor`, or `patch`) based on Conventional Commits:
  - `BREAKING CHANGE:` or `type!:` -> **Major** bump (`v1.0.0` -> `v2.0.0`)
  - `feat:` -> **Minor** bump (`v1.0.0` -> `v1.1.0`)
  - `fix:`, `perf:`, `refactor:`, `docs:`, etc. -> **Patch** bump (`v1.0.0` -> `v1.0.1`)
- **Categorized Release Notes:** Groups commits into clean Markdown sections:
  - 💥 Breaking Changes
  - 🚀 Features
  - 🐛 Bug Fixes
  - ⚡ Performance Improvements
  - ♻️ Refactoring
  - 📝 Documentation
  - 🔧 Build System & CI
  - 🧹 Chores & Maintenance
- **GitHub PR & Commit Linking:** Extracts squash commit PR numbers (e.g. `(#123)`) and generates clickable links to GitHub Pull Requests, Commits, and Diff Comparers.
- **GitHub Native:** Uses GitHub REST API with standard `GITHUB_TOKEN` authentication.
- **Dry-Run Mode:** Preview upcoming version tags and release notes without making remote changes.

---

## 🚀 Installation

### Using `go install`

```bash
go install github.com/faridlamaul/shiptag/cmd/shiptag@latest
```

### Build from Source

```bash
git clone https://github.com/faridlamaul/shiptag.git
cd shiptag
make install
```

### Docker (GHCR)

Pull the pre-built container image from GitHub Container Registry:

```bash
docker pull ghcr.io/faridlamaul/shiptag:latest

# Run tag command inside container
docker run --rm -e GITHUB_TOKEN=$GITHUB_TOKEN ghcr.io/faridlamaul/shiptag:latest tag --repo owner/repo
```

---

## 📖 CLI Commands

### 1. `tag` — Automated Semantic Release

Calculates the next version, creates the git tag, and publishes the GitHub Release:

```bash
# Auto-detect bump from Conventional Commits
shiptag tag --repo owner/repo --token $GITHUB_TOKEN

# Manual bump override
shiptag tag --repo owner/repo --bump minor

# Dry-run preview
shiptag tag --repo owner/repo --dry-run
```

### 2. `changelog` — Generate Release Notes

Prints release notes for unreleased commits to stdout:

```bash
shiptag changelog --repo owner/repo
```

### 3. `next-version` — Predict Next Version

Outputs only the next semantic version string (useful in shell scripts and CI pipelines):

```bash
NEXT_VER=$(shiptag next-version --repo owner/repo)
echo "Upcoming version: ${NEXT_VER}"
```

---

## 🤖 GitHub Actions Integration

Use `shiptag` directly in your GitHub Actions workflow:

```yaml
name: Release

on:
  push:
    branches:
      - main

permissions:
  contents: write

jobs:
  semantic-release:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Pull shiptag container image
        run: docker pull ghcr.io/faridlamaul/shiptag:latest

      - name: Run Semantic Release
        run: |
          docker run --rm \
            -e GITHUB_TOKEN="${{ secrets.GITHUB_TOKEN }}" \
            -e GITHUB_REPOSITORY="${{ github.repository }}" \
            -e GITHUB_SHA="${{ github.sha }}" \
            -v "${{ github.workspace }}:/workspace" \
            -w /workspace \
            ghcr.io/faridlamaul/shiptag:latest tag
```

---

## 📄 License

MIT License © 2026 Ahmad Lamaul Farid.
