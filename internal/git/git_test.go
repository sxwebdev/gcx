package git

import (
	"context"
	"testing"
)

func TestGetTag(t *testing.T) {
	ctx := context.Background()
	tag := GetTag(ctx)
	// In a git repo with tags, this should return a tag; otherwise "0.0.0"
	if tag == "" {
		t.Error("GetTag returned empty string")
	}
}

func TestGetCommitHash(t *testing.T) {
	ctx := context.Background()
	hash := GetCommitHash(ctx)
	if hash == "" {
		t.Error("GetCommitHash returned empty string")
	}
	// Should be a short hash or "none"
	if len(hash) > 12 && hash != "none" {
		t.Errorf("GetCommitHash returned unexpectedly long hash: %s", hash)
	}
}

func TestGetPreviousTag(t *testing.T) {
	ctx := context.Background()
	tag := GetPreviousTag(ctx)
	if tag == "" {
		t.Error("GetPreviousTag returned empty string")
	}
}

func TestGetPreviousStableTag(t *testing.T) {
	ctx := context.Background()
	tag := GetPreviousStableTag(ctx)
	if tag == "" {
		t.Error("GetPreviousStableTag returned empty string")
	}
}
