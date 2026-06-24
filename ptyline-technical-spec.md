# ptyline

## Technical Specification and Architecture

## 1. Purpose

This document describes the technical specification for a lightweight terminal wrapper that provides a persistent one-line bottom status bar inside a terminal session.

The application is intended to behave like the status line of `tmux`, but without implementing panes, tabs, sessions, layout management, or copy mode.

The main goal is to provide a configurable bottom bar for a normal interactive shell session while keeping native terminal scrollback behavior.

## 2. Product Goal

Create a command-line application that runs a user shell inside a pseudo-terminal and reserves the last visible terminal row for a custom status bar.

Example usage:

```sh
ptyline
ptyline -- zsh
ptyline -- ssh host.example
```

The user should see a normal shell experience, with one persistent line at the bottom showing configurable information such as current directory, Git branch, hostname, time, exit code, and custom module output.

## 3. Non-Goals

The first version must not attempt to become a full terminal multiplexer.

The following features are explicitly out of scope for the MVP:

- panes
- tabs
- sessions
- detached sessions
- copy mode
- custom scrollback buffer
- mouse-driven viewport control
- full VT/ANSI terminal emulation
- plugin system through WebAssembly or dynamic libraries
- terminal emulator specific integrations
- terminal font installation or font switching
- bundled custom fonts
- remote control protocol
- tmux compatibility

The application should remain a small PTY wrapper with a bottom bar.

## 4. Target Environment

MVP target environment:

- Linux, with Ubuntu 24.04 as the reference distribution
- WSL and WSL2, using the Linux binary

Native macOS and Windows binaries are post-MVP targets. Their backends must not delay
or weaken the Linux/WSL MVP.

Terminal emulators expected to work:

- Windows Terminal
- Ghostty
- WezTerm
- Alacritty
- GNOME Terminal
- Kitty

The application should rely on standard PTY and VT escape sequence behavior rather than terminal-emulator-specific APIs.

### 4.1 Platform Build Matrix (Post-MVP)

The project will use one source codebase and may produce separate binaries for each
target operating system after the Linux/WSL MVP.

```text
Linux binary
├─ common core
├─ Unix PTY backend
├─ Linux terminal backend
├─ Linux modules
└─ WSL detection/runtime branches

macOS binary
├─ common core
├─ Unix PTY backend
├─ macOS terminal backend
└─ macOS modules

Windows binary
├─ common core
├─ ConPTY backend selected and tested independently of the Unix PTY implementation
├─ Windows terminal backend
└─ Windows modules
```

WSL is not a separate binary target. WSL and WSL2 should be treated as runtime branches inside the Linux binary.

Build-time platform split:

```text
linux   → Linux binary
darwin  → macOS binary
windows → Windows binary
```

Runtime split inside the Linux binary:

```text
native Linux
wsl2
```

### 4.2 Runtime Environment Detection

The application should detect the runtime environment once during startup and expose the result as a normalized platform profile.

Suggested runtime kinds:

```text
native_linux
wsl2
macos
native_windows
unknown
```

The rest of the application should avoid scattered platform checks such as `if wsl2` inside individual modules.

Preferred flow:

```text
runtime detection
  ↓
platform profile
  ↓
capability flags
  ↓
backend selection
```

Suggested capability flags:

```text
unix_pty
windows_conpty
vt_sequences
linux_procfs
linux_sysfs
windows_interop
host_windows_access
native_battery_api
```

Components should depend on capabilities and selected backends, not on raw OS names.

Example:

```text
battery module asks for available battery providers
rather than checking directly whether it runs inside WSL2
```

## 5. High-Level Architecture

The application runs between the real terminal emulator and the user shell.

```text
Terminal Emulator
        ↓
ptyline application
        ↓
Pseudo-terminal / PTY
        ↓
any shell or interactive program: sh, bash, zsh, fish, nushell, PowerShell, ssh, vim, htop, etc.
```

The real terminal has the full visible size:

```text
cols × rows
```

The child PTY receives one row less:

```text
cols × (rows - 1)
```

The last row of the real terminal is reserved for the application status bar.

```text
┌──────────────────────────────┐
│ child PTY output              │
│ child PTY output              │
│ child PTY output              │
│ prompt                        │
├──────────────────────────────┤
│ cwd | git | time              │
└──────────────────────────────┘
```

## 6. Core Concept

The application must create and manage a child PTY.

The child process should believe that the terminal height is one row smaller than the real terminal height.

For example:

```text
real terminal: 120 × 30
child PTY:     120 × 29
bottom bar:    row 30
```

This prevents most interactive programs from writing into the reserved bottom row.

Additionally, the application should set the real terminal scroll region to exclude the last row:

```text
ESC [ 1 ; {rows - 1} r
```

