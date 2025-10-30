package cli

import (
	"strings"
	"testing"
)

func TestTrimInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Simple string", "hello", "hello"},
		{"With spaces", "  hello  ", "hello"},
		{"With newline", "hello\n", "hello"},
		{"With carriage return", "hello\r\n", "hello"},
		{"Multiple newlines", "hello\n\n\n", "hello"},
		{"Empty string", "", ""},
		{"Only whitespace", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimInput(tt.input)
			if result != tt.expected {
				t.Errorf("trimInput(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		expectErr bool
		errMsg    string
	}{
		{
			name: "Valid activation config",
			config: Config{
				Justification: "Testing",
				Hours:         4,
				Deactivate:    false,
			},
			expectErr: false,
		},
		{
			name: "Valid deactivation config",
			config: Config{
				Justification: "",
				Hours:         1,
				Deactivate:    true,
			},
			expectErr: false,
		},
		{
			name: "Missing justification for activation",
			config: Config{
				Justification: "",
				Hours:         1,
				Deactivate:    false,
			},
			expectErr: true,
			errMsg:    "justification required",
		},
		{
			name: "Hours too low",
			config: Config{
				Justification: "Testing",
				Hours:         0,
				Deactivate:    false,
			},
			expectErr: true,
			errMsg:    "hours must be between",
		},
		{
			name: "Hours too high",
			config: Config{
				Justification: "Testing",
				Hours:         10,
				Deactivate:    false,
			},
			expectErr: true,
			errMsg:    "hours must be between",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}
