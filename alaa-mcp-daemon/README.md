# alaa-mcp-daemon

`alaa-mcp-daemon` is a Windows-first local process supervisor for MCP servers and similar long-running developer tools. It does not proxy MCP traffic. Claude Code and Codex connect directly to each managed HTTP endpoint; the daemon owns only process launch, readiness, restart, logs, configuration reload, and termination.

Version 1 is designed for one current-user daemon, one configuration file, and multiple project-specific services. The initial example manages one shared Serena HTTP process per project.

## Operational guarantees

- Every managed Windows process is created suspended, assigned to its own Job Object, and then resumed. Closing the daemon's Job Object handle terminates the complete descendant process tree.
- Only declared process trees may be terminated. A foreign process occupying a claimed port is never killed; the service retries within its bounded restart budget and fails closed if the port stays occupied.
- Configuration is decoded with unknown-field and duplicate-key rejection. A malformed reload never replaces the last-known-good configuration.
- Command arguments are passed as an argument array. No implicit `cmd.exe`, PowerShell, or shell-string evaluation occurs.
- TCP claims and TCP/HTTP probes are restricted to loopback addresses.
- The local Windows control endpoint is a named pipe protected for the current user and LocalSystem; remote pipe clients are rejected.
- Service stdout and stderr are captured to rotating files. Environment values are not included in status or daemon logs.
- Manual `stop` creates a runtime hold. Config reloads do not restart that service until `start` is issued; the hold is cleared when the daemon itself restarts.

## Build

The project has no third-party Go module dependencies.

```powershell
pwsh -File .\scripts\build.ps1
```

The build output is written to `dist\` with:

```text
dist\
  alaa-mcp-daemon.exe
  alaa-mcp-daemon.schema.json
  alaa-mcp-daemon.example.json
```

The executable carries build metadata when `-Version`, `-Commit`, and `-Date` are supplied to the script.

## Configure Serena

1. Copy `examples\serena.windows.example.json` to `dist\alaa-mcp-daemon.json`.
2. Change its `$schema` value to `./alaa-mcp-daemon.schema.json`.
3. Replace every `REPLACE_WITH_...` project value with the corresponding absolute project path.
4. Prefer an absolute Serena executable path for Task Scheduler reliability. Obtain it with:

```powershell
(Get-Command serena).Source
```

5. Validate the complete configuration:

```powershell
.\dist\alaa-mcp-daemon.exe validate --config .\dist\alaa-mcp-daemon.json
```

The example assigns:

| Service | Endpoint | Serena context |
|---|---|---|
| `serena-auth` | `127.0.0.1:42100` | `ide` |
| `serena-comment` | `127.0.0.1:42101` | `ide` |
| `serena-content` | `127.0.0.1:42102` | `ide` |

Each service declares both a TCP claim and an exclusive project key. The TCP claim rejects duplicate ports. The exclusive key rejects a second configured Serena process for the same project.

The example uses `--open-web-dashboard false` so automatic startup does not open browser tabs. Change it deliberately when dashboard auto-opening is required.

## Run and install autostart

Run in the current terminal for initial validation:

```powershell
.\dist\alaa-mcp-daemon.exe run --config .\dist\alaa-mcp-daemon.json
```

Install the current executable as a per-user Task Scheduler logon task:

```powershell
.\dist\alaa-mcp-daemon.exe install-autostart --config .\dist\alaa-mcp-daemon.json
```

The command prints the exact task name. Start the installed task immediately without attaching the daemon to the current PowerShell process:

```powershell
Start-ScheduledTask -TaskName "<TASK_NAME_PRINTED_BY_INSTALL>"
```

Stop the daemon and all managed process trees:

```powershell
.\dist\alaa-mcp-daemon.exe shutdown --config .\dist\alaa-mcp-daemon.json
```

Start the same scheduled task again when needed. Remove autostart only after shutting the daemon down:

```powershell
.\dist\alaa-mcp-daemon.exe uninstall-autostart --config .\dist\alaa-mcp-daemon.json
```

`uninstall-autostart` removes the scheduled task; it does not terminate an already-running daemon.

## Connect Claude Code and Codex

Claude Code project `.mcp.json` for the auth project:

```json
{
  "mcpServers": {
    "serena": {
      "type": "http",
      "url": "http://127.0.0.1:42100/mcp",
      "timeout": 30
    }
  }
}
```

Codex project configuration for the same endpoint:

```toml
[mcp_servers.serena]
enabled = true
url = "http://127.0.0.1:42100/mcp"
startup_timeout_sec = 30.0
tool_timeout_sec = 120.0
```

Both clients share the same project-specific Serena process. Client configuration remains outside the daemon and is never edited by it.

## CLI

```text
alaa-mcp-daemon run [--config PATH]
alaa-mcp-daemon validate [--config PATH] [--json]
alaa-mcp-daemon status [--config PATH] [--json] [SERVICE]
alaa-mcp-daemon start|stop|restart [--config PATH] SERVICE
alaa-mcp-daemon reload [--config PATH]
alaa-mcp-daemon logs [--config PATH] [--follow] [--lines N] [--stream both|stdout|stderr] SERVICE
alaa-mcp-daemon install-autostart [--config PATH]
alaa-mcp-daemon uninstall-autostart [--config PATH]
alaa-mcp-daemon shutdown [--config PATH]
alaa-mcp-daemon version
```

When `--config` is omitted, the daemon uses `alaa-mcp-daemon.json` beside the executable.

Common operations:

```powershell
.\dist\alaa-mcp-daemon.exe status --json
.\dist\alaa-mcp-daemon.exe restart serena-auth
.\dist\alaa-mcp-daemon.exe stop serena-content
.\dist\alaa-mcp-daemon.exe start serena-content
.\dist\alaa-mcp-daemon.exe logs --follow --stream stderr serena-auth
.\dist\alaa-mcp-daemon.exe reload
```

Lifecycle commands are accepted asynchronously. Use `status` to observe the resulting state.

## Service states

```text
disabled -> stopped -> starting -> ready
                         |          |
                         |          +-> unhealthy -> stopping -> backoff
                         +-> blocked
                         +-> backoff -> starting
                         +-> failed
