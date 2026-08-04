# Configuration

The daemon accepts one JSON configuration file (maximum 1 MiB) with `version: 1`, `runtime`, `commands`, and `services`. It rejects unknown fields, duplicate object keys, trailing JSON values, duplicate service IDs, duplicate TCP claims, and duplicate exclusive keys as a whole.

Relative paths resolve from the configuration file directory, not the caller's working directory. Validate before `run` or `reload`:

```powershell
.\dist\alaa-mcp-daemon.exe validate --config .\dist\alaa-mcp-daemon.json
```

Start with [minimal.example.json](../examples/minimal.example.json), then read [runtime and command templates](./configuration/10-runtime-and-command-templates.md), [services and probes](./configuration/20-services-and-probes.md), and [policies and reload behavior](./configuration/30-policies-and-reload.md).

For an operator procedure that adds a service without weakening the current configuration, read the [service operations runbook](./operations/10-service-runbook.md).