This ensures that normal terminal scrolling affects only the shell output area and does not scroll the status bar.

## 7. Runtime Flow

Startup flow:

```text
start
  ↓
read config
  ↓
save current terminal state
  ↓
enable raw mode
  ↓
detect terminal size
  ↓
set real terminal scroll region to 1..rows-1
  ↓
spawn child shell inside PTY with size cols × rows-1
  ↓
start event loop
  ↓
proxy input/output and render bottom bar
  ↓
child exits
  ↓
restore terminal state
  ↓
exit with child exit code
```

## 8. Main Components

### 8.1 Terminal Controller

Responsible for managing the real terminal.

Responsibilities:

- save original terminal state
- restore terminal state on exit
- enable raw mode
- detect terminal size
- handle terminal resize events
- set and reset scroll region
- move cursor
- save and restore cursor position
- clear individual lines
- reset terminal attributes
- hide/show cursor only when needed

Required cleanup actions:

```text
reset scroll region
reset text attributes
restore cursor position
restore terminal mode
show cursor
```

The application must restore terminal state even when interrupted by signals such as `SIGINT`, `SIGTERM`, or child process exit.

### 8.2 PTY Supervisor

Responsible for creating and managing the child PTY.

Responsibilities:

- create PTY
- spawn shell or command inside PTY
- set child PTY size to `cols × (rows - 1)`
- update PTY size on terminal resize
- monitor child process lifecycle
- return child exit code from the wrapper process

On Unix the supervisor must create a session and controlling terminal for the child,
make the child process group foreground for that PTY, and preserve normal shell job
control. It owns the child process group rather than only the shell PID.

Signal ownership:

- bytes such as Ctrl-C and Ctrl-Z are forwarded to the PTY unchanged, so the kernel
  delivers terminal-generated signals to the child foreground process group;
- `SIGWINCH` resizes the PTY and lets the child receive its normal resize signal;
- on wrapper `SIGTERM`, `SIGHUP`, or controlled shutdown, terminate the child process
  group, wait for it, then restore the real terminal;
- terminal restoration is best-effort for uncatchable termination such as `SIGKILL`.

Resize rule:

```text
real terminal size: cols × rows
child PTY size:     cols × max(rows - 1, 1)
bar row:            rows
```

### 8.3 IO Proxy

Responsible for forwarding data between the real terminal and the child PTY.

Data flow:

```text
stdin  → application → child PTY
PTY    → application → stdout
```

The IO proxy should use an event loop or concurrent tasks.

All writes to the real terminal must pass through one serialized writer. It accepts
filtered PTY bytes and complete bar frames; no bar frame may be inserted into the
middle of a child output write. A redraw is scheduled only after the writer reaches a
safe event-loop boundary. The writer must handle short writes and interrupted writes
without dropping, duplicating, or reordering child bytes.

It must listen for:

- stdin data
- PTY output
- resize events
- status update ticks
- child process exit
- termination signals

Pseudo-flow:

```text
loop:
  if stdin is ready:
    read from stdin
    write to child PTY

  if child PTY output is ready:
    read from PTY
    pass through ANSI filter
    enqueue filtered bytes to serialized terminal writer
    schedule status bar redraw

  if resize event:
    update terminal size
    apply the mode-specific resize procedure

  if status timer tick:
    refresh status modules
    schedule a bar frame unless alternate screen is active

  if child exits:
    cleanup and exit
```

### 8.4 ANSI Filter

The MVP should not implement a full terminal emulator.

However, it should include a lightweight ANSI/VT filter to protect the reserved bottom row.

The filter should understand at least:

- `CSI r` — set/reset scroll margins
- `CSI J` — erase display
- `CSI K` — erase line
- `CSI H` / `CSI f` — cursor position
- `ESC [?1049h` — enter alternate screen
- `ESC [?1049l` — leave alternate screen
- `ESC [?1047h/l`
- `ESC [?47h/l`
- OSC sequences used for shell integration
- cursor save/restore sequences

The parser must be incremental and byte-oriented: CSI and OSC sequences may span PTY
reads, and ordinary PTY data is forwarded unchanged even when it is not valid UTF-8.
It must impose size limits on buffered control sequences and pass malformed or unknown
sequences through unchanged after recording a diagnostic.

Important rule:

If the child process sends:

```text
ESC [ r
```

which means reset scroll region, the wrapper should rewrite it as:

```text
ESC [ 1 ; {rows - 1} r
```

While the normal screen is active, if the child process tries to set a scroll region
that includes the last row, the wrapper should clamp it to `rows - 1`. While the
alternate screen is active, the filter must not clamp margins: the bar is hidden and
the child owns every terminal row.

Example:

