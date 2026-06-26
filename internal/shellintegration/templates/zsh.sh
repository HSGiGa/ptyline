# ptyline zsh integration — printed by `ptyline init zsh`.
# Enable it from ~/.zshrc:  eval "$(ptyline init zsh)"
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms, env) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17). Values are emitted in canonical form here —
# exit_code as a plain integer, duration_ms already in milliseconds, cwd absolute
# — so the Go side consumes one representation for every shell.

__ptyline_emit() { printf '\e]777;%s=%s\e\\' "$1" "$2"; }
__ptyline_now_ms() { date +%s%3N; }
__ptyline_emit_env() {
    [ -z "$PTYLINE_ENV_NAMES" ] && return
    local __ptyline_name __ptyline_value __ptyline_out __ptyline_count
    local -a __ptyline_names
    __ptyline_names=("${(@s:,:)PTYLINE_ENV_NAMES}")
    __ptyline_count=${#__ptyline_names[@]}
    __ptyline_out=
    for __ptyline_name in "${__ptyline_names[@]}"; do
        case "$__ptyline_name" in
        ""|*[!A-Za-z0-9_]*|[0-9]*) continue ;;
        esac
        __ptyline_value="${(P)__ptyline_name}"
        [ -z "$__ptyline_value" ] && continue
        if [ "$__ptyline_count" -eq 1 ]; then
            __ptyline_out=$__ptyline_value
        else
            __ptyline_out="${__ptyline_out:+$__ptyline_out }$__ptyline_name=$__ptyline_value"
        fi
    done
    __ptyline_emit env "$__ptyline_out"
}

# preexec receives the full command line; record it and the start time.
__ptyline_preexec() {
    __ptyline_cmd=$1
    __ptyline_start=$(__ptyline_now_ms)
    __ptyline_emit command "$__ptyline_cmd"
    __ptyline_emit_env
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
    __ptyline_emit_env
    __ptyline_emit command ""
}

autoload -Uz add-zsh-hook
add-zsh-hook preexec __ptyline_preexec
add-zsh-hook precmd __ptyline_precmd

__ptyline_emit_env

# Wrap ssh to report outbound connections to the ptyline status bar.
# Use `command ssh` to bypass this wrapper when needed.
ssh() {
    __ptyline_emit ssh_start "${@[-1]}"
    command ssh "$@"
    local _code=$?
    __ptyline_emit ssh_end ""
    return $_code
}
