package publish

import (
	"context"
	"fmt"
	"log"

	"github.com/sxwebdev/gcx/internal/config"
	"github.com/sxwebdev/gcx/internal/git"
)

// Publisher uploads artifacts to a remote destination.
type Publisher interface {
	Name() string
	Publish(ctx context.Context, artifactsDir string, version string) error
}

// NewPublisher creates a Publisher from a BlobConfig.
func NewPublisher(cfg config.BlobConfig) (Publisher, error) {
	switch cfg.Provider {
	case "s3":
		return NewS3Publisher(cfg)
	case "ssh":
		return NewSSHPublisher(cfg)
	default:
		return nil, fmt.Errorf("unsupported publish provider: %s", cfg.Provider)
	}
}

// Run publishes artifacts to configured destinations.
func Run(ctx context.Context, cfg *config.Config, publishName string) error {
	artifactsDir := cfg.OutDir
	tag := git.GetTag(ctx)

	var blobs []config.BlobConfig
	if publishName != "" {
		var found bool
		for _, blob := range cfg.Blobs {
			if blob.Name == publishName {
				blobs = append(blobs, blob)
				found = true
			}
		}
		if !found {
			return fmt.Errorf("publish configuration %q not found", publishName)
		}
	} else {
		blobs = cfg.Blobs
	}

	for _, blob := range blobs {
		publisher, err := NewPublisher(blob)
		if err != nil {
			return fmt.Errorf("create publisher %q: %w", blob.Name, err)
		}
		log.Printf("Publishing to: %s", publisher.Name())
		if err := publisher.Publish(ctx, artifactsDir, tag); err != nil {
			return fmt.Errorf("publish %q: %w", blob.Name, err)
		}
	}
	return nil
}