```text
child sends: ESC [ 1 ; 30 r
real rows:   30
proxy sends: ESC [ 1 ; 29 r
```

The MVP does not virtualize every VT cursor operation. Therefore its fullscreen
guarantee applies to programs that enter the alternate screen; an application that
draws a fullscreen UI in the normal screen without doing so is a best-effort case.

### 8.5 Status State

The application should maintain a structured state object used by the renderer.

Example fields:

```text
cwd
git_branch
git_dirty
hostname
username
shell
last_exit_code
last_command_duration
time
custom module values
alternate_screen_active
terminal_cols
terminal_rows
```

The state should be updated by:

- shell integration OSC messages
- periodic modules
- internal events
- resize events
- child process lifecycle events

### 8.6 Bar Renderer

Responsible for drawing the bottom bar on the last visible terminal row.

The renderer must:

- save current cursor position
- move cursor to `row = terminal_rows`, `col = 1`
- clear the current line
- render the formatted status string
- reset styles
- restore cursor position

The renderer must not print a trailing newline.

Rendering flow:

```text
save cursor
move to last row
clear line
write status bar
reset style
restore cursor
```

The bar should support:

- left section
- center section
- right section
- block-based layout
- anchoring to the left, center, or right side of the terminal
- text alignment inside each block
- fixed, automatic, fill, and percentage-based widths
- minimum and maximum width constraints
- truncation
- padding
- Unicode width awareness
- ANSI color/style sequences
- configurable separators
- configurable update interval
- theme and style presets
- icon and emoji rendering through terminal fonts

Suggested format syntax:

```text
"{cwd} {git_branch} || {time}"
```

Where:

```text
left:   cwd + git_branch
center: empty
right:  time
```

This is a target-schema example: `git_branch` is a post-MVP provider. The MVP default
uses only its initial modules.

### 8.7 Module System

Modules provide values for the status bar.

Initial modules:

- time
- cwd
- hostname
- static text

The Git branch, dirty state, username, shell name, exit code, command duration, and
custom-command modules are post-MVP providers. Examples that use them describe the
target configuration schema, not the minimal default configuration.

Each module should have:

- refresh interval
- timeout
- cached value
- fallback value
- render function

Expensive modules must be cached.

Bad behavior:

```text
every redraw → run git status
```

Preferred behavior:

```text
every 2 seconds → refresh git module with 100 ms timeout
every redraw    → use cached git value
```

Custom command module example:

```toml
[module.kube]
command = "kubectl config current-context"
interval_ms = 10000
timeout_ms = 200
```

### 8.8 Status Bar Layout Schema

The status bar configuration should not use Markdown. Markdown is intended for documents and includes many concepts that are unnecessary for a one-line terminal status bar.

The preferred approach is:

```text
TOML configuration
+ small placeholder template language
+ structured layout schema
```

The layout system should separate three concerns:

```text
layout  → where a block is placed and how much space it receives
content → what a block renders
style   → how a block looks
```

A bar is composed of blocks. Each block may be anchored to a side of the terminal and aligned inside its own allocated area.

Core block layout properties:

```text
anchor      → left | center | right
align       → left | center | right
width       → auto | fill | N | N%
min_width   → N
max_width   → N | N%
truncate    → left | right | middle | none
priority    → number
```

Terminal layout should be measured in terminal cells, not pixels.

Supported width units:

```text
20      → 20 terminal cells
30%     → 30% of the full bar width
auto    → size based on rendered content
fill    → take remaining available space
```

The renderer must measure display width, not byte length and not rune count.

Example block-level configuration:

```toml
[[bar.block]]
module = "git"
anchor = "left"
width = "30%"
align = "left"
truncate = "right"
style = "git"

[[bar.block]]
module = "cpu"
anchor = "center"
width = "auto"
align = "center"
style = "cpu"

[[bar.block]]
module = "time"
anchor = "right"
width = 10
align = "right"
style = "time"
```

Each block may still use a simple content template:

```toml
[module.git]
format = "{icon} {branch}"
```

### 8.9 Theme and Style System

The application should provide a theme system for color schemes, visual styles, block shapes, icons, and fallback modes.

Conceptual split:

```text
color scheme → palette of colors
style preset → overall visual language
block style  → appearance of a specific block
```

Suggested color schemes:

```text
default
dark
light
gruvbox
catppuccin
nord
solarized
```

Suggested style presets:

```text
minimal
flat
powerline
box
pill
```

The style system should support:

- foreground color
- background color
- accent color
- bold, dim, italic, and underline attributes
- padding
- margins
- left separator
- right separator
- inner separator
- fallback shape

Example theme-level configuration:

