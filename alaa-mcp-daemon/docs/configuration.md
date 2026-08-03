# Configuration contract

## File-level rules

- Encoding: JSON.
- Maximum size: 1 MiB.
- Supported `version`: `1`.
- Unknown fields, duplicate object keys, trailing JSON values, duplicate service IDs, duplicate TCP claims, and duplicate exclusive keys reject the complete file.
- Relative paths resolve against the directory containing the config file.
- `runtime.log_dir` and `runtime.state_dir` cannot change during hot reload.

## Runtime

| Field | Default | Validation |
|---|---:|---|
| `log_dir` | `./logs` | Non-empty path |
| `state_dir` | `./state` | Non-empty path |
| `max_parallel_starts` | `2` | 1 through 32 |
| `watch_interval` | `1s` | 100 ms through 1 minute |
| `reload_debounce` | `250ms` | 10 ms through 10 seconds |

## Command templates

A command template has:

- `program`: required executable name or path.
- `args`: ordered argument array.
- `cwd`: working directory; defaults to the config directory.
- `env`: environment overrides merged case-insensitively with the daemon environment.
- `priority`: `idle`, `below_normal`, `normal`, `above_normal`, or `high`; default `normal`.

A relative program containing a path separator resolves against the config directory. A bare program name is resolved from the daemon process `PATH`. For Task Scheduler deployments, prefer an absolute executable path.

There is no shell interpolation. Operators who deliberately require a shell must declare that shell as `program` and each shell argument explicitly; the daemon does not add one.

## Variables

Service `vars` are plain strings. Expansion syntax is `${name}`.

Rules:

- Expansion occurs exactly once.
- A variable value may not contain another `${...}` reference.
- Missing or malformed references reject the full config.
- NUL characters are rejected.
- Service variables may not override `service_id`, `config_dir`, `log_dir`, or `state_dir`.

Variable expansion is available in program, args, cwd, command env values, claims, exclusive keys, and readiness fields.

## Services

| Field | Default | Effect |
|---|---:|---|
| `enabled` | `true` | `false` forces `disabled` and suppresses launch |
| `autostart` | `true` | Launch after daemon startup unless manually held |
| `description` | empty | Status metadata only |
| `command` | required | Selects one command template |
| `vars` | empty | Supplies template values |
| `claims` | empty | Declares loopback endpoints that must be free before launch |
| `exclusive_keys` | empty | Declares semantic single-owner resources |
| `readiness` | process probe | Controls startup and runtime health |

IDs and command names must match:

```text
^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$
```

## TCP claims

A claim requires:

```json
{
  "type": "tcp",
  "host": "127.0.0.1",
  "port": "${port}"
}
```

Hosts must be `localhost` or a literal IPv4/IPv6 loopback address. `localhost` normalizes to `127.0.0.1`. Ports must expand to an integer from 1 through 65535.

A claim is a safety declaration, not a reservation. The daemon briefly binds it during preflight, closes the check socket, then starts the process. The managed process remains responsible for binding its endpoint.

## Exclusive keys

Exclusive keys are non-empty expanded strings compared case-insensitively. Use them for semantic ownership that cannot be inferred from command arguments, for example:

```json
"exclusive_keys": ["serena-project:${project_path}"]
```

Use canonical absolute project paths when the key represents a filesystem object.

## Probes

Shared defaults:

| Field | Default | Validation |
|---|---:|---|
| `type` | `process` | `process`, `tcp`, or `http` |
| `startup_timeout` | `30s` | Positive, at most 30 minutes |
| `interval` | `10s` | 100 ms through 10 minutes |
| `timeout` | `2s` | Positive and no greater than `interval` |
| `failure_threshold` | `3` | 1 through 100 |

TCP probe fields are `host` and `port`, both loopback-only.

HTTP probe fields are:

- `url`: absolute `http` or `https` loopback URL without user information.
- `method`: `GET` or `HEAD`; default `GET`.
- `allowed_statuses`: default `200` and `204`.

HTTP redirects are not followed.

## Restart policy

| Field | Default | Validation |
|---|---:|---|
| `policy` | `always` | `always`, `on_failure`, or `never` |
| `max_attempts` | `5` | 1 through 1000 |
| `window` | `60s` | Positive, at most 24 hours |
| `backoff_initial` | `1s` | Positive, at most 10 minutes |
| `backoff_max` | `30s` | At least initial, at most 1 hour |

`always` restarts after any unexpected exit. `on_failure` restarts only a non-zero unexpected exit, while startup and health failures still count as failures. `never` transitions startup/health failures to `failed` and does not restart an exited process.

## Shutdown policy

`grace_period` defaults to `8s` and accepts zero through 5 minutes. On Windows, the daemon attempts Ctrl-Break only when Windows accepts the signal. It then waits the grace period and force-terminates the Job Object if the process tree remains.

## Log policy

| Field | Default | Validation |
|---|---:|---|
| `max_bytes` | 10 MiB | 64 KiB through 10 GiB |
| `backups` | `5` | 1 through 100 |

The active log is rotated before a write that would cross the size threshold, except that a single write larger than the limit is written intact.

## Reload classification

Process restart fields:

- program
- args
- cwd
- env
- priority
- TCP claims
- exclusive keys

Supervision fields:

- enabled
- autostart
- readiness
- restart policy
- shutdown policy

Metadata fields:

- description
- log policy

## Failure behavior

Config parsing and semantic validation have no retry budget: one invalid candidate is read up to four times to tolerate atomic-write windows, then rejected. It is retried only after another file change or explicit `reload`.

A managed service uses its declared restart budget. A deterministic preflight failure transitions directly to `blocked` without consuming repeated restart attempts.
