# Process and control safety

On Windows, every managed root process is created suspended, assigned to a service-specific Job Object, then resumed. Closing that Job Object handle terminates its process tree; the daemon only terminates handles it created.

The daemon never kills a foreign process that occupies a claimed port. A potentially transient collision follows the configured bounded restart policy; persistent occupancy becomes `failed` without launching or disturbing the foreign owner.

The local control protocol is one newline-delimited JSON request and response per connection, with protocol version `1`, a 1 MiB message limit, unknown-field rejection, and no trailing JSON value. Its Windows named pipe allows the current user and LocalSystem and rejects remote clients.

The configuration and command arrays are executable authority. There is no implicit `cmd.exe`, PowerShell, or shell-string evaluation. Service environment values are not included in status or daemon logs.

For the Windows shutdown order and Scheduler contract, read [Windows lifecycle](../windows-lifecycle.md).
