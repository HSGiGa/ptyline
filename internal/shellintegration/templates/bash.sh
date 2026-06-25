# ptyline bash integration — printed by `ptyline init bash`.
# Enable it from ~/.bashrc:  eval "$(ptyline init bash)"
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17). Values are emitted in canonical form here —
# exit_code as a plain integer, duration_ms already in milliseconds, cwd absolute
# — so the Go side consumes one representation for every shell.

__ptyline_emit() { printf '\e]777;%s=%s\e\\' "$1" "$2"; }
__ptyline_now_ms() { date +%s%3N; }

# DEBUG fires before each command; capture the first command of the line and its
# start time, ignoring our own precmd hook.
__ptyline_preexec() {
    case "$BASH_COMMAND" in
    __ptyline_precmd*) return ;;
    esac
    [ -n "$__ptyline_running" ] && return
    __ptyline_running=1
    __ptyline_cmd=$BASH_COMMAND
    __ptyline_start=$(__ptyline_now_ms)
}

# PROMPT_COMMAND runs before each prompt; report exit code, cwd, and (if a command
# actually ran) the command and its duration.
__ptyline_precmd() {
    __ptyline_exit=$?
    if [ -n "$__ptyline_running" ]; then
        __ptyline_emit command "$__ptyline_cmd"
        __ptyline_emit duration_ms "$(($(__ptyline_now_ms) - __ptyline_start))"
        __ptyline_running=
    fi
    __ptyline_emit exit_code "$__ptyline_exit"
    __ptyline_emit cwd "$PWD"
}

trap '__ptyline_preexec' DEBUG
case "$PROMPT_COMMAND" in
*__ptyline_precmd*) ;;
*) PROMPT_COMMAND="__ptyline_precmd${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
esac
