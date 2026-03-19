package tmpl

import (
	"fmt"
	"strings"
	"text/template"
)

// Process parses and executes a Go text/template with the given data.
func Process(name, tmplStr string, data any) (string, error) {
	t, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", name, err)
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %q: %w", name, err)
	}
	return buf.String(), nil
}
