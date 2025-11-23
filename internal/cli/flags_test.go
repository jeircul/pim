package cli

import (
	"reflect"
	"testing"
)

func TestParseArgsActivate(t *testing.T) {
	cmd, err := ParseArgs([]string{"activate", "-j", "Work", "--mg", "demo", "--sub", "alpha"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Kind != CommandActivate {
		t.Fatalf("expected activate command, got %v", cmd.Kind)
	}
	if cmd.Activate.Justification != "Work" {
		t.Fatalf("unexpected justification %q", cmd.Activate.Justification)
	}
	if len(cmd.Activate.ManagementGroups) != 1 || cmd.Activate.ManagementGroups[0] != "demo" {
		t.Fatalf("unexpected management group filters: %#v", cmd.Activate.ManagementGroups)
	}
}

func TestParseArgsRejectsLegacyShorthand(t *testing.T) {
	if _, err := ParseArgs([]string{"-j", "Legacy"}); err == nil {
		t.Fatal("expected error when using legacy shorthand")
	}
}

func TestParseArgsRequireJustification(t *testing.T) {
	cmd, err := ParseArgs([]string{"activate"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Kind != CommandActivate {
		t.Fatalf("expected activate command, got %v", cmd.Kind)
	}
	if !cmd.Activate.NeedsJustification() {
		t.Fatalf("expected justification prompt to be required")
	}
}

func TestParseArgsStatus(t *testing.T) {
	cmd, err := ParseArgs([]string{"status"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Kind != CommandStatus {
		t.Fatalf("expected status command, got %v", cmd.Kind)
	}
}

func TestParseArgsHelp(t *testing.T) {
	cmd, err := ParseArgs([]string{"help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Kind != CommandHelp {
		t.Fatalf("expected help command, got %v", cmd.Kind)
	}
}

func TestParseArgsNoArgsShowsPrompt(t *testing.T) {
	cmd, err := ParseArgs([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Kind != CommandPrompt {
		t.Fatalf("expected prompt command, got %v", cmd.Kind)
	}
}

func TestActivateConfigValidateMinutes(t *testing.T) {
	cases := []struct {
		name    string
		minutes int
		err     bool
	}{
		{"min", 30, false},
		{"one hour", 60, false},
		{"max", 480, false},
		{"below", 0, true},
		{"above", 600, true},
		{"not 30 min increment", 45, true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ActivateConfig{Justification: "", Minutes: tt.minutes}
			err := cfg.Validate()
			if tt.err && err == nil {
				t.Fatalf("expected error for minutes=%d", tt.minutes)
			}
			if !tt.err && err != nil {
				t.Fatalf("unexpected error for minutes=%d: %v", tt.minutes, err)
			}
		})
	}
}

func TestStringSliceFlag(t *testing.T) {
	var f stringSliceFlag
	if err := f.Set("one"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := f.Set("two"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := f.Slice()
	expected := []string{"one", "two"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}
