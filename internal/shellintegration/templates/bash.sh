# ptyline bash integration — printed by `ptyline init bash`.
# Source it from ~/.bashrc:  eval "$(ptyline init bash)"
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17).
#
# TODO scaffold (plan 12): implement real hooks, e.g.
#   __ptyline_precmd() {
#     local ec=$?
#     printf '\e]777;exit_code=%s\e\\' "$ec"
#     printf '\e]777;cwd=%s\e\\' "$PWD"
#   }
#   PROMPT_COMMAND="__ptyline_precmd${PROMPT_COMMAND:+; $PROMPT_COMMAND}"
#   # plus a DEBUG trap for command + duration_ms
:
