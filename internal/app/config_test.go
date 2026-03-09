package app

import (
	"errors"
	"flag"
	"testing"
)

func TestParse_commands(t *testing.T) {
	tests := []struct {
		args        []string
		wantCmd     string
		wantVersion bool
		wantErr     bool
	}{
		{nil, "", false, false},
		{[]string{"activate"}, CmdActivate, false, false},
		{[]string{"a"}, CmdActivate, false, false},
		{[]string{"deactivate"}, CmdDeactivate, false, false},
		{[]string{"d"}, CmdDeactivate, false, false},
		{[]string{"off"}, CmdDeactivate, false, false},
		{[]string{"status"}, CmdStatus, false, false},
		{[]string{"s"}, CmdStatus, false, false},
		{[]string{"version"}, "", true, false},
		{[]string{"v"}, "", true, false},
		{[]string{"completion", "bash"}, CmdCompletion, false, false},
		{[]string{"help"}, "", false, true},
		{[]string{"-h"}, "", false, true},
	}

	for _, tc := range tests {
		cfg, err := Parse(tc.args)
		if tc.wantErr {
			if !errors.Is(err, flag.ErrHelp) {
				t.Errorf("Parse(%v) err = %v, want flag.ErrHelp", tc.args, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%v) unexpected err: %v", tc.args, err)
			continue
		}
		if cfg.Command != tc.wantCmd {
			t.Errorf("Parse(%v) Command = %q, want %q", tc.args, cfg.Command, tc.wantCmd)
		}
		if cfg.Version != tc.wantVersion {
			t.Errorf("Parse(%v) Version = %v, want %v", tc.args, cfg.Version, tc.wantVersion)
		}
	}
}

func TestParse_activationFlags(t *testing.T) {
	cfg, err := Parse([]string{
		"activate",
		"--role", "Reader",
		"--role", "Owner",
		"--scope", "/sub/aaa",
		"--time", "2h",
		"--justification", "ticket",
		"--yes",
		"--headless",
		"--output", "json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Command != CmdActivate {
		t.Errorf("Command = %q, want activate", cfg.Command)
	}
	if len(cfg.Roles) != 2 {
		t.Errorf("Roles = %v, want 2", cfg.Roles)
	}
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != "/sub/aaa" {
		t.Errorf("Scopes = %v, want [/sub/aaa]", cfg.Scopes)
	}
	if cfg.TimeStr != "2h" {
		t.Errorf("TimeStr = %q, want 2h", cfg.TimeStr)
	}
	if cfg.Justification != "ticket" {
		t.Errorf("Justification = %q, want ticket", cfg.Justification)
	}
	if !cfg.Yes {
		t.Error("Yes should be true")
	}
	if !cfg.Headless {
		t.Error("Headless should be true")
	}
	if cfg.Output != OutputJSON {
		t.Errorf("Output = %v, want json", cfg.Output)
	}
}

func TestParse_completionShell(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		cfg, err := Parse([]string{"completion", shell})
		if err != nil {
			t.Fatalf("completion %s: %v", shell, err)
		}
		if cfg.Command != CmdCompletion {
			t.Errorf("Command = %q, want completion", cfg.Command)
		}
		if cfg.CompletionShell != shell {
			t.Errorf("CompletionShell = %q, want %s", cfg.CompletionShell, shell)
		}
	}
}

func TestCanAutoAdvance(t *testing.T) {
	full := Config{
		Roles:         []string{"Reader"},
		Scopes:        []string{"/sub/aaa"},
		TimeStr:       "1h",
		Justification: "ticket",
	}
	if !full.CanAutoAdvance() {
		t.Error("full config should CanAutoAdvance")
	}

	partial := full
	partial.TimeStr = ""
	if partial.CanAutoAdvance() {
		t.Error("partial config should not CanAutoAdvance")
	}
}
