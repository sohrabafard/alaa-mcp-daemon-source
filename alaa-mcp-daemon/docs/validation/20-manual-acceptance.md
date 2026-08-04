# Manual acceptance

After the automated Windows gate, build into a permanent directory, validate the deployment configuration, and run the daemon interactively. Confirm each enabled service reaches `ready` or has a specific `blocked` or `failed` cause.

Connect each client directly to its shared managed endpoint. Change one service's port and confirm only that service restarts. Save invalid JSON and confirm the running services remain unchanged while `last_reload_error` is populated; correct it and issue `reload` to advance the generation.

Stop a service, make a metadata-only edit, and confirm its hold remains. Start it and confirm readiness. Exercise both a port released during backoff and a persistent foreign port, recording that the first recovers and the second exhausts its retry budget without launching or disturbing the foreign owner.

Install autostart, run the printed task, close the initiating terminal, and confirm the daemon persists. Run `shutdown` and confirm managed descendants disappear. In a disposable session, externally terminate the daemon and confirm Job Object descendants disappear. Record observed command and process evidence.
