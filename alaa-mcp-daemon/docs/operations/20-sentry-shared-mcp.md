# Shared Sentry MCP

The daemon supervises `mcp-proxy`, which owns one Sentry MCP stdio process. Claude Code and Codex connect to the proxy's Streamable HTTP endpoint at `http://127.0.0.1:42107/mcp`. All clients share the upstream token, host, and skill grants. This reduces duplicate server processes; CPU savings still require measurement under the actual workload.

Use [sentry.windows.example.json](../../examples/sentry.windows.example.json) as an additive command/service configuration. Do not replace an existing daemon configuration with this standalone example. Preserve existing services and port assignments. The example is disabled until its prerequisites are ready.

## Prerequisites

Installation requires operator authorization. The integration targets `mcp-proxy@6.7.19` and the locally verified `@sentry/mcp-server@0.42.0`. The latter requires Node 22.13 or newer. Install the proxy once, then execute installed JavaScript files directly rather than using `npx` on every start:

```powershell
npm install -g mcp-proxy@6.7.19
npm root -g
```

Set `node_path` to the absolute `node.exe` path. Under the reported global npm root, set `proxy_script` to `mcp-proxy/dist/bin/mcp-proxy.mjs` and `sentry_script` to `@sentry/mcp-server/dist/index.js`. Verify the proxy's `--help` after installation before enabling it. The proxy creates downstream MCP server/session objects while sharing one upstream client; these objects are not additional Sentry processes. Client-initiated workflows work through the proxy, but upstream requests for client input such as sampling, roots, and elicitation are not relayed.

## Local credentials and upstream

Create `sentry.env` beside the active daemon configuration, normally inside the gitignored `dist/` directory:

```dotenv
SENTRY_ACCESS_TOKEN=REPLACE_WITH_LOCAL_TOKEN
```

Restrict access to the file to the daemon user and never commit or print its contents. Node's `--env-file` reads it before starting the proxy; the proxy passes its environment to the Sentry child. The daemon configuration contains no token. An existing `SENTRY_ACCESS_TOKEN` in the daemon's environment takes precedence over the file, so remove stale inherited values before restarting if necessary. The example does not create a real credential file.

Set `sentry_host` to the actual hostname and optional port, without a URL scheme. The supplied `--insecure-http` changes the Sentry API connection to plaintext HTTP. It does not disable HTTPS certificate validation. Remove this argument when using HTTPS. The proxy's own listener remains loopback-only HTTP regardless of the upstream protocol.

The command grants `MCP_SKILLS=inspect,seer`. Seer requires support in the self-hosted Sentry instance; granting a skill does not add the server-side feature. Use `inspect` alone when Seer is unavailable. In Sentry MCP 0.42.0 the accepted identifiers are `inspect`, `seer`, `docs`, `triage`, and `project-management`; `docs` is deprecated and documentation tools are included in `inspect`. Explicit grants avoid the CLI's broader implicit defaults. Token API scopes must also permit the selected tools.

## Clients

For Claude Code's project `.mcp.json`:

```json
{
  "mcpServers": {
    "sentry": {
      "type": "http",
      "url": "http://127.0.0.1:42107/mcp"
    }
  }
}
```

For Codex's project `.codex/config.toml`:

```toml
[mcp_servers.sentry]
url = "http://127.0.0.1:42107/mcp"
```

Replace the client's old Sentry stdio registration in its intended scope; leaving it registered would continue spawning extra processes. Sentry credentials and `MCP_SKILLS` belong on the managed server, not in HTTP client configuration.

## Activation and proof

After installing and verifying the proxy, setting the actual host, and creating the local credential file, set only the Sentry service's `enabled` and `autostart` fields to `true`. Validate the active configuration before activation; the daemon watches valid changes automatically. Use the existing [service runbook](10-service-runbook.md) for lifecycle commands.

The configured TCP probe proves a listener, not Sentry API access. Before declaring the integration operational, initialize two independent MCP clients, run `tools/list` from both, and confirm that they use one Sentry process. Then make one authorized read-only Sentry query to verify host/token/API compatibility. This repository configuration alone does not prove those live gates or measured CPU improvement.

Upstream references: [MCP Proxy](https://github.com/punkpeye/mcp-proxy), [Sentry MCP](https://github.com/getsentry/sentry-mcp), and [Sentry skill registry](https://github.com/getsentry/sentry-mcp/blob/main/packages/mcp-core/src/skills.ts).
