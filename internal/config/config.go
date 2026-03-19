package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the top-level gcx configuration.
type Config struct {
	OutDir      string          `yaml:"out_dir"`
	Concurrency int             `yaml:"concurrency,omitempty"`
	Before      HooksConfig     `yaml:"before,omitempty"`
	After       HooksConfig     `yaml:"after,omitempty"`
	Builds      []BuildConfig   `yaml:"builds,omitempty"`
	Archives    []ArchiveConfig `yaml:"archives,omitempty"`
	Blobs       []BlobConfig    `yaml:"blobs,omitempty"`
	Deploys     []DeployConfig  `yaml:"deploys,omitempty"`
}

// HooksConfig holds shell commands to execute before/after build.
type HooksConfig struct {
	Hooks []string `yaml:"hooks,omitempty"`
}

// BuildConfig defines a cross-compilation build target.
type BuildConfig struct {
	Main                  string   `yaml:"main"`
	OutputName            string   `yaml:"output_name,omitempty"`
	DisablePlatformSuffix bool     `yaml:"disable_platform_suffix,omitempty"`
	Goos                  []string `yaml:"goos"`
	Goarch                []string `yaml:"goarch"`
	Goarm                 []string `yaml:"goarm,omitempty"`
	Flags                 []string `yaml:"flags,omitempty"`
	Ldflags               []string `yaml:"ldflags,omitempty"`
	Env                   []string `yaml:"env,omitempty"`
}

// ArchiveConfig defines how built binaries are archived.
type ArchiveConfig struct {
	Formats      []string `yaml:"formats,omitempty"`
	NameTemplate string   `yaml:"name_template,omitempty"`
}

// BlobConfig defines a publish destination (S3 or SSH).
type BlobConfig struct {
	Provider string `yaml:"provider"`
	Name     string `yaml:"name"`
	// S3 fields
	Bucket   string `yaml:"bucket,omitempty"`
	Region   string `yaml:"region,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"`
	// SSH fields
	Server                string `yaml:"server,omitempty"`
	User                  string `yaml:"user,omitempty"`
	KeyPath               string `yaml:"key_path,omitempty"`
	KeyRaw                string `yaml:"key_raw,omitempty"`
	InsecureIgnoreHostKey bool   `yaml:"insecure_ignore_host_key,omitempty"`
	// Common
	Directory string `yaml:"directory"`
}

// DeployConfig defines a deployment target.
type DeployConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	// SSH fields
	Server                string   `yaml:"server,omitempty"`
	User                  string   `yaml:"user,omitempty"`
	KeyPath               string   `yaml:"key_path,omitempty"`
	KeyRaw                string   `yaml:"key_raw,omitempty"`
	InsecureIgnoreHostKey bool     `yaml:"insecure_ignore_host_key,omitempty"`
	Commands              []string `yaml:"commands"`
	// Alerts
	Alerts AlertConfig `yaml:"alerts,omitempty"`
}

// AlertConfig contains notification settings.
type AlertConfig struct {
	URLs []string `yaml:"urls,omitempty"`
}

// Load reads and parses a YAML configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	if cfg.OutDir == "" {
		cfg.OutDir = "dist"
	}
	return &cfg, nil
}

// Validate checks the entire configuration for correctness.
func (c *Config) Validate() error {
	if len(c.Builds) == 0 {
		return fmt.Errorf("at least one build configuration is required")
	}
	for i, b := range c.Builds {
		if err := b.Validate(); err != nil {
			return fmt.Errorf("builds[%d]: %w", i, err)
		}
	}
	for i, blob := range c.Blobs {
		if err := blob.Validate(); err != nil {
			return fmt.Errorf("blobs[%d]: %w", i, err)
		}
	}
	for i, deploy := range c.Deploys {
		if err := deploy.Validate(); err != nil {
			return fmt.Errorf("deploys[%d]: %w", i, err)
		}
	}
	for i, archive := range c.Archives {
		if err := archive.Validate(); err != nil {
			return fmt.Errorf("archives[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate checks BuildConfig for required fields.
func (b *BuildConfig) Validate() error {
	if b.Main == "" {
		return fmt.Errorf("main is required")
	}
	if len(b.Goos) == 0 {
		return fmt.Errorf("at least one goos value is required")
	}
	if len(b.Goarch) == 0 {
		return fmt.Errorf("at least one goarch value is required")
	}
	return nil
}

// Validate checks BlobConfig based on provider type.
func (b *BlobConfig) Validate() error {
	if b.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch b.Provider {
	case "s3":
		if b.Bucket == "" {
			return fmt.Errorf("bucket is required for s3 provider")
		}
		if b.Endpoint == "" {
			return fmt.Errorf("endpoint is required for s3 provider")
		}
		if b.Directory == "" {
			return fmt.Errorf("directory is required for s3 provider")
		}
	case "ssh":
		if b.Server == "" {
			return fmt.Errorf("server is required for ssh provider")
		}
		if b.User == "" {
			return fmt.Errorf("user is required for ssh provider")
		}
		if b.KeyPath == "" && b.KeyRaw == "" {
			return fmt.Errorf("either key_path or key_raw is required for ssh provider")
		}
		if b.KeyPath != "" && b.KeyRaw != "" {
			return fmt.Errorf("only one of key_path or key_raw should be provided")
		}
		if b.Directory == "" {
			return fmt.Errorf("directory is required for ssh provider")
		}
	default:
		return fmt.Errorf("unsupported provider: %s", b.Provider)
	}
	return nil
}

// Validate checks DeployConfig for required fields.
func (d *DeployConfig) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch d.Provider {
	case "ssh":
		if d.Server == "" {
			return fmt.Errorf("server is required for ssh provider")
		}
		if d.User == "" {
			return fmt.Errorf("user is required for ssh provider")
		}
		if d.KeyPath == "" && d.KeyRaw == "" {
			return fmt.Errorf("either key_path or key_raw is required for ssh provider")
		}
		if d.KeyPath != "" && d.KeyRaw != "" {
			return fmt.Errorf("only one of key_path or key_raw should be provided")
		}
		if len(d.Commands) == 0 {
			return fmt.Errorf("at least one command is required")
		}
	default:
		return fmt.Errorf("unsupported deploy provider: %s", d.Provider)
	}
	return nil
}

// Validate checks ArchiveConfig for supported formats.
func (a *ArchiveConfig) Validate() error {
	for _, f := range a.Formats {
		switch f {
		case "tar.gz", "zip":
			// ok
		default:
			return fmt.Errorf("unsupported archive format: %s", f)
		}
	}
	return nil
}
