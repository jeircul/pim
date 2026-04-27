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
    local common_flags="--role -r --scope --time -t --justification -j --yes -y --headless --output -o --config-dir"
    local activate_flags="$common_flags"
    local deactivate_flags="--role -r --scope --headless --output -o --config-dir"
    local status_flags="--role -r --scope --headless --output -o --config-dir"

    case "$prev" in
        --output|-o)
            COMPREPLY=( $(compgen -W "table json" -- "$cur") )
            return ;;
        --time|-t)
            COMPREPLY=( $(compgen -W "30m 1h 2h 4h 8h" -- "$cur") )
            return ;;
        --config-dir)
            _filedir -d
            return ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
            return ;;
    esac

    local subcmd=""
    local i
    for (( i=1; i < cword; i++ )); do
        if [[ "${words[i]}" != -* ]]; then
            subcmd="${words[i]}"
            break
        fi
    done

    if [[ "$cur" == -* ]]; then
        case "$subcmd" in
            activate)
                COMPREPLY=( $(compgen -W "$activate_flags" -- "$cur") ) ;;
            deactivate)
                COMPREPLY=( $(compgen -W "$deactivate_flags" -- "$cur") ) ;;
            status)
                COMPREPLY=( $(compgen -W "$status_flags" -- "$cur") ) ;;
            version|help)
                COMPREPLY=( $(compgen -W "--config-dir" -- "$cur") ) ;;
            *)
                COMPREPLY=( $(compgen -W "$common_flags" -- "$cur") ) ;;
        esac
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
        '-r[role name filter (repeatable)]:role name'
        '--scope[scope path (repeatable)]:scope path'
        '--time[activation duration (e.g. 1h, 30m)]:duration:(30m 1h 2h 4h 8h)'
        '-t[activation duration (e.g. 1h, 30m)]:duration:(30m 1h 2h 4h 8h)'
        '--justification[justification text]:text'
        '-j[justification text]:text'
        '--yes[skip confirmation]'
        '-y[skip confirmation]'
        '--headless[non-TUI mode]'
        '--output[output format]:format:(table json)'
        '-o[output format]:format:(table json)'
        '--config-dir[override config directory]:dir:_directories'
    )

    local -a deactivate_flags
    deactivate_flags=(
        '--role[role name filter (repeatable)]:role name'
        '-r[role name filter (repeatable)]:role name'
        '--scope[scope path (repeatable)]:scope path'
        '--headless[non-TUI mode]'
        '--output[output format]:format:(table json)'
        '-o[output format]:format:(table json)'
        '--config-dir[override config directory]:dir:_directories'
    )

    local -a status_flags
    status_flags=(
        '--role[role name filter (repeatable)]:role name'
        '-r[role name filter (repeatable)]:role name'
        '--scope[scope path (repeatable)]:scope path'
        '--headless[non-TUI mode]'
        '--output[output format]:format:(table json)'
        '-o[output format]:format:(table json)'
        '--config-dir[override config directory]:dir:_directories'
    )

    local -a base_flags
    base_flags=(
        '--config-dir[override config directory]:dir:_directories'
    )

    if (( CURRENT == 2 )); then
        _describe 'command' commands
        return
    fi

    case "$words[2]" in
        activate)
            _arguments "${activate_flags[@]}" ;;
        deactivate)
            _arguments "${deactivate_flags[@]}" ;;
        status)
            _arguments "${status_flags[@]}" ;;
        completion)
            _values 'shell' bash zsh fish ;;
        version|help)
            _arguments "${base_flags[@]}" ;;
    esac
}

compdef _pim pim
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
    -l role -s r     -d "role name filter (repeatable)"
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
complete -c pim -n "__fish_seen_subcommand_from activate" \
    -l config-dir    -d "override config directory"

# deactivate flags
complete -c pim -n "__fish_seen_subcommand_from deactivate" \
    -l role -s r     -d "role name filter (repeatable)"
complete -c pim -n "__fish_seen_subcommand_from deactivate" \
    -l scope         -d "scope path (repeatable)"
complete -c pim -n "__fish_seen_subcommand_from deactivate" \
    -l headless      -d "non-TUI mode"
complete -c pim -n "__fish_seen_subcommand_from deactivate" \
    -l output -s o   -d "output format" \
    -a "table json"
complete -c pim -n "__fish_seen_subcommand_from deactivate" \
    -l config-dir    -d "override config directory"

# status flags
complete -c pim -n "__fish_seen_subcommand_from status" \
    -l role -s r     -d "role name filter (repeatable)"
complete -c pim -n "__fish_seen_subcommand_from status" \
    -l scope         -d "scope path (repeatable)"
complete -c pim -n "__fish_seen_subcommand_from status" \
    -l headless      -d "non-TUI mode"
complete -c pim -n "__fish_seen_subcommand_from status" \
    -l output -s o   -d "output format" \
    -a "table json"
complete -c pim -n "__fish_seen_subcommand_from status" \
    -l config-dir    -d "override config directory"

# version/help flags
complete -c pim -n "__fish_seen_subcommand_from version help" \
    -l config-dir    -d "override config directory"
`)
}
