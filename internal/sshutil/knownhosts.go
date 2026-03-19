package sshutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sxwebdev/gcx/internal/helpers"
)

// EnsureKnownHost checks if the server is in known_hosts.
// If the known_hosts file doesn't exist, it creates it and runs ssh-keyscan.
func EnsureKnownHost(server string) error {
	knownHostsPath, err := helpers.ExpandPath("~/.ssh/known_hosts")
	if err != nil {
		return fmt.Errorf("failed to expand known hosts path: %w", err)
	}

	if _, err := os.Stat(knownHostsPath); !os.IsNotExist(err) {
		return nil
	}

	sshDir := filepath.Dir(knownHostsPath)
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	if err := os.WriteFile(knownHostsPath, []byte{}, 0o600); err != nil {
		return fmt.Errorf("failed to create known_hosts file: %w", err)
	}

	cmd := exec.Command("ssh-keyscan", "-H", server)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ssh-keyscan failed for %s: %w", server, err)
	}

	f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open known_hosts file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(output); err != nil {
		return fmt.Errorf("failed to write to known_hosts file: %w", err)
	}

	return nil
}
