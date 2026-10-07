# Service operations runbook

Use this procedure to add or change one local service without disturbing the accepted configuration. Run commands from the daemon project directory and replace only the obvious placeholder paths and service values.

## Add and load a service

1. Back up the configuration through your normal local-change workflow; do not edit the running JSON blindly.
2. Copy the service shape from [minimal.example.json](../../examples/minimal.example.json). Choose a unique `id`, loopback `host` and `port`, and an exclusive key when another service could represent the same local resource.
3. Use an absolute `program` path for Scheduler use, an absolute working directory, ordered `args`, and a readiness probe that proves the endpoint you need.
4. Validate before reload:

```powershell
.\dist\alaa-mcp-daemon.exe validate --config .\dist\alaa-mcp-daemon.json
.\dist\alaa-mcp-daemon.exe reload --config .\dist\alaa-mcp-daemon.json
.\dist\alaa-mcp-daemon.exe status --config .\dist\alaa-mcp-daemon.json <SERVICE_ID>
```

5. If `autostart` is `false`, explicitly start the service:

```powershell
.\dist\alaa-mcp-daemon.exe start --config .\dist\alaa-mcp-daemon.json <SERVICE_ID>
```

Wait for `ready` before pointing a client at the managed endpoint.

## Serena dashboard startup

For multiple Serena services with dashboards enabled, set `runtime.max_parallel_starts` to `1`, as in the Serena example. Serena selects a free dashboard port before binding it; simultaneous starts can select the same port. The daemon holds a start slot until the configured readiness probe completes, so a TCP probe on each Serena MCP endpoint serializes these starts through MCP readiness.

The dashboard port is separate from the configured MCP port and may change after a restart. Read the latest `Serena web dashboard started at` message in each service's stderr log, then verify that URL and its active project. MCP TCP readiness alone does not prove dashboard availability. If a collision has already occurred, apply the start limit and restart the affected Serena services; a reload of the limit alone does not restart existing processes.

## Reload and troubleshoot

Invalid candidates retain the last-known-good configuration. Inspect `status` for `last_reload_error`, correct the JSON, validate, and issue `reload` again.

For a service problem, inspect state and stderr without exposing environment values:

```powershell
.\dist\alaa-mcp-daemon.exe status --config .\dist\alaa-mcp-daemon.json <SERVICE_ID>
.\dist\alaa-mcp-daemon.exe logs --config .\dist\alaa-mcp-daemon.json --follow --stream stderr <SERVICE_ID>
```

`blocked` means a deterministic prerequisite such as executable or working directory failure; correct it, then `start` or `restart`. `backoff` means a retry is pending. `failed` means the restart budget is exhausted or restarting is prohibited; correct the cause, then `start` or `restart`.

Do not kill a foreign process to reclaim a claimed port. Release it through its owner or choose a new port, then let the configured retry path or an explicit `start` recover the service.
