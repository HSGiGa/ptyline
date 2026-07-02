# Contributing to ptyline

ptyline is a Go terminal wrapper that reserves bottom terminal rows for a status
bar. Changes in terminal, PTY, proxy, and shell-integration code can corrupt a
live terminal, so keep changes focused and verified.

## Prerequisites

- Go 1.26.4 or newer.
- Linux, WSL2, or macOS.

## Local Workflow

```sh
make bootstrap
make build
make check
go test -race -shuffle=on ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

`make check` runs formatting, `go vet`, tests, and the pinned golangci-lint
setup. Run `make fmt` before opening a PR if `fmt-check` fails.

## Terminal Safety

Read `ARCHITECTURE.md` before touching `internal/terminal`, `internal/pty`, or
`internal/proxy`.

Load-bearing rules:

- child PTY height is computed through `reserved.Area`;
- scroll region excludes reserved rows in normal screen and resets in alt screen;
- all terminal writes go through the serialized writer;
- the bar is drawn with absolute cursor positioning and no trailing newline;
- terminal state is restored on exit, signal, and init failure;
- OSC messages are parsed strictly and never executed.

## Pull Requests

- Keep commits focused.
- Explain terminal, security, and performance risks when relevant.
- Include verification commands in the PR description.
- If touching terminal/proxy/PTY/shell templates, include manual smoke notes:
  shell launch/exit, `reset`, alt-screen app, resize, and `ptyline --reload`.

