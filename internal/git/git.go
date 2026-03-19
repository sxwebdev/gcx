package git

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
)

var stableTagRegex = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

const defaultVersion = "0.0.0"

// GetTag returns the current git tag. Returns "0.0.0" if not found.
func GetTag(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "git", "describe", "--tags", "--abbrev=0")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to get git tag: %v. Using default value %s", err, defaultVersion)
		return defaultVersion
	}
	tag := strings.TrimSpace(string(out))
	if tag == "" {
		log.Printf("Git tag is empty, using default value %s", defaultVersion)
		return defaultVersion
	}
	return tag
}

// GetPreviousTag returns the previous git tag before the current one.
func GetPreviousTag(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "git", "tag", "-l", "--sort=-v:refname")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to get git tags: %v. Using default value %s", err, defaultVersion)
		return defaultVersion
	}

	tags := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(tags) < 2 {
		log.Printf("No previous tag found, using default value %s", defaultVersion)
		return defaultVersion
	}

	currentTag := GetTag(ctx)
	for i, tag := range tags {
		if tag == currentTag && i+1 < len(tags) {
			return tags[i+1]
		}
	}

	return defaultVersion
}

// GetPreviousStableTag returns the previous stable git tag (vX.Y.Z without pre-release suffix).
func GetPreviousStableTag(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "git", "tag", "-l", "--sort=-v:refname")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to get git tags: %v. Using default value %s", err, defaultVersion)
		return defaultVersion
	}

	tags := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(tags) == 0 {
		log.Printf("No tags found, using default value %s", defaultVersion)
		return defaultVersion
	}

	currentTag := GetTag(ctx)
	foundCurrent := false

	for _, tag := range tags {
		if !foundCurrent && tag == currentTag {
			foundCurrent = true
			continue
		}
		if stableTagRegex.MatchString(tag) {
			return tag
		}
	}

	return defaultVersion
}

// GetCommitHash returns the short git commit hash.
func GetCommitHash(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to get git commit hash: %v. Using default value 'none'", err)
		return "none"
	}
	return strings.TrimSpace(string(out))
}

// GetChangelog returns a markdown formatted changelog between two tags.
func GetChangelog(ctx context.Context, from, to string) (string, error) {
	remoteCmd := exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url")
	remoteOut, err := remoteCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}

	repoURL := strings.TrimSpace(string(remoteOut))
	repoURL = strings.TrimSuffix(repoURL, ".git")
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = strings.Replace(repoURL, ":", "/", 1)
		repoURL = strings.Replace(repoURL, "git@", "https://", 1)
	}

	if from == defaultVersion || from == "" {
		return "", nil
	}

	cmd := exec.CommandContext(ctx, "git", "log",
		"--pretty=format:* %s by @%an in %h",
		fmt.Sprintf("%s..%s", from, to))
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git log: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("## What's Changed\n\n")
	sb.WriteString(string(out) + "\n")
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "**Full Changelog**: %s/compare/%s...%s\n", repoURL, from, to)

	return sb.String(), nil
}
