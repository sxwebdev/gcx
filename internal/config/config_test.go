package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid config", func(t *testing.T) {
		path := filepath.Join(dir, "valid.yaml")
		data := `
out_dir: dist
builds:
  - main: ./cmd/app
    goos: [linux]
    goarch: [amd64]
`
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.OutDir != "dist" {
			t.Errorf("OutDir = %q, want %q", cfg.OutDir, "dist")
		}
		if len(cfg.Builds) != 1 {
			t.Errorf("len(Builds) = %d, want 1", len(cfg.Builds))
		}
	})

	t.Run("default out_dir", func(t *testing.T) {
		path := filepath.Join(dir, "defaults.yaml")
		data := `
builds:
  - main: ./cmd/app
    goos: [linux]
    goarch: [amd64]
`
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.OutDir != "dist" {
			t.Errorf("OutDir = %q, want %q", cfg.OutDir, "dist")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := Load(filepath.Join(dir, "nonexistent.yaml"))
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := filepath.Join(dir, "invalid.yaml")
		if err := os.WriteFile(path, []byte("builds:\n  - {invalid yaml\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil {
			t.Error("expected error for invalid yaml")
		}
	})
}

func TestConfigValidate(t *testing.T) {
	t.Run("no builds", func(t *testing.T) {
		cfg := &Config{}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for empty builds")
		}
	})

	t.Run("valid minimal", func(t *testing.T) {
		cfg := &Config{
			Builds: []BuildConfig{
				{Main: "./cmd/app", Goos: []string{"linux"}, Goarch: []string{"amd64"}},
			},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid build", func(t *testing.T) {
		cfg := &Config{
			Builds: []BuildConfig{{Main: ""}},
		}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for invalid build")
		}
	})
}

func TestBlobConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     BlobConfig
		wantErr bool
	}{
		{
			name:    "missing name",
			cfg:     BlobConfig{Provider: "s3"},
			wantErr: true,
		},
		{
			name:    "unknown provider",
			cfg:     BlobConfig{Name: "test", Provider: "ftp"},
			wantErr: true,
		},
		{
			name: "valid s3",
			cfg: BlobConfig{
				Name: "test", Provider: "s3",
				Bucket: "b", Endpoint: "https://s3.example.com", Directory: "/releases",
			},
			wantErr: false,
		},
		{
			name: "s3 missing bucket",
			cfg: BlobConfig{
				Name: "test", Provider: "s3",
				Endpoint: "https://s3.example.com", Directory: "/releases",
			},
			wantErr: true,
		},
		{
			name: "valid ssh",
			cfg: BlobConfig{
				Name: "test", Provider: "ssh",
				Server: "host", User: "user", KeyPath: "/key", Directory: "/releases",
			},
			wantErr: false,
		},
		{
			name: "ssh both keys",
			cfg: BlobConfig{
				Name: "test", Provider: "ssh",
				Server: "host", User: "user", KeyPath: "/key", KeyRaw: "raw", Directory: "/d",
			},
			wantErr: true,
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

func TestDeployConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     DeployConfig
		wantErr bool
	}{
		{
			name:    "missing name",
			cfg:     DeployConfig{Provider: "ssh"},
			wantErr: true,
		},
		{
			name: "valid ssh deploy",
			cfg: DeployConfig{
				Name: "prod", Provider: "ssh",
				Server: "host", User: "user", KeyPath: "/key",
				Commands: []string{"systemctl restart app"},
			},
			wantErr: false,
		},
		{
			name: "no commands",
			cfg: DeployConfig{
				Name: "prod", Provider: "ssh",
				Server: "host", User: "user", KeyPath: "/key",
			},
			wantErr: true,
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

func TestArchiveConfigValidate(t *testing.T) {
	t.Run("valid formats", func(t *testing.T) {
		a := ArchiveConfig{Formats: []string{"tar.gz", "zip"}}
		if err := a.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("unsupported format", func(t *testing.T) {
		a := ArchiveConfig{Formats: []string{"rar"}}
		if err := a.Validate(); err == nil {
			t.Error("expected error for unsupported format")
		}
	})
}
