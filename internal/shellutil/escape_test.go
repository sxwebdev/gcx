package shellutil

import "testing"

func TestQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"", "''"},
		{"it's", "'it'\\''s'"},
		{"foo;bar", "'foo;bar'"},
		{"$(cmd)", "'$(cmd)'"},
		{"`cmd`", "'`cmd`'"},
		{"a'b'c", "'a'\\''b'\\''c'"},
		{"/path/to/dir", "'/path/to/dir'"},
		{"hello\nworld", "'hello\nworld'"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Quote(tt.input)
			if got != tt.want {
				t.Errorf("Quote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
