# Runtime ownership and reconciliation

`internal/config` strictly decodes, normalizes, expands variables, detects conflicts, hashes, and diffs configuration. `internal/manager` holds the accepted generation and stages additions before it removes old services.

`internal/supervisor` serializes one service state machine; `internal/process` owns the platform process backend; `internal/probe` provides readiness and health checks. `internal/watcher` polls the configuration hash, debounces changes, and reports rejected reloads. `internal/logging` writes daemon JSON Lines and rotates service streams.

The manager classifies service changes as `add`, `remove`, `process_restart`, `supervision`, `metadata`, or `none`. It validates the whole candidate, stages additions disabled, removes deleted services, applies existing changes, activates additions, then commits the new generation and hash.

`runtime.log_dir` and `runtime.state_dir` are control-plane ownership paths. A hot reload that changes either is rejected; restart the daemon with the new configuration.

Service states are `disabled`, `stopped`, `starting`, `ready`, `unhealthy`, `backoff`, `stopping`, `blocked`, and `failed`. A manual `stop` creates a hold that reload does not clear; `start`, `restart`, or daemon restart clears it.

For the exact field-to-effect mapping, read [reload behavior](../configuration/30-policies-and-reload.md).