```toml
[theme]
color_scheme = "catppuccin"
style = "powerline"
icons = "nerd-font"
fallback = "ascii"

[theme.palette]
bar_bg = "#202020"
text = "#ffffff"
muted = "#888888"
accent = "#aaff00"
```

Example block style:

```toml
[style.git]
fg = "#000000"
bg = "#aaff00"
bold = true
shape = "powerline"
left_separator = ""
right_separator = ""
padding_left = 1
padding_right = 1

[style.time]
fg = "#ffffff"
bg = "#4444ff"
shape = "pill"
align = "right"
padding_left = 1
padding_right = 1
```

Visual styles are terminal text, not a GUI. Segment shapes are produced using Unicode characters, separators, background colors, and padding.

Example Powerline-like rendering:

```text
 git main    cpu 12%          14:32 
```

Example ASCII fallback:

```text
[ git main ]  [ cpu 12% ]        [ 14:32 ]
```

For the MVP, the project should support a minimal built-in theme and allow basic per-block foreground/background customization. Rich theme presets can be expanded later.

### 8.10 Icons, Emoji, and Font Handling

Icons and emoji should be treated as normal text rendered by the terminal emulator.

The application should output:

```text
text + Unicode glyphs + ANSI styles
```

The terminal emulator is responsible for font selection and glyph rendering.

The application should not attempt to:

- bundle a custom font
- switch terminal fonts
- install fonts automatically
- rely on terminal-emulator-specific font APIs

Recommended icon modes:

```text
nerd-font → use Nerd Font glyphs and Powerline separators
emoji     → use standard Unicode emoji where practical
ascii     → use plain text fallback
```

Nerd Font support should be optional. If the user's terminal font does not contain the requested glyphs, the bar should still be usable through fallback icons.

Example module icon configuration:

```toml
[module.git]
icon = ""
fallback_icon = "git"

[module.battery]
icon = "🔋"
fallback_icon = "bat"
```

Important rendering requirement:

```text
The renderer measures display width, not byte length and not rune count.
```

Emoji and some Unicode glyphs can have ambiguous display width across terminals. The configuration should allow a conservative fallback mode.

Example:

```toml
[icons]
enabled = true
preset = "nerd-font" # nerd-font | emoji | ascii
emoji_width = "auto" # auto | 1 | 2
fallback = true
```

### 8.11 Mouse Interaction Layer

Clickable and mouse-aware modules are a post-MVP feature.

The architecture should leave room for mouse handling without making it mandatory for the first version.

Conceptual flow:

```text
mouse input layer
  ↓
hitbox registry
  ↓
block action dispatcher
```

During rendering, each visible block can register its terminal cell range:

```text
block_id
row
start_col
end_col
action
```

Mouse event ownership rule:

```text
if mouse event is on the status bar row → handle by ptyline
if mouse event is above the status bar row → forward to child PTY
```

This is important because applications such as `vim`, `less`, `htop`, and `btop` may use mouse input inside the child PTY.

Potential actions:

```text
left click
right click
middle click
scroll over block
```

Example future configuration:

```toml
[[bar.block]]
module = "git"
on_click = "open_git_status"

[[bar.block]]
module = "battery"
on_click = "show_battery_details"
```

Mouse support should remain optional and disabled by default until the behavior is reliable across terminal emulators.

## 9. Shell Integration

The wrapper cannot reliably know shell state only from PTY bytes.

For accurate current directory, exit code, and command duration, shell integration should be supported.

The shell should send metadata to the wrapper using OSC sequences.

Example conceptual protocol:

```text
OSC 777 ; cwd=/home/vi/project ST
OSC 777 ; exit_code=0 ST
OSC 777 ; duration_ms=153 ST
OSC 777 ; command=git status ST
```

Only OSC 777 payloads matching `key=value` are metadata. The MVP accepts the
whitelisted keys `cwd`, `exit_code`, `duration_ms`, and `command`; values must not
contain control characters, and a payload is limited to 8 KiB. The ANSI filter
intercepts accepted messages, updates internal state, and avoids printing them to the
real terminal. Unknown or malformed OSC 777 messages are discarded with a diagnostic;
they never cause command execution.

## 10. Scrolling Behavior

There are two different kinds of scrolling:

1. terminal live scroll region
2. terminal emulator scrollback

The application can control the live scroll region, but it cannot control the terminal emulator scrollback viewport as a fixed GUI overlay.

### 10.1 Live Scrolling

The app should set the real terminal scroll region to:

```text
1..rows-1
```

Result:

```text
rows 1..rows-1 → normal command output
row rows       → bottom status bar
```

When command output reaches the bottom of the shell area, only the shell area scrolls.

The status bar remains visible in the live screen.

### 10.2 Terminal Emulator Scrollback

Windows Terminal, Ghostty, WezTerm, Alacritty, and similar terminal emulators maintain their own scrollback buffers.

