---
name: gcx
description: >
  Go cross-compilation, build, publish, and deploy CLI tool (github.com/sxwebdev/gcx).
  Use this skill whenever working in the gcx codebase — editing build/publish/deploy logic,
  CLI commands, config structs, archive creation, SSH/S3 publishing, deployment alerts, or tests.
  Also triggers when code imports "sxwebdev/gcx", references gcx.yaml configuration,
  or when the user mentions: gcx CLI, Go cross-compilation tool, gcx build, gcx publish,
  gcx deploy, gcx release changelog, gcx config init, BlobConfig, BuildConfig, DeployConfig,
  ArchiveConfig, HooksConfig, Publisher, Deployer, Archiver, AlertData,
  shoutrrr alerts, or GoReleaser alternative.
user-invocable: false
---

# gcx — Go Cross-Compilation & Deploy Tool

## Overview

gcx is a lightweight CLI tool for cross-compiling Go binaries and publishing them to S3 or SSH servers. It reads YAML configuration (`gcx.yaml`), manages secrets via `.env` files, uses git tags for versioning, and supports deployment with notifications.

The codebase follows a clean package architecture with each concern separated into its own package under `internal/`.

## Architecture

See [references/architecture.md](references/architecture.md) for the full codebase map including file layout, key packages, and data flow.

**Key packages:**

- `cmd/gcx/main.go` — thin CLI layer (~200 lines): command definitions, flag wiring, package orchestration
- `internal/config/` — all config structs, YAML loading, comprehensive validation
- `internal/build/` — build orchestration, BuildArtifact struct, archive creation
- `internal/archive/` — Archiver interface with tar.gz and zip implementations
- `internal/publish/` — Publisher interface with S3 and SSH implementations
- `internal/deploy/` — Deployer interface with SSH implementation
- `internal/notify/` — notification sending via shoutrrr
- `internal/git/` — git operations (tag, changelog, commit hash)
- `internal/sshutil/` — shared SSH client factory, known hosts management
- `internal/tmpl/` — shared template processing utility
- `internal/hook/` — hook execution via `sh -c`
- `internal/shellutil/` — shell escaping utilities
- `internal/helpers/` — path expansion utility

## Configuration Schema

See [references/config-schema.md](references/config-schema.md) for the complete config schema with all fields, types, defaults, and template variables.

## Instructions

### Adding a new CLI command

1. Define the command in `cmd/gcx/main.go` inside the `cli.Command` tree
2. Use `urfave/cli/v3` patterns — `cli.Command` with `Name`, `Usage`, `Flags`, and `Action`
3. The action handler receives `context.Context` — pass it to all package functions
4. Access config via `config.Load(cmd.String("config"))` when the command needs configuration

### Modifying config structs

1. Add the new field to the appropriate struct in `internal/config/config.go`
2. Include the `yaml:"field_name,omitempty"` tag
3. Add validation in the struct's `Validate()` method
4. Update `examples/gcx.yaml` to document the new field

### Adding a new publish provider

1. Create a new file `internal/publish/{provider}.go`
2. Implement the `Publisher` interface: `Name() string` and `Publish(ctx, artifactsDir, version) error`
3. Add provider-specific fields to `config.BlobConfig` with `yaml:"...,omitempty"` tags
4. Add validation in `BlobConfig.Validate()` for the new provider
5. Register the provider in `publish.NewPublisher()` factory
6. Use `tmpl.Process()` for directory template processing
7. Use `sshutil.NewClient()` if the provider needs SSH (eliminates code duplication)

### Adding a new deploy provider

1. Create a new file `internal/deploy/{provider}.go`
2. Implement the `Deployer` interface: `Name() string` and `Deploy(ctx) error`
3. Add provider-specific fields to `config.DeployConfig`
4. Add validation in `DeployConfig.Validate()`
5. Register in `deploy.NewDeployer()` factory

### Working with templates

gcx uses `internal/tmpl.Process()` — a wrapper around Go's `text/template`:

```go
result, err := tmpl.Process("name", templateStr, data)
```

Template data for builds:

- `{{.Version}}` — git tag
- `{{.Commit}}` — short commit hash
- `{{.Date}}` — RFC3339 build timestamp
- `{{.Env.VAR_NAME}}` — environment variable (security-filtered)

For archive naming (`ArchiveTemplateData`): `{{.Binary}}`, `{{.Version}}`, `{{.Os}}`, `{{.Arch}}`

### Adding deployment alerts

Alerts use the [shoutrrr](https://containrrr.dev/shoutrrr/) library via `internal/notify/`.

```go
notify.Send(urls, notify.AlertData{
    AppName: "myapp",
    Version: "v1.0.0",
    Status:  "Success",
})
```

### Error handling patterns

- Wrap errors with context: `fmt.Errorf("doing X: %w", err)`
- All config structs have `Validate()` methods
- Alert failures are logged but don't block deployment
- Hook failures stop execution immediately
- Git command failures return sensible defaults ("0.0.0" for tag, "none" for commit)

### Building and testing

```bash
make build          # builds to ./bin/gcx
go test ./...       # run all tests
make docker-build   # build Docker image
make install        # go install
```

## Key principles

- **Clean package architecture**: Each concern is in its own package. `cmd/gcx/main.go` is a thin CLI layer that wires packages together.

- **Interfaces for providers**: `Publisher`, `Deployer`, and `Archiver` interfaces enable testability and easy extension via factory functions.

- **Shared SSH client factory**: `sshutil.NewClient()` eliminates SSH client code duplication between publish and deploy.

- **Shell safety**: Hooks run via `sh -c` (not naive `strings.Fields` parsing). SSH remote commands use `shellutil.Quote()` for shell escaping.

- **Context propagation**: All operations accept `context.Context` for cancellation support.

- **Template safety**: Environment variables are selectively exposed — only vars explicitly referenced in `{{.Env.X}}` patterns are loaded.

- **Structured artifact metadata**: `build.Artifact` carries binary name, version, OS, arch as structured data — no fragile filename parsing.

- **Parallel builds**: `errgroup` with configurable concurrency limit handles concurrent compilation and archiving.
