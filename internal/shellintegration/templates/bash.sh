# ptyline bash integration — printed by `ptyline init bash`.
# Enable it from ~/.bashrc:  eval "$(ptyline init bash)"
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms, env) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17). Values are emitted in canonical form here —
# exit_code as a plain integer, duration_ms already in milliseconds, cwd absolute
# — so the Go side consumes one representation for every shell.
#
# colors: bash has no standard color variable system ($PS1 embeds raw ANSI escapes,
# not named color variables), so the "colors" key is not emitted here. ptyline uses
# its default palette which already matches bash prompt conventions.

__ptyline_emit() { printf '\e]777;%s=%s\e\\' "$1" "$2"; }
# $EPOCHREALTIME (bash ≥5, macOS bash 3.2 lacks it) avoids GNU-only date +%s%3N.
__ptyline_now_ms() {
    if [ -n "${EPOCHREALTIME-}" ]; then
        printf '%.0f' "${EPOCHREALTIME}000"
    else
        printf '%s000' "$(date +%s)"
    fi
}
__ptyline_emit_env() {
    [ -z "$PTYLINE_ENV_NAMES" ] && return
    local __ptyline_names __ptyline_name __ptyline_value __ptyline_out __ptyline_count
    IFS=',' read -r -a __ptyline_names <<< "$PTYLINE_ENV_NAMES"
    __ptyline_count=${#__ptyline_names[@]}
    __ptyline_out=
    for __ptyline_name in "${__ptyline_names[@]}"; do
        case "$__ptyline_name" in
        ""|*[!A-Za-z0-9_]*|[0-9]*) continue ;;
        esac
        __ptyline_value=${!__ptyline_name}
        [ -z "$__ptyline_value" ] && continue
        if [ "$__ptyline_count" -eq 1 ]; then
            __ptyline_out=$__ptyline_value
        else
            __ptyline_out="${__ptyline_out:+$__ptyline_out }$__ptyline_name=$__ptyline_value"
        fi
    done
    __ptyline_emit env "$__ptyline_out"
}
# Mirror selected exported variables to ptyline so exec modules ({gh}, …) run with
# the shell's live environment. Each pattern is an exact name or a NAME* prefix;
# values are base64-encoded (so ';'/'='/control chars can't corrupt the frame) and
# the whole frame carries $PTYLINE_NONCE so injected bytes can't forge it.
__ptyline_emit_exec_env() {
    [ -z "$PTYLINE_EXEC_ENV_NAMES" ] && return
    [ -z "$PTYLINE_NONCE" ] && return
    local __ptyline_patterns __ptyline_pat __ptyline_names __ptyline_name __ptyline_value __ptyline_b64 __ptyline_out
    IFS=',' read -r -a __ptyline_patterns <<< "$PTYLINE_EXEC_ENV_NAMES"
    __ptyline_out=
    for __ptyline_pat in "${__ptyline_patterns[@]}"; do
        case "$__ptyline_pat" in
        ""|*[!A-Za-z0-9_*]*|[0-9]*) continue ;;
        esac
        case "$__ptyline_pat" in
        *"*")
            __ptyline_names=$(compgen -e -- "${__ptyline_pat%\*}" 2>/dev/null)
            ;;
        *)
            # Exact name: mirror only if it is exported (matching the wildcard branch,
            # which uses `compgen -e`). declare -p prints `declare -x NAME=…` for an
            # exported variable and errors for an unset one.
            case "$(declare -p "$__ptyline_pat" 2>/dev/null)" in
            "declare -x "*) __ptyline_names=$__ptyline_pat ;;
            *) __ptyline_names= ;;
            esac
            ;;
        esac
        for __ptyline_name in $__ptyline_names; do
            __ptyline_value=${!__ptyline_name}
            [ -z "$__ptyline_value" ] && continue
            __ptyline_b64=$(printf '%s' "$__ptyline_value" | base64 | tr -d '\n')
            __ptyline_out="${__ptyline_out:+$__ptyline_out;}$__ptyline_name=$__ptyline_b64"
        done
    done
    __ptyline_emit exec_env "$PTYLINE_NONCE:$__ptyline_out"
}

# DEBUG fires before each command; capture the first command of the line and its
# start time, ignoring our own precmd hook and any sub-commands run inside
# PROMPT_COMMAND (guarded by __ptyline_in_prompt).
__ptyline_preexec() {
    [ -n "$__ptyline_in_prompt" ] && return  # inside PROMPT_COMMAND — not a user command
    case "$BASH_COMMAND" in
    __ptyline_precmd*) return ;;
    esac
    [ -n "$__ptyline_running" ] && return
    __ptyline_running=1
    __ptyline_cmd=$BASH_COMMAND
    __ptyline_start=$(__ptyline_now_ms)
    __ptyline_emit command "$__ptyline_cmd"
    __ptyline_emit_env
    __ptyline_emit_exec_env
}

# PROMPT_COMMAND runs before each prompt; report exit code, cwd, and (if a command
# actually ran) its duration, then clear the active command.
__ptyline_precmd() {
    __ptyline_exit=$?
    __ptyline_in_prompt=1
    if [ -n "$__ptyline_running" ]; then
        __ptyline_emit duration_ms "$(($(__ptyline_now_ms) - __ptyline_start))"
        __ptyline_running=
    fi
    __ptyline_emit exit_code "$__ptyline_exit"
    __ptyline_emit cwd "$PWD"
    __ptyline_emit_env
    __ptyline_emit_exec_env
    __ptyline_emit command ""
    __ptyline_in_prompt=
}

# Append to any existing DEBUG trap rather than replacing it, so bash-preexec
# and other DEBUG-based tools keep working alongside ptyline.
__ptyline_existing_debug=$(trap -p DEBUG 2>/dev/null)
if [ -n "$__ptyline_existing_debug" ]; then
    __ptyline_existing_debug=$(printf '%s' "$__ptyline_existing_debug" | sed "s/^trap -- '\\(.*\\)' DEBUG\$/\\1/")
    trap "${__ptyline_existing_debug}; __ptyline_preexec" DEBUG
else
    trap '__ptyline_preexec' DEBUG
fi
unset __ptyline_existing_debug
case "$PROMPT_COMMAND" in
*__ptyline_precmd*) ;;
*) PROMPT_COMMAND="__ptyline_precmd${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
esac

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