When the user manually scrolls back, the terminal emulator shows historical content.

The application cannot draw a true pinned overlay over that scrollback viewport.

Expected behavior:

- while at the live bottom, the status bar remains visible
- when the user scrolls into history, the status bar may disappear from view
- this is acceptable for MVP
- the application should not implement its own scrollback in the first version

### 10.3 Bar and Scrollback

The status bar should not intentionally enter scrollback.

The renderer must:

- avoid printing newline after the bar
- draw using absolute cursor positioning
- restore cursor after drawing

## 11. Alternate Screen Behavior

Fullscreen applications such as `vim`, `nvim`, `less`, `fzf`, `htop`, and `btop` often use alternate screen mode.

The wrapper must detect alternate screen enter/leave sequences:

```text
ESC [?1049h
ESC [?1049l
ESC [?1047h
ESC [?1047l
ESC [?47h
ESC [?47l
```

For the MVP there is one fixed policy: hide the bar whenever the child enters the
alternate screen. Fullscreen support is determined by these VT transitions, not by
the process name. This covers typical `vim`, `less`, `fzf`, `htop`, and ncurses
programs that use the alternate buffer; it does not promise special handling for a
program that draws fullscreen content in the normal buffer.

On alternate-screen entry, the serialized terminal writer must:

1. stop scheduled bar redraws;
2. reset the real terminal scroll region;
3. forward the alternate-screen enter sequence;
4. resize the child PTY to `cols × rows`, causing its normal resize notification.

On alternate-screen exit, it must:

1. forward the leave sequence;
2. resize the child PTY to `cols × max(rows - 1, 1)`;
3. set the real scroll region to `1..rows-1` when there are at least two rows;
4. redraw the bar.

The same serialized writer owns filtered PTY output and bar frames. It must not
interleave a bar redraw with child output.

## 12. Resize Behavior

On terminal resize:

1. pause redraw briefly or debounce resize events
2. query new terminal size
3. reset scroll region if needed
4. if alternate screen is active, resize child PTY to `cols × rows` and keep the
   scroll region reset;
5. otherwise resize child PTY to `cols × max(rows - 1, 1)`, set the real scroll
   region to `1..rows-1`, and redraw the bar;
6. skip bar rendering while alternate screen is active;
7. continue IO proxying

Resize must work both in normal and alternate screen modes.

## 13. Configuration

Suggested config path:

```text
$XDG_CONFIG_HOME/ptyline/config.toml
```

Fallback:

```text
~/.config/ptyline/config.toml
```

Example config:

```toml
shell = "auto" # auto uses $SHELL or the platform default; a command/path may also be set
refresh_interval_ms = 1000
config_version = 1

[bar]
format = "{cwd} {hostname} || {time}"
separator = " | "

[module.time]
enabled = true
format = "%H:%M:%S"
interval_ms = 1000

[module.cwd]
enabled = true
mode = "shell-integration"

[module.hostname]
enabled = true
```

`cwd` with `mode = "shell-integration"` displays its configured fallback (empty by
default) until an adapter supplies metadata. It must not infer the current directory
from arbitrary PTY output.

Extended target-schema layout and theme example; it uses post-MVP providers:

```toml
[bar]
height = 1
padding = 0

[[bar.block]]
module = "git"
anchor = "left"
width = "30%"
align = "left"
truncate = "right"
style = "git"

[[bar.block]]
module = "cpu"
anchor = "center"
width = "auto"
align = "center"
style = "cpu"

[[bar.block]]
module = "time"
anchor = "right"
width = 10
align = "right"
style = "time"

[theme]
color_scheme = "default"
style = "flat"
icons = "ascii"
fallback = "ascii"

[icons]
enabled = true
preset = "ascii"
fallback = true

[style.git]
fg = "black"
bg = "green"
bold = true
padding_left = 1
padding_right = 1

[style.cpu]
fg = "white"
bg = "black"
padding_left = 1
padding_right = 1

[style.time]
fg = "white"
bg = "blue"
bold = true
padding_left = 1
padding_right = 1
```

### 13.1 MVP Configuration Contract

`config_version` is required and currently must equal `1`. Unknown top-level keys,
unknown module keys, invalid enum values, and invalid width expressions are startup
errors; the application must name the file and key in its diagnostic. Configuration is
loaded, migrated to the current version, validated, and only then applied.

For the MVP, `bar.format` and `[[bar.block]]` are mutually exclusive:

- if `[[bar.block]]` is present, it is the layout source and `bar.format` is rejected;
- otherwise `bar.format` is used, with the text before `||` on the left and after it
  on the right; no centre block is created;
