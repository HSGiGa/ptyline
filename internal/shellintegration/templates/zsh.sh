# ptyline zsh integration — printed by `ptyline init zsh`.
# Enable it from ~/.zshrc:  eval "$(ptyline init zsh)"
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms, env) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17). Values are emitted in canonical form here —
# exit_code as a plain integer, duration_ms already in milliseconds, cwd absolute
# — so the Go side consumes one representation for every shell.
#
# colors: ptyline no longer reads shell color variables; the bar palette comes
# entirely from its themes. Under zsh, color_scheme = "default" resolves to the
# zsh-default theme.

__ptyline_emit() { printf '\e]777;%s=%s\e\\' "$1" "$2"; }
# zsh has $EPOCHREALTIME via zsh/datetime; avoids GNU-only date +%s%3N on macOS.
__ptyline_now_ms() {
    zmodload zsh/datetime 2>/dev/null
    if (( ${+EPOCHREALTIME} )); then
        printf '%.0f\n' $(( EPOCHREALTIME * 1000 ))
    else
        printf '%s000\n' "$(date +%s)"
    fi
}
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
# The exec_env mirror below reads the $parameters association from the zsh/parameter
# module to enumerate exported variables; it is not loaded by default, so pull it in
# here (a no-op if already loaded or unavailable).
zmodload zsh/parameter 2>/dev/null

# Mirror selected exported variables to ptyline so exec modules ({gh}, …) run with
# the shell's live environment. Each pattern is an exact name or a NAME* prefix;
# values are base64-encoded (so ';'/'='/control chars can't corrupt the frame) and
# the whole frame carries $PTYLINE_NONCE so injected bytes can't forge it.
__ptyline_emit_exec_env() {
    [ -z "$PTYLINE_EXEC_ENV_NAMES" ] && return
    [ -z "$PTYLINE_NONCE" ] && return
    local __ptyline_pat __ptyline_name __ptyline_value __ptyline_b64 __ptyline_out
    local -a __ptyline_patterns
    __ptyline_patterns=("${(@s:,:)PTYLINE_EXEC_ENV_NAMES}")
    __ptyline_out=
    for __ptyline_pat in "${__ptyline_patterns[@]}"; do
        case "$__ptyline_pat" in
        ""|*[!A-Za-z0-9_*]*|[0-9]*) continue ;;
        esac
        for __ptyline_name in ${(k)parameters}; do
            [[ $__ptyline_name == ${~__ptyline_pat} ]] || continue
            [[ ${(t)parameters[$__ptyline_name]} == *export* ]] || continue
            __ptyline_value="${(P)__ptyline_name}"
            [ -z "$__ptyline_value" ] && continue
            __ptyline_b64=$(printf '%s' "$__ptyline_value" | base64 | tr -d '\n')
            __ptyline_out="${__ptyline_out:+$__ptyline_out;}$__ptyline_name=$__ptyline_b64"
        done
    done
    __ptyline_emit exec_env "$PTYLINE_NONCE:$__ptyline_out"
}

# preexec receives the full command line; record it and the start time.
__ptyline_preexec() {
    __ptyline_cmd=$1
    __ptyline_start=$(__ptyline_now_ms)
    __ptyline_emit command "$__ptyline_cmd"
    __ptyline_emit_env
    __ptyline_emit_exec_env
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
    # cwd is nonce-tagged like exec_env: ptyline runs exec-module commands and
    # discovers project .ptyline files from it, so a forged OSC 777 cwd (printed
    # from a file or command output) must not be able to redirect them.
    __ptyline_emit cwd "$PTYLINE_NONCE:$PWD"
    __ptyline_emit_env
    __ptyline_emit_exec_env
    __ptyline_emit command ""
}

autoload -Uz add-zsh-hook
add-zsh-hook preexec __ptyline_preexec
add-zsh-hook precmd __ptyline_precmd

__ptyline_emit_env
__ptyline_emit_exec_env

# Wrap ssh to report outbound connections to the ptyline status bar.
# Use `command ssh` to bypass this wrapper when needed.
ssh() {
    local _ptyline_host=
    for _ptyline_a in "$@"; do
        case "$_ptyline_a" in -*) ;; *) _ptyline_host=$_ptyline_a; break ;; esac
    done
    __ptyline_emit ssh_start "$_ptyline_host"
    command ssh "$@"
    local _code=$?
    __ptyline_emit ssh_end ""
    return $_code
}
