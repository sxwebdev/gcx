package hook

import (
	"context"
	"testing"
)

func TestRun(t *testing.T) {
	ctx := context.Background()

	t.Run("empty hooks", func(t *testing.T) {
		if err := Run(ctx, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty string hook", func(t *testing.T) {
		if err := Run(ctx, []string{""}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("successful command", func(t *testing.T) {
		if err := Run(ctx, []string{"echo hello"}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("quoted arguments", func(t *testing.T) {
		if err := Run(ctx, []string{`echo "hello world"`}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("pipe support", func(t *testing.T) {
		if err := Run(ctx, []string{"echo hello | cat"}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("failing command", func(t *testing.T) {
		err := Run(ctx, []string{"false"})
		if err == nil {
			t.Error("expected error for failing command")
		}
	})

	t.Run("stops on first failure", func(t *testing.T) {
		err := Run(ctx, []string{"true", "false", "true"})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		err := Run(ctx, []string{"sleep 10"})
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})
}