- `bar.height` must be `1` in the normal screen; alternate-screen behavior is fixed
  by section 11 and is not configurable in the MVP.

Module IDs are unique. A block references an enabled module by ID. The loader supplies
documented defaults only when a value is omitted; it does not silently reinterpret an
invalid configuration. An unavailable optional provider renders its fallback and emits
a diagnostic; it does not block terminal I/O.

## 14. CLI Interface

Proposed commands:

```sh
ptyline
ptyline -- zsh
ptyline -- /usr/local/bin/nu
ptyline --config ~/.config/ptyline/config.toml -- bash
ptyline init bash
ptyline --version
ptyline --help
```

Behavior:

- without arguments, use configured shell or `$SHELL`
- with command arguments, run that command inside the PTY
- `init <shell>` prints integration code for a supported shell; the PTY wrapper itself does not require integration
- wrapper exits with the child process exit code

## 15. Error Handling

The application must handle:

- terminal size smaller than 2 rows
- PTY creation failure
- shell spawn failure
- invalid config
- status module timeout
- child process exit
- broken pipe
- interrupted system calls
- terminal resize during rendering
- partial CSI/OSC sequences across PTY reads
- malformed or oversized ANSI sequences
- child process-group termination and wait timeout

If initialization fails after modifying terminal state, the app must restore terminal state before exiting.

## 16. Performance Requirements

The wrapper should add minimal latency.

Requirements:

- input forwarding should feel immediate
- output forwarding should avoid unnecessary buffering
- status redraws should be throttled
- modules should be cached
- expensive commands should have timeouts
- bar redraw should be skipped if rendered content has not changed
- resize events should be debounced
- a terminal write must preserve the byte order produced by the child

Suggested defaults:

```text
status refresh interval: 1000 ms
git refresh interval:    2000 ms
custom command timeout:  200 ms
resize debounce:         30-50 ms
maximum bar redraw rate: 20 Hz
maximum OSC payload:     8 KiB
maximum buffered CSI:    4 KiB
```

## 17. Security Considerations

Custom command modules execute local shell commands.

The config file should be treated as trusted user configuration.

The application should not execute remote content automatically.

OSC shell integration messages should be parsed strictly and should not trigger arbitrary command execution.

## 18. MVP Scope

The MVP should implement:

- run shell in PTY
- raw mode
- stdin to PTY proxy
- PTY output to stdout proxy
- child PTY size = rows - 1
- real terminal scroll region = `1..rows-1`
- draw one-line bottom bar
- redraw on timer
- redraw on resize
- cleanup on exit
- basic config
- basic block layout schema
- left, center, and right anchored blocks
- fixed-width and auto-width blocks
- basic ANSI foreground/background colors
- one default theme
- ASCII-safe fallback rendering
- time module
- hostname module
- static text module
- minimal ANSI filter for scroll region reset
- detect alternate-screen transitions; hide the bar and give the child full height
- restore the reserved row, scroll region, and bar after alternate-screen exit
- optional shell integration for cwd and exit code; adapters are independent and may be added for any shell
- serialized terminal writer for PTY output and bar frames
- Unix session, controlling-terminal, process-group, and signal handling
- Linux/WSL runtime support only

## 19. Post-MVP Features

Possible future features:

- Git dirty state
- Git branch
- CPU module
- command duration
- current command name
- battery module
- Kubernetes context module
- AWS profile module
- advanced theme presets
- color scheme presets such as gruvbox, catppuccin, nord, and solarized
- Powerline, pill, box, and other segment styles
- Nerd Font icon presets
- clickable/mouse-aware modules
- additional shell-integration adapters
- smarter ANSI parser
- optional visible bar in alternate screen
- status update on shell events instead of timer
- terminal emulator feature detection
- tests with recorded PTY sessions
- native macOS and Windows/ConPTY backends

## 20. Acceptance Criteria

MVP is considered successful when:

1. Running `ptyline` starts the configured or detected interactive shell; `ptyline -- <command>` runs any command inside the PTY.
2. The bottom row displays a configurable status bar.
3. Normal command output does not overwrite the status bar.
4. Long output scrolls only above the status bar.
5. Terminal resize keeps the bar on the last row in the normal screen.
6. The child sees terminal height as `rows - 1` in the normal screen and full
   terminal height while alternate screen is active.
7. `vim`, `less`, `fzf`, and `htop` that enter alternate screen receive full height,
   temporarily hide the bar, and restore the normal terminal state on exit.
8. Exiting the child shell restores terminal state.
9. If the app is interrupted, terminal state is still restored.
10. The wrapper exits with the same exit code as the child process.
11. In a benchmark with status modules served from cache, the p95 time from scheduling
    a bar frame to completing its terminal write is at most 10 ms; a bar redraw never
    changes the order of child-output bytes.
