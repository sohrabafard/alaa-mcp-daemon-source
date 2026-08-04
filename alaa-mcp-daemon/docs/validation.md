# Validation gates

## Exit contract

Repository verification scripts use:

- exit `0`: every requested gate ran and passed;
- exit `1`: at least one gate ran and found a failure;
- exit `2`: a required tool or supported runtime was unavailable, or a requested side-effecting gate could not run.

Exit `2` is not a pass.

## Windows development gate

```powershell
pwsh -File .\scripts\verify.ps1
```

The script runs:

```text
gofmt cleanliness
go test ./...
go vet ./...
go test -race ./...
govulncheck ./...
Windows amd64 production build
```

On Windows, the normal test suite includes:

- Job Object descendant cleanup;
- concurrent Windows launches with per-child stdin/stdout/stderr handle isolation;
- named-pipe request/response and shutdown;
- deterministic control admission, completed-response, shutdown-order, and stalled-read tests;
- repeated Windows named-pipe round trips covering start, status, and shutdown;
- single-instance mutex release;
- UTF-16LE Task Scheduler XML generation (including the BOM and matching declaration);
- strict config decoding and conflict rejection;
- last-known-good reload retention;
- staged-addition failure retention;
- manual stop holds;
- old-port release gating before replacement;
- restart-budget exhaustion;
- transient foreign-port recovery and permanent foreign-port retry exhaustion with zero launches;
- deterministic preflight blocking without retry;
- process-tree termination;
- rotating logs;
- watcher debounce/retry behavior;
- CLI validation output.
- app executable-adjacent default-config and explicit `--config` override tests.

## Task Scheduler release gate

The scheduler gate has a real working-tree-external effect and is opt-in:

```powershell
pwsh -File .\scripts\verify.ps1 -RunSchedulerIntegration
```

It creates a uniquely keyed per-user scheduled task and removes it before the test returns. A test interruption may leave the task behind; remove it with `uninstall-autostart` using the same config path or through Task Scheduler.

This gate runs the real `schtasks.exe` create/remove integration, not only XML parsing. It should
be run on Windows when validating Task Scheduler compatibility.

## Non-Windows core gate

The non-Windows backend exists to test platform-neutral orchestration and process-group cleanup in CI. It is not the declared v1 production target.

```bash
./scripts/verify.sh
```

A non-Windows pass does not replace the native Windows gate.

## Manual acceptance sequence

After the automated Windows gate passes:

1. Build to a permanent directory.
2. Validate the deployment config.
3. Run the daemon interactively and confirm every enabled service reaches `ready` or a specific `blocked`/`failed` state.
4. Connect Claude Code and Codex directly to one shared Serena endpoint.
5. Change one service port and confirm only that service restarts.
6. Save invalid JSON and confirm running services remain unchanged while `last_reload_error` is populated.
7. Correct the file and issue `reload`; confirm generation advances.
8. Stop one service and confirm its manual hold survives a metadata-only config edit.
9. Start it and confirm readiness.
10. Briefly occupy an unused configured port, release it during backoff, and confirm the service reaches `ready` automatically without a manual `start`.
11. Keep that port occupied in a separate run and confirm the service exhausts its bounded retry budget without launching or disturbing the foreign owner.
12. Install autostart, launch the printed scheduled task, and close the initiating PowerShell window.
13. Confirm the daemon and services remain running.
14. Run `shutdown` and confirm Serena and its language-server descendants disappear.
15. Run the scheduled task again, then terminate the daemon process externally in a disposable test session and confirm Job Object descendants disappear.

Record observed command output and process evidence. Do not replace an unrun Windows scenario with an expected result.
