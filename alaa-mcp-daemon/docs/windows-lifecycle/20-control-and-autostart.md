# Control and autostart

The Windows CLI connects to a named pipe keyed by the canonical configuration path. It admits the current user's SID and LocalSystem, rejects remote clients, and exchanges one request and response per connection. A `Local\` mutex keyed by that path prevents two daemons in one user session from supervising the same configuration.

`install-autostart` creates a current-user logon task with `InteractiveToken`, least privilege, `IgnoreNew` multiple-instance policy, no execution limit, start-when-available, and bounded task-level restart. Its action uses an absolute executable path and `run --config <absolute-path>`.

Task XML is UTF-16 little-endian with an `FF FE` BOM and `encoding="UTF-16"`, preserving Unicode paths. The mutex remains the final duplicate-instance guard.

Use the [Scheduler integration gate](../validation/10-automated-windows-gate.md) to prove real create/remove behavior on Windows.