12. Manual terminal scrollback remains native to the terminal emulator, even if the status bar is not pinned during scrollback.
13. Configured left, center, and right blocks render in the expected positions.
14. Basic foreground/background styling does not corrupt the child terminal output.
15. The default theme remains readable without Nerd Font or emoji support.
16. Automated tests cover partial CSI/OSC input, malformed control sequences,
    alternate-screen entry/exit, resize in both modes, and terminal cleanup after a
    controlled signal.

### 20.1 Verification Matrix

The repository must provide deterministic tests and replay fixtures for the following
cases. Interactive manual testing supplements these tests but is not the acceptance
mechanism.

| Case | Required assertion |
| --- | --- |
| Normal shell output | output bytes retain order; bottom row is redrawn without a newline |
| Scroll-margin reset from child | output margin is clamped to `rows - 1` in normal screen |
| Split CSI/OSC input | parser produces the same result regardless of PTY read boundaries |
| Alternate-screen enter/exit | child receives `rows`, then `rows - 1`; bar is absent, then restored |
| Resize | correct PTY size and scroll margin are applied for the current screen mode |
| Controlled shutdown | child process group is reaped and terminal modes, cursor, and margins are restored |

Reference manual checks run in the Linux/WSL MVP environment with `bash` or `zsh` and
`vim`, `less`, `fzf`, and `htop`. They verify only programs that enter alternate
screen; normal-buffer fullscreen programs remain best-effort by design.

## 21. Suggested Project Structure

```text
ptyline/
  cmd/ptyline/
    main.go
  internal/
    app/
    terminal/
      controller.go
      raw_mode.go
      size.go
      escape.go
      scroll_region.go
    pty/
      supervisor.go
      resize.go
      spawn_unix.go
      spawn_windows.go
    proxy/
      eventloop.go
      ansi_filter.go
      osc.go
    status/
      state.go
      module.go
      renderer/
      layout/
      theme/
    runtimeenv/
      detector.go
      profile.go
      capabilities.go
    platform/
      linux.go
      wsl.go
      darwin.go
      windows.go
    modules/
      time.go
      cwd.go
      hostname.go
      static.go
    shellintegration/
      osc.go
      adapters/
    config/
      schema.go
      loader.go
      migrate.go
    event/
    diagnostics/
```

## 22. Implementation Language: Go

Pros:

- simple concurrency model
- easy static binaries
- fast development

Suggested packages:

- `creack/pty`
- `golang.org/x/term`
- `termenv`
- `go-runewidth`

Suggested Go package layout:

```text
internal/
  app/
  terminal/
  pty/
  proxy/
  status/
    layout/
    renderer/
    theme/
    style/
    width/
    icons/
  modules/
  shellintegration/
  runtimeenv/
  platform/
```

Build matrix:

```text
GOOS=linux   → Linux binary with Unix PTY and WSL runtime branches
GOOS=darwin  → macOS binary with Unix PTY
GOOS=windows → Windows binary with ConPTY
```

Go is the project language. The implementation must use idiomatic Go, standard
context cancellation, and platform-specific files or build tags where required.

## 23. Design Decision Summary

Chosen design:

```text
PTY wrapper
+ child PTY height = rows - 1
+ real terminal scroll region = 1..rows-1
+ bottom-line renderer
+ lightweight ANSI filter
+ shell integration through OSC
+ native terminal emulator scrollback
+ TOML configuration
+ structured status bar layout schema
+ theme and style system
+ icon and emoji support through terminal fonts
+ platform build matrix with WSL as Linux runtime branch
```

Rejected for MVP:

```text
full terminal multiplexer
custom scrollback
terminal-emulator-specific UI APIs
custom terminal font management
full VT emulator
```

This keeps the project focused, practical, and significantly simpler than `tmux` or `zellij`, while still solving the main problem: a persistent configurable bottom status bar in a normal terminal session.

## 24. Future-Ready Architecture

The MVP must not be hardcoded around a final string such as `cwd | git | time`.
It should use a unidirectional pipeline:

```text
input sources → events → normalized state → layout → renderer → terminal output
```

Providers collect data; the state layer normalizes it; layout decides what fits; only
the renderer writes terminal escape sequences. Rendering must consume prepared state,
never query Git, a shell, agents, or custom commands directly.

### 24.1 State Store and Events

Maintain one structured state object. Fields not implemented by the MVP remain
reserved so providers can be introduced without reshaping the renderer.

```go
type StatusState struct {
    Terminal    TerminalState
    Shell       ShellState
    Git         *GitState
    Modules     ModuleValues
    Agents      []AgentState
    Diagnostics DiagnosticsState
}

type AppEvent interface{}
// Concrete events: StdinInput, PtyOutput, Resize, Tick, ShellMeta,
// ModuleUpdated, AgentUpdated, ChildExited, and TerminationSignal.
```

