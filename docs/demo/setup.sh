#!/usr/bin/env bash
# Creates a throwaway git repository used by demo.tape.
# Usage: docs/demo/setup.sh <dir>
set -euo pipefail

dir="${1:?usage: setup.sh <dir>}"
rm -rf "$dir"
mkdir -p "$dir"
cd "$dir"

git init -q -b main
git config user.name "Ada Lovelace"
git config user.email "ada@example.com"
git config commit.gpgsign false
git config tag.gpgsign false
git remote add origin https://github.com/acme/rocket.git

# Fixed dates keep the commit hashes and the notes date stable between runs.
export GIT_AUTHOR_DATE="2026-10-17T10:00:00Z" GIT_COMMITTER_DATE="2026-10-17T10:00:00Z"

commit() {
  echo "$1" >>CHANGES.txt
  git add CHANGES.txt
  git commit -q -m "$@"
}

commit "feat: launch the first rocket"
git tag -a v1.2.0 -m v1.2.0
commit "feat(cli): add --countdown flag (#41)"
commit "fix(engine): stop fuel leak on retry (#43)"
commit "perf: cache trajectory math"
commit "docs: explain launch windows"
commit "chore(deps): bump go to 1.27"
commit "feat(api)!: rename Launch to Ship (#45)" -m "BREAKING CHANGE: Launch() is now Ship(); update your callers."
