package chat

import "testing"

func TestFormatDecorationLayout(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "GNOME default appmenu:close",
			input:    "appmenu:close",
			expected: "appmenu:minimize,maximize,close",
		},
		{
			name:     "Standard right side close only :close",
			input:    ":close",
			expected: ":minimize,maximize,close",
		},
		{
			name:     "Empty layout defaults to right side buttons",
			input:    "",
			expected: ":minimize,maximize,close",
		},
		{
			name:     "Left side close (macOS style) close:",
			input:    "close:",
			expected: "close,minimize,maximize:",
		},
		{
			name:     "Left side close with appmenu close,appmenu:",
			input:    "close,appmenu:",
			expected: "close,appmenu,minimize,maximize:",
		},
		{
			name:     "Already has minimize and maximize on right",
			input:    ":minimize,maximize,close",
			expected: ":minimize,maximize,close",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatDecorationLayout(tc.input)
			if got != tc.expected {
				t.Errorf("FormatDecorationLayout(%q) = %q, expected %q", tc.input, got, tc.expected)
			}
		})
	}
}
