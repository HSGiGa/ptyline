package app

// handoffVersion must be incremented whenever handoffState fields change in an
// incompatible way. parseHandoff rejects mismatched versions so an old binary
// cannot accidentally continue inside a new binary's exec handoff.
const handoffVersion = 1

// handoffEnvKey is the environment variable that carries the serialised
// handoffState across a re-exec boundary.
const handoffEnvKey = "PTYLINE_HANDOFF"

// handoffState is the payload transferred from the old process image to the new
// one via PTYLINE_HANDOFF=base64(json). Only the fields listed here survive
// exec(); everything else is re-initialised from config and runtime detection.
type handoffState struct {
	Version   int      `json:"v"`
	PtyFD     int      `json:"pty_fd"`
	ChildPID  int      `json:"child_pid"`
	Nonce     string   `json:"nonce"`      // execEnvNonce; the shell already has it in $PTYLINE_NONCE
	ChildArgv []string `json:"child_argv"` // passed to modules.NewShell for display only
}
