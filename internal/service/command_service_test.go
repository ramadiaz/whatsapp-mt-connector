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

func TestParsePaymentBilling(t *testing.T) {
	tests := []struct {
		input       string
		expectedAmt float64
		expectedTel string
		expectedDes string
		wantErr     bool
	}{
		{
			input:       "/payment tagih 50rb ke 6281312345678 bayar kopi",
			expectedAmt: 50000,
			expectedTel: "6281312345678",
			expectedDes: "bayar kopi",
			wantErr:     false,
		},
		{
			input:       "/payment tagih 100k ke 081234567890 makan siang bareng",
			expectedAmt: 100000,
			expectedTel: "6281234567890",
			expectedDes: "makan siang bareng",
			wantErr:     false,
		},
		{
			input:       "/payment 1.5jt ke 628991122334 freelance web",
			expectedAmt: 1500000,
			expectedTel: "628991122334",
			expectedDes: "freelance web",
			wantErr:     false,
		},
		{
			input:       "/payment 25000 628130000000",
			expectedAmt: 25000,
			expectedTel: "628130000000",
			expectedDes: "Tagihan Pembayaran",
			wantErr:     false,
		},
		{
			input:   "/payment",
			wantErr: true,
		},
		{
			input:   "/payment tagih",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		res, err := parsePaymentBilling(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parsePaymentBilling(%q) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePaymentBilling(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if res.Amount != tt.expectedAmt {
			t.Errorf("parsePaymentBilling(%q) amount = %v, want %v", tt.input, res.Amount, tt.expectedAmt)
		}
		if res.TargetPhone != tt.expectedTel {
			t.Errorf("parsePaymentBilling(%q) phone = %v, want %v", tt.input, res.TargetPhone, tt.expectedTel)
		}
		if res.Description != tt.expectedDes {
			t.Errorf("parsePaymentBilling(%q) desc = %v, want %v", tt.input, res.Description, tt.expectedDes)
		}
	}
}
