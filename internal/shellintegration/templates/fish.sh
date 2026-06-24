# ptyline fish integration — printed by `ptyline init fish`.
# Source it from config.fish:  ptyline init fish | source
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17). (fish syntax; .sh extension is just the
# template naming convention.)
#
# TODO scaffold (plan 12): implement real hooks, e.g.
#   function __ptyline_postexec --on-event fish_postexec
#       printf '\e]777;exit_code=%s\e\\' $status
#       printf '\e]777;command=%s\e\\' "$argv"
#   end
#   function __ptyline_pwd --on-variable PWD
#       printf '\e]777;cwd=%s\e\\' "$PWD"
#   end
