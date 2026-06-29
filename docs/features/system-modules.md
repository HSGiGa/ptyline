# System Modules

System modules expose host metrics in the status bar without doing work during
rendering. They are built-in modules addressed by placeholders:

- `{load}`
- `{cpu}`
- `{memory}`
- `{battery}`
- `{disk}`

Each module performs startup discovery before it is scheduled. If the required
platform provider is unavailable at startup, the module is marked unavailable and
no refresh ticker is started. This is especially important for `{battery}` on
desktops, VMs, and WSL installs where no battery source exists.

## Startup Probe

System modules implement `status.ProbeModule`:

```go
type ProbeModule interface {
	Module
	Probe(ctx context.Context) ModuleProbe
}
```

Probe results:

- `Available`: schedule interval refresh and take an initial sample.
- `Unavailable`: hide the module and do not poll again.

Runtime sampling errors after a successful probe are different from startup
unavailability: they mark the snapshot stale or errored while preserving the last
good value when possible.

The default does not reprobe unavailable modules. A future `reprobe_ms` setting
may support hot-plug cases, but the default must remain disabled.

## Defaults

| Module | Default interval | Default timeout | Missing provider |
| --- | ---: | ---: | --- |
| `{load}` | 5000 ms | 100 ms | hidden, no polling |
| `{cpu}` | 2000 ms | 100 ms | hidden, no polling |
| `{memory}` | 5000 ms | 100 ms | hidden, no polling |
| `{battery}` | 30000 ms | 250 ms | hidden, no polling |
| `{disk}` | 60000 ms | 250 ms | hidden, no polling |

These defaults bias toward low wakeups. Users can opt into faster updates per
module with `interval_ms`.

## Platform Matrix

| Module | Linux | WSL | macOS | Windows |
| --- | --- | --- | --- | --- |
| `{load}` | `/proc/loadavg` | `/proc/loadavg` | `getloadavg` or `sysctl` | unavailable |
| `{cpu}` | `/proc/stat` delta | `/proc/stat` delta | host CPU statistics delta | performance counters |
| `{memory}` | `/proc/meminfo` | `/proc/meminfo` | VM statistics + total memory | `GlobalMemoryStatusEx` |
| `{battery}` | `/sys/class/power_supply` | sysfs if present; Windows interop later | IOKit power sources | `GetSystemPowerStatus` |
| `{disk}` | `statfs` | `statfs` | `statfs` | `GetDiskFreeSpaceExW` |

WSL is not a separate build target. It uses the Linux binary and may specialize
provider choice based on the runtime profile.

## Configuration

System modules use ordinary `[module.<id>]` config:

```toml
[module.cpu]
enabled = true
interval_ms = 2000
timeout_ms = 100
format = "cpu {percent}%"
```

The initial post-MVP preset can use:

```text
{cwd} {git} || {load} {cpu} {memory} {battery} || {time}
```

## Module Notes

### `{load}`

Shows the 1-minute load average by default. Linux/WSL reads `/proc/loadavg`.
Windows has no Unix-style load average and should hide this module by default.

### `{cpu}`

Shows total CPU utilization percentage. Providers must keep the previous sample
and compute deltas; the first sample may hide the module until a second sample
exists.

### `{memory}`

Shows memory usage percentage. Linux uses `MemTotal` and `MemAvailable`, not
`MemFree`, so caches are treated correctly.

### `{battery}`

Shows battery percentage and charging state. If no battery is found at startup,
the module is unavailable and is not checked repeatedly.

### `{disk}`

Shows filesystem usage. The default path is `cwd`, using the shell integration
current directory when available and falling back to `/` or the platform root.
