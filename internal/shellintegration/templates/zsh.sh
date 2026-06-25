# ptyline zsh integration — printed by `ptyline init zsh`.
# Enable it from ~/.zshrc:  eval "$(ptyline init zsh)"
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17). Values are emitted in canonical form here —
# exit_code as a plain integer, duration_ms already in milliseconds, cwd absolute
# — so the Go side consumes one representation for every shell.

__ptyline_emit() { printf '\e]777;%s=%s\e\\' "$1" "$2"; }
__ptyline_now_ms() { date +%s%3N; }

# preexec receives the full command line; record it and the start time.
__ptyline_preexec() {
    __ptyline_cmd=$1
    __ptyline_start=$(__ptyline_now_ms)
    __ptyline_emit command "$__ptyline_cmd"
}

# precmd runs before each prompt; report exit code, cwd, and the previous
# command duration when one ran, then clear the active command.
__ptyline_precmd() {
    __ptyline_exit=$?
    if [ -n "$__ptyline_start" ]; then
        __ptyline_emit duration_ms "$(($(__ptyline_now_ms) - __ptyline_start))"
        __ptyline_start=
    fi
    __ptyline_emit exit_code "$__ptyline_exit"
    __ptyline_emit cwd "$PWD"
    __ptyline_emit command ""
}

autoload -Uz add-zsh-hook
add-zsh-hook preexec __ptyline_preexec
add-zsh-hook precmd __ptyline_precmd
