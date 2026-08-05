package service

import (
	"reflect"
	"testing"
)

func TestParseCommandString(t *testing.T) {
	tests := []struct {
		input    string
		expected *ParsedCommand
	}{
		{
			input: "/blacklist 08123456789 --reason=spam",
			expected: &ParsedCommand{
				Name: "blacklist",
				Args: []string{"08123456789"},
				Flags: map[string]string{
					"reason": "spam",
				},
			},
		},
		{
			input: "/blacklist add 08123456789 --reason=fraud",
			expected: &ParsedCommand{
				Name: "blacklist",
				Args: []string{"add", "08123456789"},
				Flags: map[string]string{
					"reason": "fraud",
				},
			},
		},
		{
			input: "/blacklist remove 08123456789",
			expected: &ParsedCommand{
				Name:  "blacklist",
				Args:  []string{"remove", "08123456789"},
				Flags: map[string]string{},
			},
		},
		{
			input: "/blacklist --help",
			expected: &ParsedCommand{
				Name: "blacklist",
				Args: []string{},
				Flags: map[string]string{
					"help": "true",
				},
			},
		},
		{
			input: "/help",
			expected: &ParsedCommand{
				Name:  "help",
				Args:  []string{},
				Flags: map[string]string{},
			},
		},
		{
			input:    "hello world",
			expected: nil,
		},
	}

	for _, tt := range tests {
		got := ParseCommandString(tt.input)
		if !reflect.DeepEqual(got, tt.expected) {
			t.Errorf("ParseCommandString(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}

	helpCmd := ParseCommandString("/blacklist --help")
	if !helpCmd.HasHelpFlag() {
		t.Errorf("expected HasHelpFlag() to be true")
	}
}
