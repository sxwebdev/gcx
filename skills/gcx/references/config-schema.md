# gcx Configuration Schema Reference

## Table of Contents

- [Top-level Config](#top-level-config)
- [HooksConfig](#hooksconfig)
- [BuildConfig](#buildconfig)
- [ArchiveConfig](#archiveconfig)
- [BlobConfig (Publishing)](#blobconfig-publishing)
- [DeployConfig](#deployconfig)
- [AlertConfig](#alertconfig)
- [Template Variables](#template-variables)
- [Environment Variables](#environment-variables)

---

## Top-level Config

**Go struct:** `Config` in `cmd/gcx/main.go`

| YAML Key   | Type              | Default | Description                          |
| ---------- | ----------------- | ------- | ------------------------------------ |
| `version`  | `int`             | —       | Config file version (currently `1`)  |
| `out_dir`  | `string`          | `dist`  | Output directory for built artifacts |
| `before`   | `HooksConfig`     | —       | Commands to run before build         |
| `after`    | `HooksConfig`     | —       | Commands to run after build          |
| `builds`   | `[]BuildConfig`   | —       | Build configurations                 |
| `archives` | `[]ArchiveConfig` | —       | Archive creation settings            |
| `blobs`    | `[]BlobConfig`    | —       | Artifact publishing destinations     |
| `deploys`  | `[]DeployConfig`  | —       | Deployment configurations            |

## HooksConfig

**Go struct:** `HooksConfig`

| YAML Key | Type       | Description                                                    |
| -------- | ---------- | -------------------------------------------------------------- |
| `hooks`  | `[]string` | Shell commands executed sequentially. Failure stops execution. |

## BuildConfig

**Go struct:** `BuildConfig`

| YAML Key                  | Type       | Default | Description                                         |
| ------------------------- | ---------- | ------- | --------------------------------------------------- |
| `main`                    | `string`   | —       | Path to main Go package (e.g., `./cmd/myapp`)       |
| `output_name`             | `string`   | —       | Binary output name (defaults to dir name of `main`) |
| `disable_platform_suffix` | `bool`     | `false` | Skip adding `_os_arch` suffix to binary name        |
| `goos`                    | `[]string` | —       | Target operating systems (e.g., `linux`, `darwin`)  |
| `goarch`                  | `[]string` | —       | Target architectures (e.g., `amd64`, `arm64`)       |
| `goarm`                   | `[]string` | —       | ARM versions (e.g., `6`, `7`) — only for `arm` arch |
| `flags`                   | `[]string` | —       | Go build flags (e.g., `-trimpath`)                  |
| `ldflags`                 | `[]string` | —       | Linker flags, supports template variables           |
| `env`                     | `[]string` | —       | Environment variables (e.g., `CGO_ENABLED=0`)       |

**Notes:**

- When `goarm` is specified, it generates additional builds for each ARM version combined with the `arm` architecture
- The output binary path is: `{out_dir}/{output_name}_{os}_{arch}[v{arm}]/{output_name}`
- ldflags support Go template syntax: `{{.Version}}`, `{{.Commit}}`, `{{.Date}}`, `{{.Env.VAR}}`

## ArchiveConfig

**Go struct:** `ArchiveConfig`

| YAML Key        | Type       | Default | Description                              |
| --------------- | ---------- | ------- | ---------------------------------------- |
| `formats`       | `[]string` | —       | Archive formats. Currently only `tar.gz` |
| `name_template` | `string`   | —       | Template for archive file name           |

**Name template variables** (via `ArchiveTemplateData`):

| Variable       | Description      |
| -------------- | ---------------- |
| `{{.Binary}}`  | Binary name      |
| `{{.Version}}` | Git tag version  |
| `{{.Os}}`      | Operating system |
| `{{.Arch}}`    | Architecture     |

**Example:** `"{{.Binary}}_{{.Version}}_{{.Os}}_{{.Arch}}"` produces `myapp_v1.0.0_linux_amd64.tar.gz`

## BlobConfig (Publishing)

**Go struct:** `BlobConfig`

This is a union struct — fields are provider-specific. Set `provider` to select which fields apply.

### Common fields

| YAML Key    | Type     | Description                                |
| ----------- | -------- | ------------------------------------------ |
| `provider`  | `string` | `s3` or `ssh`                              |
| `name`      | `string` | Name identifier for this blob config       |
| `directory` | `string` | Remote directory path (supports templates) |

### S3 provider fields

| YAML Key   | Type     | Description                                  |
| ---------- | -------- | -------------------------------------------- |
| `bucket`   | `string` | S3 bucket name                               |
| `region`   | `string` | AWS region                                   |
| `endpoint` | `string` | S3 endpoint URL (for S3-compatible services) |

**Required env vars:** `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`

### SSH provider fields

| YAML Key                   | Type     | Description                            |
| -------------------------- | -------- | -------------------------------------- |
| `server`                   | `string` | SSH server hostname                    |
| `user`                     | `string` | SSH username                           |
| `key_path`                 | `string` | Path to SSH private key (supports `~`) |
| `insecure_ignore_host_key` | `bool`   | Skip host key verification             |

**Validation:** `name`, `server`, `user`, and either `key_path` or `key_raw` are required.

## DeployConfig

**Go struct:** `DeployConfig`

| YAML Key                   | Type          | Default | Description                          |
| -------------------------- | ------------- | ------- | ------------------------------------ |
| `name`                     | `string`      | —       | Deployment name (e.g., `production`) |
| `provider`                 | `string`      | —       | Currently only `ssh`                 |
| `server`                   | `string`      | —       | SSH server hostname                  |
| `user`                     | `string`      | —       | SSH username                         |
| `key_path`                 | `string`      | —       | Path to SSH private key              |
| `key_raw`                  | `string`      | —       | Raw SSH private key content          |
| `insecure_ignore_host_key` | `bool`        | `false` | Skip host key verification           |
| `commands`                 | `[]string`    | —       | Commands to execute on remote server |
| `alerts`                   | `AlertConfig` | —       | Notification settings                |

**Validation:** `name`, `server`, `user`, `commands` (non-empty), and either `key_path` or `key_raw` are required.

## AlertConfig

**Go struct:** `AlertConfig`

| YAML Key | Type       | Description                          |
| -------- | ---------- | ------------------------------------ |
| `urls`   | `[]string` | Notification URLs in shoutrrr format |

### Supported shoutrrr URL formats

| Service  | URL Format                                               |
| -------- | -------------------------------------------------------- |
| Telegram | `telegram://token@telegram?channels=channel-1,channel-2` |
| Slack    | `slack://token-a/token-b/token-c`                        |
| Discord  | `discord://token@channel`                                |
| Teams    | `teams://token-a/token-b/token-c`                        |
| Webhook  | `generic://example.com/webhook?token=token`              |

### Alert message template

The `AlertTemplateData` struct provides:

| Field     | Description                            |
| --------- | -------------------------------------- |
| `AppName` | Deploy name + "-" + deploy config name |
| `Version` | Current git tag                        |
| `Status`  | `Success` or `Failed`                  |
| `Error`   | Error message (empty on success)       |

## Template Variables

Available in ldflags and directory paths:

| Variable            | Source                           | Description                |
| ------------------- | -------------------------------- | -------------------------- |
| `{{.Version}}`      | `git describe --tags --abbrev=0` | Current git tag            |
| `{{.Commit}}`       | `git rev-parse --short HEAD`     | Short commit hash          |
| `{{.Date}}`         | `time.Now().Format(RFC3339)`     | Build timestamp            |
| `{{.Env.VARIABLE}}` | `.env` file or system env        | Environment variable value |
| `{{.Binary}}`       | Archive templates only           | Binary name                |
| `{{.Os}}`           | Archive templates only           | Target OS                  |
| `{{.Arch}}`         | Archive templates only           | Target architecture        |

## Environment Variables

- Variables are loaded from `.env` file via `godotenv` (non-overriding: system env takes precedence)
- **Security:** Only variables explicitly referenced in `{{.Env.X}}` patterns are extracted and made available
- Build-specific env vars (in `builds[].env`) are set as process environment for `go build`
- S3 publishing requires `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` in environment
