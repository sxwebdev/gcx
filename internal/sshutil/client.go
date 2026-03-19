package sshutil

import (
	"fmt"

	"github.com/melbahja/goph"
	"github.com/sxwebdev/gcx/internal/helpers"
)

// ClientConfig holds SSH connection parameters.
type ClientConfig struct {
	Server                string
	User                  string
	KeyPath               string
	KeyRaw                string
	InsecureIgnoreHostKey bool
}

// Validate checks that the SSH client configuration is valid.
func (c *ClientConfig) Validate() error {
	if c.Server == "" {
		return fmt.Errorf("server is required")
	}
	if c.User == "" {
		return fmt.Errorf("user is required")
	}
	if c.KeyPath == "" && c.KeyRaw == "" {
		return fmt.Errorf("either key_path or key_raw is required")
	}
	if c.KeyPath != "" && c.KeyRaw != "" {
		return fmt.Errorf("only one of key_path or key_raw should be provided")
	}
	return nil
}

// NewClient creates a new SSH client from the given configuration.
// It handles key loading, known hosts verification, and client creation.
func NewClient(cfg ClientConfig) (*goph.Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid SSH configuration: %w", err)
	}

	if !cfg.InsecureIgnoreHostKey {
		if err := EnsureKnownHost(cfg.Server); err != nil {
			return nil, fmt.Errorf("known hosts check failed: %w", err)
		}
	}

	var (
		auth goph.Auth
		err  error
	)
	if cfg.KeyRaw != "" {
		auth, err = goph.RawKey(cfg.KeyRaw, "")
		if err != nil {
			return nil, fmt.Errorf("failed to load SSH key from raw data: %w", err)
		}
	} else {
		path, err := helpers.ExpandPath(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to expand key path: %w", err)
		}
		auth, err = goph.Key(path, "")
		if err != nil {
			return nil, fmt.Errorf("failed to load SSH key from %s: %w", path, err)
		}
	}

	if cfg.InsecureIgnoreHostKey {
		client, err := goph.NewUnknown(cfg.User, cfg.Server, auth)
		if err != nil {
			return nil, fmt.Errorf("failed to create insecure SSH client: %w", err)
		}
		return client, nil
	}

	client, err := goph.New(cfg.User, cfg.Server, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client: %w", err)
	}
	return client, nil
}
