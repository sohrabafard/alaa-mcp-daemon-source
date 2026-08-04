# Windows lifecycle

The Windows backend owns only processes it starts. It uses per-service Job Objects so kill-on-close removes the managed descendant tree if the daemon exits.

For process creation, stop completion, and port-release sequencing, read [launch and stop](./windows-lifecycle/10-launch-and-stop.md). For local named-pipe access, single-instance scope, and per-user Task Scheduler startup, read [control and autostart](./windows-lifecycle/20-control-and-autostart.md).

## Autostart

Install with `install-autostart --config <path>` and start the printed task with `Start-ScheduledTask`. `uninstall-autostart` removes the task but does not stop an already-running daemon; run `shutdown` first.

Cross-compilation does not prove Win32 behavior. Use the native gate described in [validation](./validation.md).
