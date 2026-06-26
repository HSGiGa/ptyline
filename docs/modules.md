# Modules

Built-in modules are addressed by `{module_id}` placeholders in `bar.format` or
`[[bar.row]].format`. Rendering uses cached `ModuleSnapshot` values; modules do
not run work during redraw.

| Placeholder | Source | Refresh | Notes |
| --- | --- | --- | --- |
| `{time}` | local clock | interval | Uses `module.time.format` with supported strftime-style tokens. |
| `{hostname}` | OS hostname | static | Resolved once at startup. |
| `{user}` | environment / OS user | static | Checks `USER`, `LOGNAME`, `USERNAME`, then OS user lookup. |
| `{runtime}` | runtime detector | static | Values match `runtimeenv.Kind.String()` such as `native_linux`, `wsl2`, `macos`. |
| `{shell}` | resolved child argv | static | Shows the basename of the shell/command ptyline starts. |
| `{env}` | configured environment variables | event | Reads `[module.env].env = ["FOO", "BAR"]`; shell OSC hook emits at integration startup and on prompt/preexec. One name shows its value, multiple names show `NAME=value` entries. |
| `{cwd}` | shell integration | event | Empty until OSC shell integration reports `cwd`; abbreviated under `$HOME`. |
| `{ssh}` | SSH environment / integration | static + event | Shows inbound SSH env or outbound `ssh_start`/`ssh_end` metadata. |
| `{git}` | git provider | interval + cwd event | Refreshes against the current shell cwd and hides outside a git repo. |
| `{command}` | shell integration | event | Shows active or recently completed foreground command state. |
