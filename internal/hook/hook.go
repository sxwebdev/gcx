package hook

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
)

// Run executes shell hooks sequentially using "sh -c" for proper shell semantics.
// It supports quoted arguments, pipes, redirections, and other shell features.
func Run(ctx context.Context, hooks []string) error {
	for _, h := range hooks {
		if h == "" {
			continue
		}
		log.Printf("Executing hook: %s", h)
		cmd := exec.CommandContext(ctx, "sh", "-c", h)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook %q failed: %w", h, err)
		}
	}
	return nil
}
