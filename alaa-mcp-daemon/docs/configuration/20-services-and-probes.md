# Services, claims, and probes

Each service selects a command and may set `enabled`, `autostart`, `description`, `vars`, `claims`, `exclusive_keys`, and `readiness`. IDs and command names match `^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`.

TCP claims and TCP/HTTP probes accept loopback hosts only. A TCP claim is a safety check, not a port reservation: the daemon checks the endpoint before launch, but the managed process binds it.

Use an exclusive key for a semantic single owner, such as a canonical project path. Keys are non-empty after expansion and compare case-insensitively.

The default readiness probe is `process`. TCP checks a loopback connection. HTTP uses a loopback `http` or `https` URL without user information, permits `GET` or `HEAD`, does not follow redirects, and defaults to allowed statuses `200` and `204`.

Shared probe defaults are a `30s` startup timeout, `10s` interval, `2s` timeout, and threshold `3`. The configured probe is used both while starting and for runtime health.
