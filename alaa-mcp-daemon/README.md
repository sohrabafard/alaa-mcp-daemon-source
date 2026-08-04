# alaa-mcp-daemon

`alaa-mcp-daemon` is a Windows-first local supervisor for MCP servers and similar long-running developer tools. It launches, probes, restarts, reloads, logs, and stops declared processes; Claude Code and Codex connect directly to each managed HTTP endpoint.

It does not proxy MCP traffic, edit client configuration, discover projects, allocate ports, adopt existing processes, or expose remote management.

## Quick start

Build from this directory:

```powershell
pwsh -File .\scripts\build.ps1
```

Copy a configuration example, replace every `REPLACE_WITH_...` value with an absolute local path, then validate and run it:

```powershell
Copy-Item .\examples\minimal.example.json .\dist\alaa-mcp-daemon.json
.\dist\alaa-mcp-daemon.exe validate --config .\dist\alaa-mcp-daemon.json
.\dist\alaa-mcp-daemon.exe run --config .\dist\alaa-mcp-daemon.json
```

Use the managed endpoint (for example, `http://127.0.0.1:<port>/mcp`) in the relevant Claude Code or Codex project configuration; the daemon never writes that client configuration.

## Operate safely

`status`, `start`, `stop`, `restart`, `reload`, `logs`, and `shutdown` talk to the running daemon over local control IPC. Lifecycle requests are asynchronous, so use `status` to observe the resulting state.

When adding, reloading, or diagnosing another service, read the copy-paste [service operations runbook](./docs/operations/10-service-runbook.md) for the required validation and recovery steps. For current-user logon startup, read [Windows lifecycle](./docs/windows-lifecycle.md#autostart).

Treat the JSON configuration as executable authority: anyone who can change it can choose what the current user launches. Keep credentials out of `vars`, `args`, and command `env`; see [SECURITY.md](./SECURITY.md).

## Documentation

- [Architecture](./docs/architecture.md) — component ownership, supervision, and local control.
- [Configuration](./docs/configuration.md) — strict JSON rules, probes, restart policy, and reload effects.
- [Windows lifecycle](./docs/windows-lifecycle.md) — Job Objects, named-pipe control, and Task Scheduler.
- [Validation](./docs/validation.md) — native Windows gate and manual acceptance evidence.
- [Service operations runbook](./docs/operations/10-service-runbook.md) — add, reload, inspect, and troubleshoot services.

## Version 1 boundary

Included: Windows 11 x64, per-user Task Scheduler startup, strict JSON, hot reload, fixed claims, process/TCP/HTTP probes, bounded restarts, Job Objects, local named-pipe CLI, rotating logs, and status. GUI, Windows Service mode, remote control, secret-store integration, resource quotas, self-update, and MCP proxying are out of scope.
