// Package completion emits shell completion scripts for pim.
package completion

import (
	"fmt"
	"io"
)

// Bash writes a bash completion script to w.
func Bash(w io.Writer) {
	fmt.Fprint(w, `_pim_completion() {
    local cur prev words cword
    _init_completion || return

    local commands="activate deactivate status completion version help"
    local flags="--role --scope --time --justification --yes --headless --output --config-dir"

    case "$prev" in
        --output|-o)
            COMPREPLY=( $(compgen -W "table json" -- "$cur") )
            return ;;
        --time|-t)
            COMPREPLY=( $(compgen -W "30m 1h 2h 4h 8h" -- "$cur") )
            return ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
            return ;;
    esac

    if [[ "$cur" == -* ]]; then
        COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
    else
        COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
    fi
}

complete -F _pim_completion pim
`)
}

// Zsh writes a zsh completion script to w.
func Zsh(w io.Writer) {
	fmt.Fprint(w, `#compdef pim

_pim() {
    local -a commands
    commands=(
        'activate:activate roles via TUI wizard'
        'deactivate:deactivate active role elevations'
        'status:view active and eligible roles'
        'completion:print shell completion script'
        'version:print version'
        'help:show help'
    )

    local -a activate_flags
    activate_flags=(
        '--role[role name filter (repeatable)]:role name'
        '--scope[scope path (repeatable)]:scope path'
        '--time[activation duration (e.g. 1h, 30m)]:duration:(30m 1h 2h 4h 8h)'
        '--justification[justification text]:text'
        '--yes[skip confirmation]'
        '--headless[non-TUI mode]'
        '--output[output format]:format:(table json)'
        '--config-dir[override config directory]:dir:_directories'
    )

    if (( CURRENT == 2 )); then
        _describe 'command' commands
        return
    fi

    case "$words[2]" in
        activate)
            _arguments $activate_flags ;;
        completion)
            _values 'shell' bash zsh fish ;;
    esac
}

_pim
`)
}

// Fish writes a fish completion script to w.
func Fish(w io.Writer) {
	fmt.Fprint(w, `# pim fish completions

set -l commands activate deactivate status completion version help

complete -c pim -f -n "not __fish_seen_subcommand_from $commands" \
    -a activate   -d "activate roles via TUI wizard"
complete -c pim -f -n "not __fish_seen_subcommand_from $commands" \
    -a deactivate -d "deactivate active role elevations"
complete -c pim -f -n "not __fish_seen_subcommand_from $commands" \
    -a status     -d "view active and eligible roles"
complete -c pim -f -n "not __fish_seen_subcommand_from $commands" \
    -a completion -d "print shell completion script"
complete -c pim -f -n "not __fish_seen_subcommand_from $commands" \
    -a version    -d "print version"

# completion subcommand shells
complete -c pim -f -n "__fish_seen_subcommand_from completion" \
    -a "bash zsh fish"

# activate flags
complete -c pim -n "__fish_seen_subcommand_from activate" \
    -l role          -d "role name filter (repeatable)"
complete -c pim -n "__fish_seen_subcommand_from activate" \
    -l scope         -d "scope path (repeatable)"
complete -c pim -n "__fish_seen_subcommand_from activate" \
    -l time -s t     -d "activation duration" \
    -a "30m 1h 2h 4h 8h"
complete -c pim -n "__fish_seen_subcommand_from activate" \
    -l justification -s j -d "justification text"
complete -c pim -n "__fish_seen_subcommand_from activate" \
    -l yes -s y      -d "skip confirmation"
complete -c pim -n "__fish_seen_subcommand_from activate" \
    -l headless      -d "non-TUI mode"
complete -c pim -n "__fish_seen_subcommand_from activate" \
    -l output -s o   -d "output format" \
    -a "table json"
`)
}
