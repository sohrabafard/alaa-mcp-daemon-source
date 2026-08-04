## Why

Describe the problem or workflow this change addresses.

## What changed

Summarize the behavior, documentation, or tooling change. Call out compatibility, configuration, lifecycle, or security-boundary implications.

## Validation

- [ ] Focused tests passed or this is documentation-only
- [ ] `go vet ./...` passed when Go changed
- [ ] `go build ./...` passed when Go changed
- [ ] `pwsh -File .\scripts\verify.ps1` passed from `alaa-mcp-daemon` on Windows, or the reason it was not run is stated below
- [ ] Documentation links and examples were checked when docs changed

Describe commands run and any intentionally unrun platform-specific checks:

## Safety

- [ ] No secrets, private paths, or personal data were added
- [ ] Windows-specific checks were run or their absence is explained
- [ ] Public behavior, configuration, and runbook documentation are aligned
- [ ] This pull request does not disclose a vulnerability; security-sensitive findings follow `alaa-mcp-daemon/SECURITY.md`
