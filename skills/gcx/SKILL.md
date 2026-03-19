---
name: gcx
description: >
  Go cross-compilation, build, publish, and deploy CLI tool (github.com/sxwebdev/gcx).
  Use this skill whenever working in the gcx codebase — editing build/publish/deploy logic,
  CLI commands, config structs, archive creation, SSH/S3 publishing, deployment alerts, or tests.
  Also triggers when code imports "sxwebdev/gcx", references gcx.yaml configuration,
  or when the user mentions: gcx CLI, Go cross-compilation tool, gcx build, gcx publish,
  gcx deploy, gcx release changelog, gcx config init, BlobConfig, BuildConfig, DeployConfig,
  ArchiveConfig, HooksConfig, S3Config, SSHPublishConfig, SSHDeployConfig, AlertConfig,
  shoutrrr alerts, or GoReleaser alternative.
user-invocable: false
---

# gcx — Go Cross-Compilation & Deploy Tool

## Overview

gcx is a lightweight CLI tool for cross-compiling Go binaries and publishing them to S3 or SSH servers. It reads YAML configuration (`gcx.yaml`), manages secrets via `.env` files, uses git tags for versioning, and supports deployment with notifications.

The codebase is compact: nearly all logic lives in a single `cmd/gcx/main.go` (~1500 lines) with a small helper package in `internal/helpers/`. It uses `urfave/cli/v3` for CLI commands.

## Architecture

See [references/architecture.md](references/architecture.md) for the full codebase map including file layout, key functions, and data flow.

**Key files:**

- `cmd/gcx/main.go` — all core logic: config parsing, build, publish, deploy, archive, alerts, CLI commands
- `internal/helpers/path.go` — `ExpandPath()` for tilde expansion in SSH key paths
- `gcx.release.yaml` — gcx's own release configuration
- `examples/gcx.yaml` — example configuration with all features

## Configuration Schema

See [references/config-schema.md](references/config-schema.md) for the complete config schema with all fields, types, defaults, and template variables.

## Instructions

### Adding a new CLI command

1. Define the command in the `main()` function inside the `cli.Command` tree
2. Use `urfave/cli/v3` patterns — `cli.Command` with `Name`, `Usage`, `Flags`, and `Action`
3. Add flags using `cli.StringFlag`, `cli.BoolFlag`, etc.
4. Access config via `loadConfig(cmd.String("config"))` when the command needs configuration
5. Follow the existing pattern of returning `error` from action functions

### Modifying config structs

1. Add the new field to the appropriate struct in `cmd/gcx/main.go` (e.g., `BuildConfig`, `BlobConfig`, `DeployConfig`)
2. Include the `yaml:"field_name,omitempty"` tag so it maps correctly from YAML
3. If the field needs validation, add checks in the relevant `Validate()` method
4. If it's a provider-specific field (S3 vs SSH), add it to both the `BlobConfig`/`DeployConfig` union struct AND the internal typed struct (`S3Config`, `SSHPublishConfig`, etc.)
5. Update the `To*Config()` conversion methods to map the new field
6. Update `examples/gcx.yaml` to document the new field

### Adding a new publish provider

1. Add a new case in `publishArtifacts()` for the provider name
2. Create a new `publishTo{Provider}()` function following the pattern of `publishToS3()` or `publishToSSH()`
3. Add provider-specific fields to `BlobConfig` with `yaml:"...,omitempty"` tags
4. Create an internal config struct (like `S3Config`) for type safety
5. Add a `To{Provider}Config()` conversion method on `BlobConfig`
6. Add validation if needed
7. Template variables (`{{.Version}}`, `{{.Commit}}`, etc.) are passed via `tmplData map[string]string` — use `processTemplate()` pattern for directory paths

### Working with templates

gcx uses Go's `text/template` for dynamic values. Template data is built from git info:

```go
tmplData := map[string]string{
    "Version": gitTag,
    "Commit":  commitHash,
    "Date":    time.Now().Format(time.RFC3339),
}
```

For archive naming, `ArchiveTemplateData` struct adds `Binary`, `Os`, `Arch` fields.

Environment variables in ldflags use `{{.Env.VAR_NAME}}` — the code extracts referenced var names via regex and builds a selective env map (security measure).

### Adding deployment alerts

Alerts use the [shoutrrr](https://containrrr.dev/shoutrrr/) library. URL format examples:

- Telegram: `telegram://token@telegram?channels=channel-1`
- Slack: `slack://token-a/token-b/token-c`
- Discord: `discord://token@channel`
- Teams: `teams://token-a/token-b/token-c`
- Generic webhook: `generic://example.com/webhook?token=token`

The `sendAlert()` function formats a message from `AlertTemplateData` and sends via shoutrrr.

### Error handling patterns

- Wrap errors with context: `fmt.Errorf("error doing X: %w", err)`
- Validation methods return descriptive errors for missing required fields
- Alert failures are logged but don't block deployment
- Hook failures stop execution immediately
- Git command failures return sensible defaults ("0.0.0" for tag, "none" for commit)

### Building and testing

```bash
# Build locally
make build          # builds to ./bin/gcx

# Run tests
go test ./...

# Build Docker image
make docker-build

# Install from source
make install        # go install
```

## Examples

**Example 1: User asks to add a new `provider: gcs` for Google Cloud Storage publishing**

Input: "Add Google Cloud Storage support as a new publish provider"

Steps:

1. Read `cmd/gcx/main.go` — find `publishArtifacts()` switch statement and existing provider implementations
2. Add GCS-specific fields to `BlobConfig` (`project_id`, etc.)
3. Create `GCSConfig` internal struct and `ToGCSConfig()` method
4. Create `publishToGCS()` function following the `publishToS3()` pattern
5. Add the `"gcs"` case in `publishArtifacts()`
6. Update `examples/gcx.yaml` with GCS example

**Example 2: User asks to add a new template variable**

Input: "Add {{.Branch}} template variable to ldflags"

Steps:

1. In `buildBinaries()`, add code to get current git branch: `git rev-parse --abbrev-ref HEAD`
2. Add `"Branch": branchName` to the template data map
3. The variable is now available in ldflags, directory templates, and archive name templates

## Key principles

- **Single-file architecture**: Nearly all logic is in `cmd/gcx/main.go`. Keep it that way unless a change clearly warrants a new package. Extract to `internal/` only when code is reusable across multiple functions.

- **Config struct = YAML contract**: Every config field must have a `yaml:"..."` tag. Use `omitempty` for optional fields. The union pattern (e.g., `BlobConfig` holds both S3 and SSH fields) is intentional — it keeps the YAML flat and user-friendly.

- **Template safety**: Environment variables are selectively exposed — only vars explicitly referenced in `{{.Env.X}}` patterns are loaded. Never bypass this by exposing all env vars, as it prevents accidental secret leakage into binaries.

- **Parallel builds**: `errgroup` with `runtime.NumCPU()` limit handles concurrent compilation. Maintain this pattern for any new parallel work.

- **Provider pattern**: Publishing and deployment use a provider string (`"s3"`, `"ssh"`) dispatched in a switch statement. Internal typed structs (`S3Config`, `SSHPublishConfig`) provide type safety after the switch. Follow this pattern for new providers.
