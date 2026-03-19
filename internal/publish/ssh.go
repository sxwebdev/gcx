package publish

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/sxwebdev/gcx/internal/config"
	"github.com/sxwebdev/gcx/internal/shellutil"
	"github.com/sxwebdev/gcx/internal/sshutil"
	"github.com/sxwebdev/gcx/internal/tmpl"
)

// SSHPublisher uploads artifacts to a remote server via SSH/SFTP.
type SSHPublisher struct {
	name      string
	sshCfg    sshutil.ClientConfig
	directory string
}

// NewSSHPublisher creates an SSHPublisher from config.
func NewSSHPublisher(cfg config.BlobConfig) (*SSHPublisher, error) {
	return &SSHPublisher{
		name: cfg.Name,
		sshCfg: sshutil.ClientConfig{
			Server:                cfg.Server,
			User:                  cfg.User,
			KeyPath:               cfg.KeyPath,
			KeyRaw:                cfg.KeyRaw,
			InsecureIgnoreHostKey: cfg.InsecureIgnoreHostKey,
		},
		directory: cfg.Directory,
	}, nil
}

func (p *SSHPublisher) Name() string { return p.name }

func (p *SSHPublisher) Publish(_ context.Context, artifactsDir string, version string) error {
	remoteDir, err := tmpl.Process("directory", p.directory, map[string]string{"Version": version})
	if err != nil {
		return fmt.Errorf("process directory template: %w", err)
	}

	client, err := sshutil.NewClient(p.sshCfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	// Shell-safe mkdir (fixes command injection vulnerability)
	if _, err := client.Run("mkdir -p " + shellutil.Quote(remoteDir)); err != nil {
		return fmt.Errorf("create remote directory: %w", err)
	}

	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		return fmt.Errorf("read directory %s: %w", artifactsDir, err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		localFilePath := filepath.Join(artifactsDir, file.Name())
		remotePath := filepath.Join(remoteDir, file.Name())
		log.Printf("Uploading %s to %s:%s", localFilePath, p.sshCfg.Server, remotePath)

		if err := client.Upload(localFilePath, remotePath); err != nil {
			return fmt.Errorf("upload file %s: %w", localFilePath, err)
		}
	}

	return nil
}