```

- `blocked`: a deterministic prerequisite failed, such as a missing executable or missing working directory. The daemon does not retry until `start`, `restart`, or a relevant config change.
- `failed`: the restart budget was exhausted or restart policy prohibited another attempt.
- `backoff`: the service failed and is waiting for its next permitted attempt. This includes a claimed port that may still be releasing from a previous process.
- `stopping`: completion waits for both root-process exit and process-backend cleanup, including declared-port release checks.

## Hot reload behavior

The daemon watches the config file by content hash, including atomic replacement. It debounces changes and retries a candidate read four times. A rejected candidate is not retried continuously; edit the file again or issue `reload` after correcting it.

| Change | Result |
|---|---|
| Add enabled autostart service | Stage it, then start it |
| Remove service | Stop its complete process tree, then remove it |
| `enabled: false` | Stop it and publish `disabled` |
| `autostart: false` | Stop it unless it was manually started |
| Program, args, cwd, env, priority, claim, or exclusive key | Restart only that service |
| Readiness, restart, shutdown, enabled, or autostart policy | Update supervision; restart only when required by desired state |
| Description or log policy | Update without process restart |
| Invalid JSON, duplicate key, unknown field, or semantic conflict | Retain last-known-good configuration |
| `runtime.log_dir` or `runtime.state_dir` change | Reject reload; restart the daemon with the new paths |

Relative paths are resolved against the configuration file's directory, never the caller's current directory.

## Configuration rules

- Variables use `${name}` and are expanded exactly once.
- A service variable may not reference another variable.
- Missing, malformed, or unresolved variables reject the full config.
- Reserved variables are `service_id`, `config_dir`, `log_dir`, and `state_dir`.
- Service IDs and command names must match `^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`.
- Environment keys are compared case-insensitively because Windows environment names are case-insensitive.
- Duplicate TCP claims and duplicate exclusive keys reject the full config.
- HTTP probes allow `GET` or `HEAD`, do not follow redirects, and require an explicitly allowed response status.
- An explicit zero shutdown grace period is valid and forces immediate termination. Explicit zero values for positive timing fields are rejected rather than interpreted as defaults.

See `docs/configuration.md` for the complete field contract.

## Logs and state

Default layout relative to the config file:

```text
logs\
  daemon.jsonl
  <service-id>\
    stdout.log
    stderr.log
state\
  autostart-<config-key>.xml
```

The daemon log is JSON Lines. Service streams remain raw so original tool output is preserved. Rotated service logs use `.1`, `.2`, and subsequent numeric suffixes.

## Security boundary

Treat the JSON config as executable authority. Anyone who can modify it can choose which local executable the current user launches. Protect the config and executable with current-user write permissions.

Do not place credentials in `vars`, `args`, or command-level `env`. Version 1 intentionally has no secret-store integration and inherits the daemon user's environment before applying declared overrides.

The daemon never:

- proxies or inspects MCP messages;
- edits Serena, Claude Code, or Codex configuration;
- kills a foreign process to reclaim a port;
- scans Serena's registered-project list;
- executes through an implicit shell;
- exposes a network management endpoint;
- adopts a process it did not launch.

## Verification

Run the repository gate on Windows:

```powershell
pwsh -File .\scripts\verify.ps1
```

Run the opt-in Task Scheduler create/remove integration test only in an interactive test account:

```powershell
pwsh -File .\scripts\verify.ps1 -RunSchedulerIntegration
```

The default Windows suite executes real Job Object descendant cleanup, named-pipe control, mutex, config, supervisor, reload, logging, and CLI tests. The scheduler integration test is opt-in because it creates and removes a real per-user scheduled task.

See `docs/validation.md` for gate semantics and `docs/windows-lifecycle.md` for the Windows process model.

## Version 1 boundaries

Included: Windows 11 x64, per-user Task Scheduler startup, strict JSON, hot reload, reusable commands, explicit variables, fixed claims, process/TCP/HTTP probes, restart budgets, Job Objects, named-pipe CLI, rotating logs, and machine-readable status.

Excluded: GUI or tray UI, MCP proxying, automatic port allocation, client-config editing, Serena project discovery, Windows Service mode, remote control, dependency graphs, resource quotas, self-update, credential-store integration, and adoption of pre-existing processes.
