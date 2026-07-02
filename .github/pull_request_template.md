## What changed

-

## Risk area

- [ ] Terminal/PTY/proxy safety
- [ ] Shell integration
- [ ] Config/reload/overlays
- [ ] Modules/exec commands
- [ ] Security
- [ ] Performance
- [ ] Docs/build/release only

## Verification

- [ ] `make check`
- [ ] `go test -race -shuffle=on ./...`
- [ ] `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
- [ ] Manual terminal smoke if touching terminal, PTY, proxy, or shell templates

Notes:

