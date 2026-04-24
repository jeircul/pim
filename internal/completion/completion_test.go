package completion

import (
	"strings"
	"testing"
)

func TestBash(t *testing.T) {
	var sb strings.Builder
	Bash(&sb)
	out := sb.String()
	if !strings.Contains(out, "_pim_completion") {
		t.Error("bash completion missing _pim_completion function")
	}
	if !strings.Contains(out, "complete -F _pim_completion pim") {
		t.Error("bash completion missing complete registration")
	}
}

func TestZsh(t *testing.T) {
	var sb strings.Builder
	Zsh(&sb)
	out := sb.String()
	if !strings.Contains(out, "#compdef pim") {
		t.Error("zsh completion missing #compdef pim")
	}
	if !strings.Contains(out, "_pim") {
		t.Error("zsh completion missing _pim function")
	}
}

func TestFish(t *testing.T) {
	var sb strings.Builder
	Fish(&sb)
	out := sb.String()
	if !strings.Contains(out, "complete -c pim") {
		t.Error("fish completion missing complete -c pim")
	}
	if !strings.Contains(out, "activate") {
		t.Error("fish completion missing activate subcommand")
	}
}