The event loop is the integration boundary for timers, PTY I/O, OSC metadata, custom
modules, sockets, clicks, recording/replay, and termination. Event types may be
unimplemented in the MVP, but adding a source must not require rewriting the core.

Shell metadata is optional. The wrapper works with any shell or command without an
adapter. Where a shell exposes hooks, an adapter may send the common OSC metadata
protocol for `cwd`, exit code, duration, and command name. The ANSI/OSC filter parses
those messages strictly, updates `ShellState`, and suppresses them from terminal output.

### 24.2 Reserved Area and Terminal Ownership

Do not spread the assumption of one bottom row throughout the codebase. Use an
explicit reserved area:

```go
type ReservedArea struct {
    Edge Edge
    Rows uint16
}
```

The MVP value is `Bottom, 1`; PTY sizing is always `terminal_rows - reserved.rows`.
This permits later multi-line bars, a hidden alternate-screen bar, and panel mode.
The terminal controller owns raw-mode setup/cleanup, cursor state, scroll regions,
resizes, and reset of attributes. The PTY supervisor owns the child process and its
size. The two components must not independently emit conflicting cursor control.

### 24.3 Providers, Typed Values, and Diagnostics

Providers publish typed snapshots rather than display strings:

```go
type ModuleValue struct {
    Kind   ModuleValueKind
    Text   string
    Number float64
    Bool   bool
    Status *StatusValue
    JSON   json.RawMessage
}

type ModuleSnapshot struct {
    ID        ModuleID
    Value     ModuleValue
    UpdatedAt time.Time
    Stale     bool
    Err       error
}
```

This distinguishes unavailable, stale, timed-out, and failed data without encoding
state into strings. `DiagnosticsState` records module errors, render duration, PTY
errors, parser warnings, and config warnings. Future `doctor`, `debug-state`, and
`replay` commands read this state rather than scrape terminal output.

Custom commands execute only as trusted local configuration, with timeout and output
limits. OSC and socket input may update state but must never trigger command execution.

### 24.4 Layout and Rendering

Internally, parse the configured format into renderable items. The layout engine must
support left/center/right anchors, ANSI styles, Unicode display width, conditional
visibility, compact forms, and priority-based truncation. Each item has a priority,
minimum width, and optional compact representation; low-value information disappears
first on narrow terminals.

Modules request semantic theme tokens rather than embedding ANSI codes. The renderer
translates tokens into terminal escapes and returns a `RenderedBar`, which can later
also contain click zones:

```go
type RenderedBar struct {
    Line       string
    ClickZones []ClickZone
}
```

Mouse behavior remains disabled by default. Click actions that execute commands are
explicit opt-in.

### 24.5 Agent and External Providers

Agent support is post-MVP but is a first-class reserved type:

```go
type AgentState struct {
    ID        string
    Name      string
    Status    AgentStatus
    CWD       string
    Task      string
    Tokens    *uint64
    Cost      *float64
    UpdatedAt time.Time
}
```

Future agent providers can use filtered OSC events, a local JSONL Unix socket such as
`$XDG_RUNTIME_DIR/ptyline/events.sock`, or a periodic JSON-producing command. They
all normalize into `AgentState`; the renderer is source-agnostic. Agent display has
compact fallbacks and never blocks normal PTY proxying.

### 24.6 Capabilities, Configuration, and Tests

Detect the runtime environment once and expose capabilities instead of scattered OS or
shell checks. Relevant capabilities include Unix PTY/ConPTY, VT sequences, truecolor,
OSC 8 links, mouse, alternate screen, Unicode/emoji fallback, and platform data
providers. WSL remains a Linux runtime branch.

Start configuration with `config_version = 1` and migrate old documents before parsing.
Configuration can therefore gain providers, themes, layout priorities, mouse actions,
and multi-line modes without breaking existing users.

Support deterministic record/replay tests for stdin, PTY output, resize, ticks, OSC
events, signals, and child exit. Replays must cover scroll-region protection, redraws,
alternate-screen transitions, cleanup, and interactive programs such as `vim`, `less`,
`fzf`, `htop`, and `ssh`.

### 24.7 MVP Boundary

The MVP implements PTY proxying, a one-row `ReservedArea`, raw-mode cleanup, resize,
one-line rendering, basic configuration, time/hostname/static modules, a minimal
ANSI/OSC filter, typed module snapshots, and the event/state/render boundaries.

It deliberately excludes multi-line panels, agent providers, mouse actions, socket
protocols, dynamic plugins, remote control, custom scrollback, and multiplexer
features. These exclusions do not alter the interfaces above.
