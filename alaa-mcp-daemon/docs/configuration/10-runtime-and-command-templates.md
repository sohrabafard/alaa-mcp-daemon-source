# Runtime and command templates

`runtime.log_dir` defaults to `./logs`, `runtime.state_dir` to `./state`, `max_parallel_starts` to `2`, `watch_interval` to `1s`, and `reload_debounce` to `250ms`. Start concurrency must be 1–32; watcher values must stay within their configured validation ranges.

A command template supplies `program`, ordered `args`, optional `cwd`, `env`, and `priority`. `cwd` defaults to the configuration directory; a program containing a path separator resolves from that directory, while a bare program uses the daemon `PATH`.

`priority` is one of `idle`, `below_normal`, `normal`, `above_normal`, or `high`; the default is `normal`. For Task Scheduler reliability, use an absolute executable path.

There is no shell interpolation. If a service requires a shell, declare that shell as `program` with every argument explicitly.

Service `vars` expand `${name}` exactly once in program, args, cwd, command env values, claims, exclusive keys, and readiness fields. Missing, malformed, nested, or reserved-name overrides reject the complete configuration. Reserved names are `service_id`, `config_dir`, `log_dir`, and `state_dir`.
