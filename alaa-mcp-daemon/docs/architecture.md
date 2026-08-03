# Architecture

## Purpose

`alaa-mcp-daemon` is a control plane for local long-running tools. It owns process lifecycle and exposes no MCP data path.

```text
Claude Code ----\
                 +--> http://127.0.0.1:<project-port>/mcp --> managed MCP process
Codex ----------/

alaa-mcp-daemon.json --> reconciler --> supervisor --> operating-system process tree
                                      --> logs
                                      --> readiness and health probes
CLI --> local control transport ------/
```

## Components

| Package | Responsibility |
|---|---|
| `internal/config` | Strict decoding, normalization, variable expansion, conflict detection, semantic hashes, and config diffing |
| `internal/manager` | Last-known-good config ownership, generation tracking, staged additions, removals, and per-service reconciliation |
| `internal/supervisor` | One serialized state machine per service, restart windows, backoff, manual holds, readiness, and stop coordination |
| `internal/process` | Windows Job Object process backend and non-Windows process-group test fallback |
| `internal/control` | Current-user local IPC, request framing, protocol validation, and command dispatch |
| `internal/watcher` | Content-hash polling, debounce, bounded rereads, and explicit rejection reporting |
| `internal/logging` | Daemon JSONL logging and bounded service-stream rotation |
| `internal/autostart` | Explicit per-user Task Scheduler registration and removal |
| `internal/instance` | One daemon instance per canonical config path |

## Ownership invariants

1. A service supervisor owns exactly one managed root process at a time.
2. Every Windows root process is assigned to a service-specific Job Object before it is resumed.
3. The daemon stops only handles created by its process backend.
4. A declared TCP claim is unique across the accepted config.
5. An exclusive key is unique case-insensitively across the accepted config.
6. The accepted config pointer, generation, and hash change only after reconciliation succeeds.
7. A malformed reload cannot mutate the accepted config or remove an existing service.
8. Added supervisors are staged disabled before any old service is removed.
9. A replacement process does not launch until root exit and stop-backend cleanup are both observed.
10. A manual stop hold survives config reload and is cleared only by manual start/restart or daemon restart.

## Reconciliation

The manager computes one change class for every service ID:

- `add`
- `remove`
- `process_restart`
- `supervision`
- `metadata`
- `none`

Application order is fixed:

1. Validate and normalize the entire candidate config.
2. Stage all additions in a disabled state and open their log sinks.
3. Stop and remove deleted services.
4. Apply changes to existing services.
5. Activate staged additions.
6. Commit the candidate config, generation, and hash.

A runtime directory change is rejected because it changes control-plane ownership; restart the daemon with the new config instead.

## Service state machine

```text
Disabled
Stopped
Starting
Ready
Unhealthy
Backoff
Stopping
Blocked
Failed
```

The supervisor goroutine serializes every command and process/probe event. No state transition is performed from worker goroutines.

`Blocked` is fail-closed: no process is launched when a deterministic prerequisite fails. `Failed` is also fail-closed: no new process is launched after the retry budget is exhausted. Manual `start` or `restart` resets the budget and retries.

## Startup concurrency

A shared dynamically sized limiter bounds services between launch admission and readiness completion. A token is released when the service becomes ready, fails startup, exits, or is intentionally stopped. Reducing the configured limit does not terminate starts already admitted; it blocks subsequent admissions until usage is below the new limit.

## Readiness and health

The same configured probe serves two phases:

- Startup: repeatedly checked until success or `startup_timeout`.
- Runtime health: checked every `interval`; consecutive failures trigger a supervised restart at `failure_threshold`.

The process probe proves only that the root process has not exited. The TCP probe proves only that a loopback connection succeeds. The HTTP probe proves only that a loopback request returns an allowed status.

## Restart budget

Each failure is recorded in a rolling window. Backoff doubles from `backoff_initial` to `backoff_max`. Reaching `max_attempts` transitions to `Failed`. Intentional process restarts caused by config or CLI commands do not consume the failure budget.

## Stop completion

An intentional stop is complete only after both conditions hold:

1. The managed root process has exited.
2. The process backend has completed graceful/forced cleanup and declared-port release checks.

This prevents a replacement from racing the old process tree or inheriting a misleading `ready` state while the previous endpoint is still occupied.

## Control protocol

The protocol is one newline-delimited JSON request and one newline-delimited JSON response per connection. Messages are limited to 1 MiB and reject unknown fields and trailing JSON values. Protocol version 1 is explicit in both directions.

## Success and blocked stops

Daemon success stop: the control listener is closed, every supervisor reaches its terminal closed state, all managed process trees are gone, and logs are closed.

Daemon blocked stop: the shutdown context expires or a managed process tree does not terminate. The daemon returns a non-zero error and reports the affected service; it never reports completion merely because a stop was requested.
