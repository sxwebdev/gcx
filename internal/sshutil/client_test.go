package sshutil

import "testing"

func TestClientConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ClientConfig
		wantErr bool
	}{
		{
			name:    "empty config",
			cfg:     ClientConfig{},
			wantErr: true,
		},
		{
			name:    "missing user",
			cfg:     ClientConfig{Server: "host"},
			wantErr: true,
		},
		{
			name:    "missing key",
			cfg:     ClientConfig{Server: "host", User: "user"},
			wantErr: true,
		},
		{
			name:    "both keys",
			cfg:     ClientConfig{Server: "host", User: "user", KeyPath: "/key", KeyRaw: "raw"},
			wantErr: true,
		},
		{
			name:    "valid with key_path",
			cfg:     ClientConfig{Server: "host", User: "user", KeyPath: "/key"},
			wantErr: false,
		},
		{
			name:    "valid with key_raw",
			cfg:     ClientConfig{Server: "host", User: "user", KeyRaw: "raw"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
