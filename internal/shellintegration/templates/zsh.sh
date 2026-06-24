# ptyline zsh integration — printed by `ptyline init zsh`.
# Source it from ~/.zshrc:  eval "$(ptyline init zsh)"
#
# Emits OSC 777 key=value metadata (cwd, exit_code, command, duration_ms) that the
# ptyline ANSI/OSC filter consumes. Payloads are strict key=value and never echo
# executable content (spec §9, §17).
#
# TODO scaffold (plan 12): implement precmd/preexec hooks, e.g.
#   __ptyline_precmd() {
#     printf '\e]777;exit_code=%s\e\\' "$?"
#     printf '\e]777;cwd=%s\e\\' "$PWD"
#   }
#   __ptyline_preexec() { printf '\e]777;command=%s\e\\' "$1"; }  # + start time → duration_ms
#   autoload -Uz add-zsh-hook
#   add-zsh-hook precmd __ptyline_precmd
#   add-zsh-hook preexec __ptyline_preexec
:
