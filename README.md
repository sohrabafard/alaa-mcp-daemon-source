# alaa-mcp-daemon

A Windows-first local process supervisor for MCP servers and other long-running developer tools. It keeps the data path direct—Claude Code and Codex connect to the managed server themselves—while the daemon handles start, readiness, bounded restart, logs, reload, and shutdown.

## Why it exists

Running one shared MCP server per project should not mean orphaned processes, conflicting ports, fragile shell launchers, or a browser tab on every sign-in. `alaa-mcp-daemon` provides a small, local control plane with explicit ownership boundaries.

## Highlights

- Windows Job Objects terminate managed process trees, including descendants.
- Strict JSON configuration rejects duplicate keys, unknown fields, duplicate TCP claims, and duplicate exclusive keys.
- A current-user Windows named pipe controls the daemon; no remote management listener exists.
- Hot reload retains the last-known-good configuration when a candidate is invalid.
- Readiness probes, retry budgets, rotating service logs, and per-user Task Scheduler autostart are built in.

## Quick start

```powershell
cd .\alaa-mcp-daemon
pwsh -File .\scripts\build.ps1
Copy-Item .\examples\serena.windows.example.json .\dist\alaa-mcp-daemon.json
.\dist\alaa-mcp-daemon.exe validate --config .\dist\alaa-mcp-daemon.json
.\dist\alaa-mcp-daemon.exe run --config .\dist\alaa-mcp-daemon.json
```

Replace each `REPLACE_WITH_...` value before validation. For adding a second service or reloading safely, read the [configuration guide](./alaa-mcp-daemon/docs/configuration.md); use the [Windows lifecycle guide](./alaa-mcp-daemon/docs/windows-lifecycle.md) and [validation guide](./alaa-mcp-daemon/docs/validation.md) when diagnosing process behavior or a failed gate.

Before relying on a configuration for daily use, run `pwsh -File .\scripts\verify.ps1` from `alaa-mcp-daemon`. The declared v1 production target is Windows 11 x64; non-Windows core coverage does not replace the native Windows validation gate.

## Documentation

- [Product and development guide](./alaa-mcp-daemon/README.md)
- [Architecture](./alaa-mcp-daemon/docs/architecture.md), [configuration](./alaa-mcp-daemon/docs/configuration.md), and [Windows lifecycle](./alaa-mcp-daemon/docs/windows-lifecycle.md)
- [Validation](./alaa-mcp-daemon/docs/validation.md) and [security model](./alaa-mcp-daemon/SECURITY.md)
- [Contributing](./CONTRIBUTING.md), [support](./SUPPORT.md), and [code of conduct](./CODE_OF_CONDUCT.md)

## Community

Use the issue templates for reproducible, non-security defects and focused workflow proposals. Start with [support](./SUPPORT.md) to choose the right channel, then read [contributing](./CONTRIBUTING.md) before preparing a change. All participation follows the [Code of Conduct](./CODE_OF_CONDUCT.md).

Do not place credentials, private paths, user data, or vulnerability details in a public issue. Security-sensitive reports belong in the [security policy](./alaa-mcp-daemon/SECURITY.md).

## Project status and license

The v1 production target is Windows 11 x64; non-Windows support exists for core test coverage, not as a production promise. This repository is licensed under [Apache-2.0](./LICENSE).
