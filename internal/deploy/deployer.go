package deploy

import (
	"context"
	"fmt"
	"log"

	"github.com/sxwebdev/gcx/internal/config"
	"github.com/sxwebdev/gcx/internal/git"
	"github.com/sxwebdev/gcx/internal/notify"
)

// Deployer executes deployment commands.
type Deployer interface {
	Name() string
	Deploy(ctx context.Context) error
}

// NewDeployer creates a Deployer from a DeployConfig.
func NewDeployer(cfg config.DeployConfig) (Deployer, error) {
	switch cfg.Provider {
	case "ssh":
		return NewSSHDeployer(cfg)
	default:
		return nil, fmt.Errorf("unsupported deploy provider: %s", cfg.Provider)
	}
}

// Run executes deployments according to the configuration.
func Run(ctx context.Context, cfg *config.Config, deployName string) error {
	if len(cfg.Deploys) == 0 {
		return fmt.Errorf("no deploy configurations found")
	}

	if deployName != "" {
		for _, deploy := range cfg.Deploys {
			if deploy.Name == deployName {
				return executeDeploy(ctx, deploy)
			}
		}
		return fmt.Errorf("deploy configuration %q not found", deployName)
	}

	for _, deploy := range cfg.Deploys {
		if err := executeDeploy(ctx, deploy); err != nil {
			return fmt.Errorf("deploy %q failed: %w", deploy.Name, err)
		}
	}
	return nil
}

func executeDeploy(ctx context.Context, deployCfg config.DeployConfig) error {
	log.Printf("Executing deploy: %s", deployCfg.Name)

	version := git.GetTag(ctx)

	deployer, err := NewDeployer(deployCfg)
	if err != nil {
		return err
	}

	alertData := notify.AlertData{
		AppName: deployCfg.Name,
		Version: version,
	}

	if deployErr := deployer.Deploy(ctx); deployErr != nil {
		alertData.Status = "Failed"
		alertData.Error = deployErr.Error()
		if err := notify.Send(deployCfg.Alerts.URLs, alertData); err != nil {
			log.Printf("Failed to send failure alert: %v", err)
		}
		return deployErr
	}

	alertData.Status = "Success"
	if err := notify.Send(deployCfg.Alerts.URLs, alertData); err != nil {
		log.Printf("Failed to send success alert: %v", err)
	}

	return nil
}
