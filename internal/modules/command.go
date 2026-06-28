package modules

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/formatting"
	"github.com/hsgiga/ptyline/internal/status/width"
)

const DefaultCommandMaxWidth = 60

const (
	DefaultCommandDoneMinDuration = 2 * time.Second
	DefaultCommandDoneSuccessTTL  = 3 * time.Second
)

type CommandDisplayPolicy struct {
	DoneMinDuration time.Duration
	DoneSuccessTTL  time.Duration
	DoneFailureTTL  time.Duration
	Separator       string
	Now             time.Time
}

// FormatCommand renders the shell command lifecycle as one state-aware module.
// The same format is used for active, done, and idle states; fields that do not
// apply to the current state expand to empty strings.
func FormatCommand(shell status.ShellState, format string, maxWidth int, policy CommandDisplayPolicy) (string, bool) {
	if maxWidth <= 0 {
		maxWidth = DefaultCommandMaxWidth
	}
	policy = normalizeCommandDisplayPolicy(policy)
	active := shell.ActiveCommand != ""
	if format == "" {
		format = "{active} {last} {exit} {duration}"
	}
	text := replaceCommandFields(format, shell, policy)
	text = strings.Join(strings.Fields(text), " ")
	return width.Truncate(text, maxWidth, "right"), active
}

func normalizeCommandDisplayPolicy(policy CommandDisplayPolicy) CommandDisplayPolicy {
	if policy.DoneMinDuration == 0 {
		policy.DoneMinDuration = DefaultCommandDoneMinDuration
	}
	if policy.DoneSuccessTTL == 0 {
		policy.DoneSuccessTTL = DefaultCommandDoneSuccessTTL
	}
	if policy.Separator == "" {
		policy.Separator = " | "
	}
	if policy.Now.IsZero() {
		policy.Now = time.Now()
	}
	return policy
}

func replaceCommandFields(format string, shell status.ShellState, policy CommandDisplayPolicy) string {
	active := shell.ActiveCommand
	last := ""
	exit := ""
	exitCode := ""
	duration := ""
	durationMS := ""
	if active == "" && shouldShowDoneCommand(shell, policy) {
		last = shell.LastCommand
		exit = FormatExit(shell.LastExitCode)
		exitCode = strconv.Itoa(shell.LastExitCode)
		duration = FormatDuration(shell.LastDurationMS)
		durationMS = strconv.Itoa(shell.LastDurationMS)
	}
	replacer := strings.NewReplacer(
		"{active}", active,
		"{last}", last,
		"{exit}", exit,
		"{exit_code}", exitCode,
		"{duration}", duration,
		"{duration_ms}", durationMS,
	)
	return formatting.CollapseSeparators(replacer.Replace(format), policy.Separator)
}

func ShouldClearDoneCommand(shell status.ShellState, policy CommandDisplayPolicy) bool {
	policy = normalizeCommandDisplayPolicy(policy)
	return shell.ActiveCommand == "" &&
		shell.LastCommandCompleted &&
		shell.LastExitCode == 0 &&
		policy.DoneSuccessTTL > 0 &&
		!shell.LastCommandCompletedAt.IsZero() &&
		!policy.Now.Before(shell.LastCommandCompletedAt.Add(policy.DoneSuccessTTL))
}

func ShouldTickDoneCommand(shell status.ShellState, policy CommandDisplayPolicy) bool {
	policy = normalizeCommandDisplayPolicy(policy)
	return shell.ActiveCommand == "" &&
		shell.LastCommandCompleted &&
		shell.LastExitCode == 0 &&
		time.Duration(shell.LastDurationMS)*time.Millisecond >= policy.DoneMinDuration &&
		policy.DoneSuccessTTL > 0 &&
		!ShouldClearDoneCommand(shell, policy)
}

func shouldShowDoneCommand(shell status.ShellState, policy CommandDisplayPolicy) bool {
	if shell.LastCommand == "" || !shell.LastCommandCompleted {
		return false
	}
	if shell.LastExitCode == 0 {
		if time.Duration(shell.LastDurationMS)*time.Millisecond < policy.DoneMinDuration {
			return false
		}
		if ShouldClearDoneCommand(shell, policy) {
			return false
		}
		return true
	}
	if policy.DoneFailureTTL > 0 &&
		!shell.LastCommandCompletedAt.IsZero() &&
		!policy.Now.Before(shell.LastCommandCompletedAt.Add(policy.DoneFailureTTL)) {
		return false
	}
	return true
}

func FormatExit(code int) string {
	if code == 0 {
		return "ok"
	}
	if signal, ok := signalExitName(code); ok {
		return signal
	}
	return "exit " + strconv.Itoa(code)
}

func signalExitName(code int) (string, bool) {
	signals := map[int]string{
		129: "sighup",
		130: "sigint",
		131: "sigquit",
		134: "sigabrt",
		137: "sigkill",
		139: "sigsegv",
		141: "sigpipe",
		143: "sigterm",
	}
	signal, ok := signals[code]
	return signal, ok
}

func FormatDuration(ms int) string {
	if ms < 0 {
		return ""
	}
	if ms < 1000 {
		return strconv.Itoa(ms) + "ms"
	}
	if ms < 10000 {
		seconds := fmt.Sprintf("%.1fs", float64(ms)/1000)
		return strings.Replace(seconds, ".0s", "s", 1)
	}
	if ms < 60000 {
		return strconv.Itoa(ms/1000) + "s"
	}
	minutes := ms / 60000
	seconds := (ms % 60000) / 1000
	return fmt.Sprintf("%dm%02ds", minutes, seconds)
}
