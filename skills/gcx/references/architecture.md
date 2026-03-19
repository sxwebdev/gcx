# gcx Architecture Reference

## Table of Contents

- [File Layout](#file-layout)
- [Dependencies](#dependencies)
- [CLI Command Tree](#cli-command-tree)
- [Package Reference](#package-reference)
- [Data Flow](#data-flow)

---

## File Layout

```
gcx/
├── cmd/gcx/
│   └── main.go                    # Thin CLI layer (~200 lines): commands, flags, wiring
├── internal/
│   ├── config/
│   │   ├── config.go              # All config structs, Load(), Validate()
│   │   └── config_test.go
│   ├── build/
│   │   ├── artifact.go            # BuildArtifact struct
│   │   ├── build.go               # Run(): hooks → compile → archive
│   │   └── build_test.go
│   ├── archive/
│   │   ├── archive.go             # Archiver interface + New() factory
│   │   ├── targz.go               # tar.gz implementation
│   │   ├── zip.go                 # zip implementation
│   │   └── archive_test.go
│   ├── publish/
│   │   ├── publisher.go           # Publisher interface + Run()
│   │   ├── s3.go                  # S3Publisher
│   │   └── ssh.go                 # SSHPublisher
│   ├── deploy/
│   │   ├── deployer.go            # Deployer interface + Run()
│   │   └── ssh.go                 # SSHDeployer
│   ├── notify/
│   │   └── notify.go              # Send() via shoutrrr
│   ├── git/
│   │   ├── git.go                 # GetTag, GetChangelog, GetCommitHash
│   │   └── git_test.go
│   ├── sshutil/
│   │   ├── client.go              # NewClient() SSH factory + ClientConfig
│   │   ├── knownhosts.go          # EnsureKnownHost()
│   │   └── client_test.go
│   ├── tmpl/
│   │   ├── template.go            # Process() template helper
│   │   └── template_test.go
│   ├── hook/
│   │   ├── hook.go                # Run() hooks via sh -c
│   │   └── hook_test.go
│   ├── shellutil/
│   │   ├── escape.go              # Quote() shell escaping
│   │   └── escape_test.go
│   └── helpers/
│       ├── path.go                # ExpandPath() tilde expansion
│       └── path_test.go
├── examples/
│   └── gcx.yaml                   # Full example configuration
├── scripts/
│   └── install.sh                 # Binary installation script
├── .github/workflows/
│   ├── go.yml                     # CI: build + test
│   └── release.yml                # Release: binaries, Docker, GitHub release
├── Dockerfile                     # Multi-stage: Go builder → minimal runtime
├── Makefile                       # build, install, docker-build, docker-push
├── gcx.release.yaml               # gcx's own release config
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
| `golang.org/x/sync/errgroup`     | Parallel build execution with concurrency limit        |

**Standard library highlights:** `archive/tar`, `archive/zip`, `compress/gzip` (archiving), `text/template` (variable substitution), `os/exec` (running go build and hooks), `regexp` (extracting env var references), `path` (URL-style S3 paths).

## CLI Command Tree

```
gcx
├── build                    # Cross-compile binaries (build.Run)
├── publish                  # Upload artifacts to S3/SSH (publish.Run)
│   └── --name, -n           # Run specific publish config by name
├── deploy                   # Execute remote commands via SSH (deploy.Run)
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

## Package Reference

### config

| Function/Method            | Purpose                             |
| -------------------------- | ----------------------------------- |
| `Load(path)`               | Read and parse YAML config file     |
| `Config.Validate()`        | Validate entire config tree         |
| `BuildConfig.Validate()`   | Validate build config               |
| `BlobConfig.Validate()`    | Validate publish config by provider |
| `DeployConfig.Validate()`  | Validate deploy config by provider  |
| `ArchiveConfig.Validate()` | Validate archive formats            |

### build

| Function/Type         | Purpose                                                          |
| --------------------- | ---------------------------------------------------------------- |
| `Run(ctx, cfg)`       | Main orchestrator: hooks → clean → parallel compile → archive    |
| `Artifact`            | Structured metadata: BinaryName, Version, OS, Arch, Arm, DirPath |
| `ArchiveTemplateData` | Template data for archive naming                                 |

### archive

| Type/Function | Purpose                           |
| ------------- | --------------------------------- |
| `Archiver`    | Interface: Archive(), Extension() |
| `New(format)` | Factory: "tar.gz" or "zip"        |
| `TarGz`       | tar.gz archiver                   |
| `Zip`         | zip archiver                      |

### publish

| Type/Function         | Purpose                                 |
| --------------------- | --------------------------------------- |
| `Publisher`           | Interface: Name(), Publish(ctx, dir, v) |
| `NewPublisher(cfg)`   | Factory from BlobConfig                 |
| `Run(ctx, cfg, name)` | Orchestrate publishing                  |
| `S3Publisher`         | S3/S3-compatible upload via minio       |
| `SSHPublisher`        | SFTP upload via goph                    |

### deploy

| Type/Function         | Purpose                            |
| --------------------- | ---------------------------------- |
| `Deployer`            | Interface: Name(), Deploy(ctx)     |
| `NewDeployer(cfg)`    | Factory from DeployConfig          |
| `Run(ctx, cfg, name)` | Orchestrate deployment with alerts |
| `SSHDeployer`         | SSH command execution              |

### notify

| Function           | Purpose                                      |
| ------------------ | -------------------------------------------- |
| `Send(urls, data)` | Send alert via shoutrrr (filters nil errors) |
| `AlertData`        | AppName, Version, Status, Error              |

### git

| Function                      | Purpose                              |
| ----------------------------- | ------------------------------------ |
| `GetTag(ctx)`                 | Current tag via `git describe`       |
| `GetPreviousTag(ctx)`         | Previous tag for changelog           |
| `GetPreviousStableTag(ctx)`   | Previous stable tag (vX.Y.Z pattern) |
| `GetChangelog(ctx, from, to)` | Markdown changelog between tags      |
| `GetCommitHash(ctx)`          | Short commit hash                    |

### sshutil

| Function/Type             | Purpose                                       |
| ------------------------- | --------------------------------------------- |
| `ClientConfig`            | SSH connection params with Validate()         |
| `NewClient(cfg)`          | Create goph.Client (shared by publish/deploy) |
| `EnsureKnownHost(server)` | Verify/create known_hosts entry               |

### tmpl

| Function              | Purpose                         |
| --------------------- | ------------------------------- |
| `Process(name, t, d)` | Parse and execute text/template |

### hook

| Function          | Purpose                                |
| ----------------- | -------------------------------------- |
| `Run(ctx, hooks)` | Execute hooks via `sh -c` with context |

### shellutil

| Function   | Purpose                          |
| ---------- | -------------------------------- |
| `Quote(s)` | Shell-safe single-quote escaping |

## Data Flow

### Build flow

```
main() → build command
  → config.Load()
  → build.Run(ctx, cfg)
    → hook.Run(ctx, before hooks)
    → clean/create out_dir
    → git.GetTag(ctx), git.GetCommitHash(ctx)
    → extract env var names from ldflags via regex (compiled once)
    → for each build config:
        collect targets (goos × goarch × goarm)
        → tmpl.Process() ldflags
        → parallel exec.CommandContext("go", "build", ...) via errgroup
    → createArchives()
        → for each artifact × archive config:
            → tmpl.Process() archive name
            → archive.New(format).Archive() (parallel via errgroup)
        → remove archived source directories
    → hook.Run(ctx, after hooks)
```

### Publish flow

```
main() → publish command
  → config.Load()
  → publish.Run(ctx, cfg, name)
    → for each blob config (filtered by --name):
        → publish.NewPublisher(cfg) → Publisher
        → publisher.Publish(ctx, artifactsDir, version)
          S3:  → tmpl.Process(directory) → minio PutObject (with ctx)
          SSH: → sshutil.NewClient() → shellutil.Quote(mkdir) → SFTP upload
```

### Deploy flow

```
main() → deploy command
  → config.Load()
  → deploy.Run(ctx, cfg, name)
    → for each deploy config (filtered by --name):
        → deploy.NewDeployer(cfg) → Deployer
        → deployer.Deploy(ctx)
          SSH: → sshutil.NewClient() → execute commands sequentially
        → notify.Send(urls, alertData) with success/failure status
```
