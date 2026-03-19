package shellutil

import "strings"

// Quote returns a shell-safe single-quoted version of the string.
// It handles strings containing single quotes by ending the quote,
// adding an escaped single quote, and re-opening the quote.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
