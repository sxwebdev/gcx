package deploy

import (
	"context"
	"fmt"
	"log"

	"github.com/sxwebdev/gcx/internal/config"
	"github.com/sxwebdev/gcx/internal/sshutil"
)

// SSHDeployer executes commands on a remote server via SSH.
type SSHDeployer struct {
	name     string
	sshCfg   sshutil.ClientConfig
	commands []string
}

// NewSSHDeployer creates an SSHDeployer from config.
func NewSSHDeployer(cfg config.DeployConfig) (*SSHDeployer, error) {
	return &SSHDeployer{
		name: cfg.Name,
		sshCfg: sshutil.ClientConfig{
			Server:                cfg.Server,
			User:                  cfg.User,
			KeyPath:               cfg.KeyPath,
			KeyRaw:                cfg.KeyRaw,
			InsecureIgnoreHostKey: cfg.InsecureIgnoreHostKey,
		},
		commands: cfg.Commands,
	}, nil
}

func (d *SSHDeployer) Name() string { return d.name }

func (d *SSHDeployer) Deploy(_ context.Context) error {
	client, err := sshutil.NewClient(d.sshCfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	for _, cmd := range d.commands {
		log.Printf("Executing command: %s", cmd)
		out, err := client.Run(cmd)
		if err != nil {
			return fmt.Errorf("command %q failed: %w", cmd, err)
		}
		log.Printf("Command output:\n%s", string(out))
	}

	return nil
}
