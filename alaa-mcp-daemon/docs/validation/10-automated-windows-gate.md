# Automated Windows gate

`scripts/verify.ps1` checks Go formatting, `go test ./...`, `go vet ./...`, `go test -race ./...`, `govulncheck ./...`, and a Windows amd64 production build. It requires Windows, Go, `govulncheck`, `CGO_ENABLED=1`, and the configured C compiler.

The normal suite covers Job Object descendant cleanup, named-pipe control and shutdown, single-instance mutex behavior, UTF-16LE Task Scheduler XML, strict configuration decoding, last-known-good reloads, staged additions, manual holds, replacement port release, bounded restarts, log rotation, watcher behavior, and CLI output.

The opt-in Scheduler gate creates and removes a real per-user task:

```powershell
pwsh -File .\scripts\verify.ps1 -RunSchedulerIntegration
```

Run it only in an interactive test account. If interrupted, remove any remaining task with `uninstall-autostart` using the same configuration path or with Task Scheduler.
