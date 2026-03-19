# gcx Architecture Reference

## Table of Contents

- [File Layout](#file-layout)
- [Dependencies](#dependencies)
- [CLI Command Tree](#cli-command-tree)
- [Key Functions](#key-functions)
- [Data Flow](#data-flow)

---

## File Layout

```
gcx/
├── cmd/gcx/
│   └── main.go              # All core logic (~1500 lines): config, CLI, build, publish, deploy, archive, alerts
├── internal/helpers/
│   ├── path.go              # ExpandPath() — tilde expansion for SSH key paths
│   └── path_test.go         # Tests for path expansion
├── examples/
│   └── gcx.yaml             # Full example configuration
├── scripts/
│   └── install.sh           # Binary installation script (curl from GitHub releases)
├── .github/workflows/
│   ├── go.yml               # CI: build + test on push/PR
│   └── release.yml          # Release: build binaries, Docker image, GitHub release on tag
├── Dockerfile               # Multi-stage: Go 1.26 builder → minimal runtime
├── Makefile                  # build, install, docker-build, docker-push targets
├── gcx.release.yaml         # gcx's own release config (linux/darwin × amd64/arm64)
├── go.mod / go.sum
└── README.md
```

## Dependencies

| Package                          | Purpose                                                |
| -------------------------------- | ------------------------------------------------------ |
| `github.com/urfave/cli/v3`       | CLI framework — commands, flags, help text             |
| `gopkg.in/yaml.v3`               | YAML config parsing                                    |
| `github.com/minio/minio-go/v7`   | S3 client for artifact publishing                      |
| `github.com/melbahja/goph`       | SSH client for publishing and deployment               |
| `github.com/containrrr/shoutrrr` | Notification sending (Telegram, Slack, Discord, Teams) |
| `github.com/joho/godotenv`       | Load `.env` files                                      |
| `golang.org/x/sync/errgroup`     | Parallel build execution with CPU limit                |

**Standard library highlights:** `archive/tar`, `compress/gzip` (archiving), `text/template` (variable substitution), `os/exec` (running go build and hooks), `regexp` (extracting env var references from templates).

## CLI Command Tree

```
gcx
├── build                    # Cross-compile binaries (buildBinaries)
├── publish                  # Upload artifacts to S3/SSH (publishArtifacts)
│   └── --name, -n           # Run specific publish config by name
├── deploy                   # Execute remote commands via SSH (deployArtifacts)
│   └── --name, -n           # Run specific deploy config by name
├── release
│   └── changelog            # Generate markdown changelog between git tags
│       └── --stable, -s     # Compare with previous stable tag (vX.Y.Z)
├── git
│   └── version              # Print current git tag
├── config
│   └── init                 # Generate new gcx.yaml
│       ├── --os, -o         # Target OS (default: runtime.GOOS)
│       ├── --arch, -a       # Target arch (default: runtime.GOARCH)
│       ├── --main, -m       # Main package path (default: ./cmd/app)
│       └── --force, -f      # Overwrite existing file
└── version                  # Print gcx version, commit, build date
```

All commands share `--config, -c` flag (default: `gcx.yaml`).

## Key Functions

### Configuration

| Function                 | Line | Purpose                         |
| ------------------------ | ---- | ------------------------------- |
| `loadConfig(configPath)` | ~245 | Read and parse YAML config file |

### Build Pipeline

| Function                 | Line | Purpose                                                             |
| ------------------------ | ---- | ------------------------------------------------------------------- |
| `buildBinaries(cfg)`     | ~430 | Main build orchestrator: hooks → clean → parallel compile → archive |
| `runHooks(hooks)`        | ~258 | Execute shell commands sequentially                                 |
| `getOutputFilename(...)` | ~412 | Compute binary output path with OS/arch suffix                      |

### Git Operations

| Function                    | Line | Purpose                                  |
| --------------------------- | ---- | ---------------------------------------- |
| `getGitTag()`               | ~277 | Get current tag via `git describe`       |
| `getPreviousGitTag()`       | ~293 | Get previous tag for changelog           |
| `getPreviousStableGitTag()` | ~320 | Get previous stable tag (vX.Y.Z pattern) |
| `getGitChangelog(from, to)` | ~359 | Generate markdown changelog between tags |
| `getGitCommitHash()`        | ~401 | Get short commit hash                    |

### Publishing

| Function                       | Line | Purpose                                         |
| ------------------------------ | ---- | ----------------------------------------------- |
| `publishArtifacts(cfg, name)`  | ~611 | Dispatch to provider-specific publish functions |
| `publishToS3(cfg, dir, tmpl)`  | ~665 | Upload artifacts to S3/S3-compatible storage    |
| `publishToSSH(cfg, dir, tmpl)` | ~748 | Upload artifacts via SSH/SFTP                   |

### Archiving

| Function                       | Line  | Purpose                                     |
| ------------------------------ | ----- | ------------------------------------------- |
| `createArchives(cfg, dir)`     | ~837  | Create tar.gz archives with template naming |
| `createTarGz(src, dest)`       | ~948  | Low-level tar.gz creation                   |
| `addFileToTar(tw, path, name)` | ~981  | Add single file to tar archive              |
| `addDirToTar(tw, dir, base)`   | ~1017 | Recursively add directory to tar archive    |

### Deployment

| Function                     | Line  | Purpose                                 |
| ---------------------------- | ----- | --------------------------------------- |
| `deployArtifacts(cfg, name)` | ~1055 | Iterate deploy configs and execute      |
| `executeDeploy(deploy)`      | ~1124 | Run deployment with alert notifications |
| `executeSSHDeploy(cfg)`      | ~1165 | SSH connect and execute remote commands |
| `sendAlert(urls, data)`      | ~1081 | Send notification via shoutrrr          |
| `checkKnonwnHost(server)`    | ~1229 | Verify SSH host key in known_hosts      |

### Validation

| Method                        | Purpose                                                        |
| ----------------------------- | -------------------------------------------------------------- |
| `SSHPublishConfig.Validate()` | Validate SSH publish config (name, server, user, key)          |
| `SSHDeployConfig.Validate()`  | Validate SSH deploy config (name, server, user, commands, key) |

### Config Conversion

| Method                             | Purpose                                            |
| ---------------------------------- | -------------------------------------------------- |
| `BlobConfig.ToS3Config()`          | Convert union BlobConfig → typed S3Config          |
| `BlobConfig.ToSSHConfig()`         | Convert union BlobConfig → typed SSHPublishConfig  |
| `DeployConfig.ToSSHDeployConfig()` | Convert union DeployConfig → typed SSHDeployConfig |

## Data Flow

### Build flow

```
main() → build command
  → loadConfig()
  → runHooks(before)
  → clean/create out_dir
  → getGitTag(), getGitCommitHash()
  → extract env var names from ldflags via regex
  → for each build config:
      for each goos × goarch (× goarm):
        → process ldflags templates (text/template)
        → exec "go build" with env vars (parallel via errgroup)
  → createArchives()
  → runHooks(after)
```

### Publish flow

```
main() → publish command
  → loadConfig()
  → getGitTag(), getGitCommitHash()
  → for each blob config (filtered by --name):
      switch provider:
        "s3"  → publishToS3()  — minio client → upload files
        "ssh" → publishToSSH() — goph client → SFTP upload
```

### Deploy flow

```
main() → deploy command
  → loadConfig()
  → for each deploy config (filtered by --name):
      → executeDeploy()
        → executeSSHDeploy()
          → checkKnonwnHost()
          → goph.New() SSH connection
          → execute commands sequentially
        → sendAlert() with success/failure status
```

### Archive flow

```
createArchives()
  → for each archive config:
      for each built binary directory:
        → process name template
        → createTarGz(srcPath, destFile)
          → addFileToTar() or addDirToTar() (recursive)
        → remove source directory
```
