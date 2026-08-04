#!/usr/bin/env sh
set -eu

root=$(git rev-parse --show-toplevel)
cd "$root"
command -v python3 >/dev/null 2>&1 || command -v python >/dev/null 2>&1 || {
  printf '%s\n' 'Python 3 is required by the repository pre-commit hook.' >&2
  exit 1
}
git config --local core.hooksPath .githooks
printf '%s\n' 'Installed repository hooks from .githooks.'
