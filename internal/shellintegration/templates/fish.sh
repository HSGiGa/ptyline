# ptyline fish integration — printed by `ptyline init fish`.
# Source it from config.fish:  ptyline init fish | source
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17). (fish syntax; .sh extension is just the
# template naming convention.)
#
function __ptyline_emit_cwd --on-variable PWD
    printf '\e]777;cwd=%s\e\\' "$PWD"
end

__ptyline_emit_cwd
