# Launch and stop

For each service, Windows resolves the executable, creates only that child's standard handles, creates a kill-on-close Job Object, creates the root suspended in a new process group, assigns the job, applies priority, then resumes the root thread. Assignment before resume closes the descendant-escape race.

An intentional stop marks the service non-restartable, cancels probes/backoff/admission, attempts Ctrl-Break when Windows accepts it, waits `grace_period`, and terminates the Job Object if needed. It then waits for the root exit and verifies every declared TCP claim can be rebound before publishing the target state or launching a replacement.

Ctrl-Break is best effort because a Task Scheduler process may not share a console; Job Object termination is the authoritative fallback.

If the daemon crashes, closing operating-system handles triggers kill-on-close for remaining job members. This does not rely on a shutdown hook.
