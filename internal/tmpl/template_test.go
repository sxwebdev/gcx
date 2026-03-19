package tmpl

import "testing"

func TestProcess(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		data    any
		want    string
		wantErr bool
	}{
		{
			name: "simple",
			tmpl: "hello {{.Name}}",
			data: struct{ Name string }{"world"},
			want: "hello world",
		},
		{
			name: "map data",
			tmpl: "v={{.Version}}",
			data: map[string]string{"Version": "1.0.0"},
			want: "v=1.0.0",
		},
		{
			name: "nested",
			tmpl: "-X main.version={{.Version}} -X main.env={{.Env.TOKEN}}",
			data: struct {
				Version string
				Env     map[string]string
			}{
				Version: "v1.0.0",
				Env:     map[string]string{"TOKEN": "abc"},
			},
			want: "-X main.version=v1.0.0 -X main.env=abc",
		},
		{
			name:    "invalid template",
			tmpl:    "{{.Invalid",
			data:    nil,
			wantErr: true,
		},
		{
			name:    "missing field",
			tmpl:    "{{.Missing}}",
			data:    struct{}{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Process(tt.name, tt.tmpl, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Process() = %q, want %q", got, tt.want)
			}
		})
	}
}
