#!/usr/bin/env bash
set -u

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

could_not_run() {
  printf 'could-not-run: %s\n' "$1" >&2
  exit 2
}

run_gate() {
  local name="$1"
  shift
  printf '==> %s\n' "$name"
  "$@" || {
    local status=$?
    printf 'finding: %s exited with code %s\n' "$name" "$status" >&2
    exit 1
  }
}

command -v go >/dev/null 2>&1 || could_not_run "go was not found in PATH"
command -v govulncheck >/dev/null 2>&1 || could_not_run "govulncheck was not found; install golang.org/x/vuln/cmd/govulncheck"

printf '==> gofmt cleanliness\n'
unformatted="$(gofmt -l .)" || exit 1
if [[ -n "$unformatted" ]]; then
  printf 'finding: unformatted Go files\n%s\n' "$unformatted" >&2
  exit 1
fi

run_gate "go test ./..." go test -count=1 ./...
run_gate "go vet ./..." go vet ./...
run_gate "go test -race ./..." go test -race -count=1 ./...
run_gate "govulncheck ./..." govulncheck ./...
run_gate "host build" go build -trimpath -o "${TMPDIR:-/tmp}/alaa-mcp-daemon-verify" ./cmd/alaa-mcp-daemon
rm -f "${TMPDIR:-/tmp}/alaa-mcp-daemon-verify"

printf 'clean: core non-Windows gates passed; native Windows release gate remains required\n'
