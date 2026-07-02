# ptyline fish integration — printed by `ptyline init fish`.
# Enable it from config.fish:  ptyline init fish | source
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms, env) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17). Values are emitted in canonical form here —
# exit_code as a plain integer, duration_ms already in milliseconds, cwd absolute
# — so the Go side consumes one representation for every shell. (fish syntax; the
# .sh extension is only the template naming convention.)

function __ptyline_emit
    printf '\e]777;%s=%s\e\\' $argv[1] $argv[2]
end

function __ptyline_emit_colors
    set -l pairs \
        "cwd=$fish_color_cwd" \
        "host=$fish_color_host" \
        "host_remote=$fish_color_host_remote" \
        "user=$fish_color_user" \
        "error=$fish_color_error" \
        "status=$fish_color_status" \
        "command=$fish_color_command"
    __ptyline_emit colors (string join ";" $pairs)
end

function __ptyline_emit_env
    if test -z "$PTYLINE_ENV_NAMES"
        return
    end
    set -l names (string split , -- "$PTYLINE_ENV_NAMES")
    set -l count (count $names)
    set -l out
    for name in $names
        if not string match -rq '^[A-Za-z_][A-Za-z0-9_]*$' -- "$name"
            continue
        end
        set -l value $$name
        if test -z "$value"
            continue
        end
        if test "$count" -eq 1
            set out "$value"
        else
            set -a out "$name=$value"
        end
    end
    __ptyline_emit env (string join ' ' $out)
end

# Mirror selected exported variables to ptyline so exec modules ({gh}, …) run with
# the shell's live environment. Each pattern is an exact name or a NAME* prefix;
# values are base64-encoded (so ';'/'='/control chars can't corrupt the frame) and
# the whole frame carries $PTYLINE_NONCE so injected bytes can't forge it.
function __ptyline_emit_exec_env
    if test -z "$PTYLINE_EXEC_ENV_NAMES"; or test -z "$PTYLINE_NONCE"
        return
    end
    set -l patterns (string split , -- "$PTYLINE_EXEC_ENV_NAMES")
    set -l out
    for pat in $patterns
        if not string match -rq '^[A-Za-z_][A-Za-z0-9_]*\*?$' -- "$pat"
            continue
        end
        for name in (set --names)
            string match -q -- "$pat" "$name"; or continue
            set -qx $name; or continue
            set -l value (string join ':' -- $$name)
            test -z "$value"; and continue
            set -l b64 (printf '%s' "$value" | base64 | tr -d '\n')
            set -a out "$name=$b64"
        end
    end
    __ptyline_emit exec_env "$PTYLINE_NONCE:"(string join ';' $out)
end

# Returns current time as an integer number of milliseconds.
# Uses $EPOCHREALTIME (fish ≥ 3.4) with the decimal point normalized to '.' so
# the result is correct regardless of the system locale (e.g. Russian locale uses
# ',' as decimal separator). Falls back to date +%s (1 s precision) on older fish
# or when EPOCHREALTIME is unavailable — macOS date lacks the GNU %N extension.
function __ptyline_ms_now
    set -l _t (string replace --all ',' '.' -- $EPOCHREALTIME 2>/dev/null)
    if test -n "$_t"
        math -s0 "$_t * 1000"
    else
        math (date +%s) \* 1000
    end
end

function __ptyline_preexec --on-event fish_preexec
    set -g __ptyline_cmd $argv[1]
    set -g __ptyline_start (__ptyline_ms_now)
    __ptyline_emit command "$__ptyline_cmd"
    __ptyline_emit_env
    __ptyline_emit_exec_env
end

function __ptyline_postexec --on-event fish_postexec
    set -l code $status
    if set -q __ptyline_start
        __ptyline_emit duration_ms (math -s0 (__ptyline_ms_now) - $__ptyline_start)
        set -e __ptyline_start
    end
    __ptyline_emit exit_code $code
    __ptyline_emit_cwd
    __ptyline_emit_env
    __ptyline_emit_exec_env
    __ptyline_emit command ""
end

# cwd is nonce-tagged like exec_env: ptyline runs exec-module commands and
# discovers project .ptyline files from it, so a forged OSC 777 cwd (printed
# from a file or command output) must not be able to redirect them.
function __ptyline_emit_cwd --on-variable PWD
    __ptyline_emit cwd "$PTYLINE_NONCE:$PWD"
end

# Wrap ssh to report outbound connections to the ptyline status bar.
# Use `command ssh` to bypass this wrapper when needed.
function ssh
    set -l _ptyline_host ""
    for _ptyline_a in $argv
        string match -q -- '-*' $_ptyline_a; and continue
        set _ptyline_host $_ptyline_a
        break
    end
    __ptyline_emit ssh_start "$_ptyline_host"
    command ssh $argv
    set -l _code $status
    __ptyline_emit ssh_end ""
    return $_code
end

__ptyline_emit_colors
__ptyline_emit_cwd
__ptyline_emit_env
__ptyline_emit_exec_env
