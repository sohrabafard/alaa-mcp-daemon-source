# Windows lifecycle model

## Launch sequence

For each service, the process backend performs this order:

1. Resolve and stat the executable.
2. Create inheritable stdout/stderr pipes and a `NUL` stdin handle.
3. Create an unnamed Job Object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`.
4. Call `CreateProcessW` with the root process suspended and in a new process group. The
   `STARTUPINFOEX` handle list limits inheritance to that child's stdin, stdout, and stderr;
   unrelated inheritable handles from concurrently starting siblings are not propagated.
5. Assign the suspended root process to the Job Object.
6. Apply the configured Windows priority class.
7. Resume the root thread.
8. Close parent-side inherited handles and start asynchronous stream copying.
9. Wait for the root process and retain its exit code.

Assignment before resume prevents the root process from spawning a descendant outside daemon ownership during the launch race.

## Descendant ownership

Descendants created by a process already in the Job Object join that job under the supported Windows model. The daemon keeps the only job handle it creates. When the daemon exits unexpectedly, Windows closes that handle and applies kill-on-close to remaining members.

The source includes a Windows-native integration test that launches a managed test root, makes it spawn a child, terminates the service, and proves the descendant also exits.

## Stop sequence

1. Mark the stop intentional so restart policy cannot race it.
2. Cancel readiness, health, backoff, and pending launch admission.
3. Release any startup-concurrency token.
4. Attempt `GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT)` for the process group.
5. Wait `grace_period` only when Windows accepted the signal.
6. Call `TerminateJobObject` when the process tree remains.
7. Wait for root-process completion.
8. Confirm every declared TCP claim can be rebound.
9. Publish the target stopped/disabled state or launch the intended replacement.

Ctrl-Break is best effort because a Task Scheduler process may not share a console. Job Object termination is the authoritative fallback.

## Daemon crash

A daemon crash closes operating-system handles. Kill-on-close then terminates the managed Job Objects. The process backend does not rely on a shutdown hook for this property.

## Local control endpoint

The Windows CLI uses a named pipe keyed by the canonical config path. The pipe security descriptor grants generic-all access to:

- the current user's SID;
- LocalSystem.

The pipe uses `PIPE_REJECT_REMOTE_CLIENTS`. One request and one response are exchanged per
connection. Completed responses close the pipe handle directly; no separate
`DisconnectNamedPipe` call is required.

Connection admission and shutdown snapshots are serialized with the server mutex and tracked by
the server `WaitGroup`. A shutdown response is completed and its connection is closed before the
shutdown callback cancels the server context. Closing a tracked handle is idempotent, so a stalled
read is unblocked by the same handle close used during shutdown.

## Single-instance boundary

A named `Local\` mutex keyed by canonical config path prevents two daemon processes in the same user session from supervising the same config.

## Task Scheduler

`install-autostart` registers a current-user logon task with:

- `InteractiveToken` logon type;
- least privilege;
- multiple-instance policy `IgnoreNew`;
- no execution time limit;
- start-when-available;
- bounded task-level restart after daemon failure;
- executable working directory set to the executable's directory.

The task action is an absolute executable path plus `run --config <absolute-path>`. The daemon's own mutex remains the final duplicate-instance gate.

The registration XML is emitted as UTF-16 little-endian with an `FF FE` BOM and an XML
declaration of `encoding="UTF-16"`, preserving Unicode executable and configuration paths for
Task Scheduler.

## Native validation requirement

Cross-compilation proves Windows source compatibility but not Win32 behavior. Release validation must run the Windows tests on Windows. The opt-in scheduler test additionally creates and removes a real per-user task and therefore requires explicit operator authorization through the test environment switch.
