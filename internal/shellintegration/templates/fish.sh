# ptyline fish integration — printed by `ptyline init fish`.
# Enable it from config.fish:  ptyline init fish | source
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17). Values are emitted in canonical form here —
# exit_code as a plain integer, duration_ms already in milliseconds, cwd absolute
# — so the Go side consumes one representation for every shell. (fish syntax; the
# .sh extension is only the template naming convention.)

function __ptyline_emit
    printf '\e]777;%s=%s\e\\' $argv[1] $argv[2]
end

function __ptyline_preexec --on-event fish_preexec
    set -g __ptyline_cmd $argv[1]
    set -g __ptyline_start (date +%s%3N)
    __ptyline_emit command "$__ptyline_cmd"
end

function __ptyline_postexec --on-event fish_postexec
    set -l code $status
    if set -q __ptyline_start
        __ptyline_emit duration_ms (math (date +%s%3N) - $__ptyline_start)
        set -e __ptyline_start
    end
    __ptyline_emit exit_code $code
    __ptyline_emit cwd "$PWD"
    __ptyline_emit command ""
end

function __ptyline_emit_cwd --on-variable PWD
    __ptyline_emit cwd "$PWD"
end

__ptyline_emit_cwd
