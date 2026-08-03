# Security model

## Trust boundary

The configuration file is executable authority for the current Windows user. A writer can select a local executable, arguments, working directory, and environment overrides. Restrict write access to the daemon executable, config file, log directory, and state directory.

## Local-only controls

- TCP claims and TCP/HTTP probes accept only loopback hosts.
- The management plane is a current-user Windows named pipe, not a TCP listener.
- Remote named-pipe clients are rejected.
- A foreign process occupying a declared port is never terminated.
- Command templates never receive an implicit shell.

The daemon cannot prove that a generic managed command binds only to loopback. That remains a command/config responsibility.

## Secrets

Do not store credentials in config variables, arguments, or command environment values. Version 1 has no credential-store integration. Environment values are not emitted by daemon status or structured logs, but a managed program may print its own values to stdout/stderr.

## Process containment

Managed Windows roots are assigned to kill-on-close Job Objects before resume. This controls descendants created within the supported Windows Job Object model. It does not sandbox filesystem, registry, network, or user-account access; managed programs run with the daemon user's authority.

## Reporting

Report a suspected vulnerability privately to the project owner with reproduction steps, affected commit, observed impact, and the smallest safe test case. Do not include live credentials or sensitive project data.
