package ui

import "testing"

func TestCleanPhoneNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Formatted Brazilian number with plus",
			input:    "+55 11 98765-4321",
			expected: "5511987654321",
		},
		{
			name:     "Number with parentheses and spaces",
			input:    "+55 (21) 91234 5678",
			expected: "5521912345678",
		},
		{
			name:     "Clean digits already",
			input:    "5511987654321",
			expected: "5511987654321",
		},
		{
			name:     "International format with symbols",
			input:    "+1 (555) 234-5678",
			expected: "15552345678",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Text with letters and symbols",
			input:    "phone: +55-11-9999-8888 ext 12",
			expected: "55119999888812",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanPhoneNumber(tt.input)
			if got != tt.expected {
				t.Errorf("CleanPhoneNumber(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}
