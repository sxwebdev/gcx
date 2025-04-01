package helpers_test

import (
	"os/user"
	"testing"

	"github.com/sxwebdev/gcx/internal/helpers"
)

func TestExpandPath(t *testing.T) {
	// get current user's home directory for constructing expected outputs.
	usr, err := user.Current()
	if err != nil {
		t.Fatalf("failed to get current user: %v", err)
	}

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "~/folder",
			expected: usr.HomeDir + "/folder",
		},
		{
			input:    "~/",
			expected: usr.HomeDir + "/",
		},
		{
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			input:    "relative/path",
			expected: "relative/path",
		},
	}

	for _, tt := range tests {
		got, err := helpers.ExpandPath(tt.input)
		if err != nil {
			t.Errorf("ExpandPath(%q) returned error: %v", tt.input, err)
		}
		if got != tt.expected {
			t.Errorf("ExpandPath(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}
