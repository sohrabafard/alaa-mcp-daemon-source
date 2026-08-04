# Contributing

Thank you for improving `alaa-mcp-daemon`. Small, well-evidenced changes are easier to review and safer for a local process supervisor.

## Choose the right path

- Report a reproducible, non-security defect with the bug-report template.
- Propose a focused workflow improvement with the feature-request template.
- Ask for troubleshooting help through [support](./SUPPORT.md).
- Report vulnerabilities only through the [security policy](./alaa-mcp-daemon/SECURITY.md); do not include them in public issues or pull requests.

## Before you start

Read the product [README](./alaa-mcp-daemon/README.md), [architecture](./alaa-mcp-daemon/docs/architecture.md), and [security model](./alaa-mcp-daemon/SECURITY.md). Do not include real credentials, private paths, project names, or user data in issues, fixtures, or examples.

Install the repository hook once per clone; it commits staged UTF-8 text as LF without a BOM and never stages unrelated working-tree changes:

```powershell
.\scripts\install-git-hooks.ps1
```

## Development loop

```powershell
cd .\alaa-mcp-daemon
go test ./...
go vet ./...
go build ./...
pwsh -File .\scripts\verify.ps1
```

The last command is the Windows release gate and requires Go, a C compiler for the race detector, and `govulncheck`. Do not run `-RunSchedulerIntegration` unless you explicitly accept its real Task Scheduler create/remove side effect.

The standard gate exits with `0` only when every requested check passes; `2` means a required tool or supported runtime was unavailable, not that the change is validated. The [validation guide](./alaa-mcp-daemon/docs/validation.md) explains the full gate and the opt-in scheduler check.

## Pull requests

- Keep each change focused and add or update tests for behavior changes.
- Preserve the daemon's local-only, no-implicit-shell, and managed-process-only boundaries.
- Update the operator documentation whenever commands, configuration, lifecycle, or troubleshooting changes.
- Explain validation performed and any deliberately unrun platform-specific checks.
- Keep examples and issue references sanitized: never add credentials, private paths, personal data, or unpublished security details.

For security-sensitive findings, follow [SECURITY.md](./alaa-mcp-daemon/SECURITY.md) instead of opening a public issue.
