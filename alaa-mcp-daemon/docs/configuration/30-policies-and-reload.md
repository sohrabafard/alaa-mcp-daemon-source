# Policies and reload behavior

Restart policy defaults to `always`, with `max_attempts: 5`, `window: 60s`, `backoff_initial: 1s`, and `backoff_max: 30s`. `on_failure` skips clean-exit restarts; `never` does not restart. Startup and health failures still count toward the budget.

Shutdown `grace_period` defaults to `8s` and permits zero through five minutes. Log rotation defaults to 10 MiB with five backups; a single oversized write remains intact.

Changing program, args, cwd, env, priority, claims, or exclusive keys restarts that service. Changing enabled, autostart, readiness, restart, or shutdown policy updates supervision. Changing description or log policy updates metadata without a process restart.

The watcher reads a candidate up to four times to tolerate atomic writes. Invalid JSON or semantic conflicts retain the last-known-good configuration and are retried only after another file change or explicit `reload`.

Missing executables and working directories become `blocked` without repeated attempts. An occupied claim is retryable; persistent occupancy exhausts the budget and becomes `failed` without affecting the foreign process.
