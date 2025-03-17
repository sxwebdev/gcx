# gcx

**gcx** is a lightweight CLI tool for cross-compiling Go binaries and publishing them to cloud storage (e.g., S3). It reads build and release settings from a YAML configuration file (similar to GoReleaser), manages secrets via environment variables or a `.env` file, and automatically uses the current Git tag as the version.

## Features

- 🔨 **Cross-compilation:** Build Go binaries for multiple OS/architecture combinations.
- 🚀 **Automated publishing:** Upload build artifacts to S3 (including self-hosted endpoints) or SSH.
- ⚙️ **Configuration driven:** Use a YAML config file (`gcx.yaml`) to define build, archive, and publish settings.
- 🏷️ **Versioning:** Automatically determine the version using the current Git tag.
- 🔄 **CI/CD friendly:** Easily integrate with CI pipelines (e.g., GitLab CI).
- 🎣 **Hooks system:** Execute commands before and after build process.
- 📦 **Archiving:** Create archives (tar.gz) of your binaries with customizable naming.
- 🚢 **Deployment:** Deploy your artifacts to servers via SSH with custom commands.
- 🔔 **Notifications:** Send deployment status alerts to multiple channels (Telegram, Slack, Discord, Teams) using Shoutrrr.

## Installation

You can download the pre-built binary from the [releases](https://github.com/sxwebdev/gcx/releases) page or build it from source:

```bash
go install github.com/sxwebdev/gcx@latest

# or

git clone https://github.com/sxwebdev/gcx.git
cd gcx
make build
```

Alternatively, you can use the Docker image available on Docker Hub:

```bash
docker pull sxwebdev/gcx:latest
```

## How to use

```text
# gcx help

NAME:
   gcx - A tool for cross-compiling and publishing Go binaries

USAGE:
   gcx [global options] command [command options]

COMMANDS:
   build    Compiles binaries
   publish  Publishes artifacts based on the configuration
   deploy   Deploys artifacts based on the configuration
   release  Release related commands
   git      Git related commands
   version  Displays the current version
   config   Configuration related commands
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h  show help
```

### Git related commands

```text
NAME:
   gcx git - Git related commands

USAGE:
   gcx git [command options]

COMMANDS:
   version  Displays the current git tag version
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

## Configuration

Create a YAML configuration file named `gcx.yaml` in your project root. An example configuration:

```yaml
version: 1
out_dir: dist

# Pre-build hooks
before:
  hooks:
    - go mod tidy

# Post-build hooks
after:
  hooks:
    - echo "Build completed!"
    - ./scripts/notify-telegram.sh "New build ready!"

# Build configuration
builds:
  - main: ./cmd/myapp
    output_name: myapp
    disable_platform_suffix: false
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.buildDate={{.Date}}

# Archive configuration
archives:
  - formats: ["tar.gz"]
    name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

# Artifact publishing configuration
blobs:
  - provider: s3
    bucket: your-bucket-name
    directory: "releases/{{.Version}}"
    region: us-west-1
    endpoint: https://s3.example.com

  - provider: ssh
    server: "storage.example.com"
    user: "deployer"
    key_path: "~/.ssh/deploy_key"
    directory: "/var/www/releases/{{.Version}}"

# Deployment configuration
deploys:
  - name: "production"
    provider: "ssh"
    server: "prod.example.com"
    user: "deployer"
    key_path: "~/.ssh/deploy_key"
    commands:
      - systemctl stop myapp
      - cp /var/www/releases/myapp/latest/myapp /usr/local/bin/
      - chmod +x /usr/local/bin/myapp
      - systemctl start myapp
    alerts:
      urls:
        - "telegram://token@telegram?channels=channel-1"
        - "slack://token-a/token-b/token-c"
        - "discord://token@channel"
        - "teams://token-a/token-b/token-c"

  - name: "staging"
    provider: "ssh"
    server: "staging.example.com"
    user: "deployer"
    key_path: "~/.ssh/deploy_key"
    commands:
      - docker-compose -f /opt/myapp/docker-compose.yml down
      - cp /var/www/releases/myapp/latest/myapp /opt/myapp/
      - docker-compose -f /opt/myapp/docker-compose.yml up -d
    alerts:
      urls:
        - "telegram://token@telegram?channels=staging-alerts"
        - "slack://token-a/token-b/token-c"
```

### Template Variables

Available in various template strings throughout the configuration:

- **Version:** Current Git tag (defaults to `0.0.0` if no tag found)
- **Binary:** Name of the binary being built
- **Os:** Target operating system
- **Arch:** Target architecture

## Environment Variables

Set the following environment variables (either in your system or in a `.env` file):

- `AWS_ACCESS_KEY_ID` - AWS access key для S3
- `AWS_SECRET_ACCESS_KEY` - AWS secret key для S3

## Alerts Configuration

The tool supports sending deployment status notifications using [shoutrrr](https://containrrr.dev/shoutrrr/). You can configure alerts for each deployment to notify different channels about success or failure of the deployment.

### Supported Services

- Telegram
- Slack
- Discord
- Microsoft Teams
- And many more (see [shoutrrr services](https://containrrr.dev/shoutrrr/services/overview/))

### URL Formats

Here are examples of URL formats for different services:

```yaml
alerts:
  urls:
    # Telegram
    - "telegram://token@telegram?channels=channel-1,channel-2"

    # Slack
    - "slack://token-a/token-b/token-c"

    # Discord
    - "discord://token@channel"

    # Microsoft Teams
    - "teams://token-a/token-b/token-c"

    # Generic Webhook
    - "generic://example.com/webhook?token=token"
```

### Alert Message Format

The alert message includes:

- Application name (from deploy configuration)
- Version (current Git tag)
- Deployment status (Success/Failed)
- Error details (in case of failure)

Example success message:

```text
Deployment Status Update
Application: myapp-production
Version: v1.2.3
Status: Success
```

Example failure message:

```text
Deployment Status Update
Application: myapp-production
Version: v1.2.3
Status: Failed
Error: command 'systemctl start myapp' failed: exit status 1
```

## CLI Usage

Once installed, you can run the following commands:

```bash
# Initialize a new gcx.yaml configuration file
gcx config init
gcx config init --os linux --arch amd64  # Create config for specific platform
gcx config init --main ./cmd/myapp       # Create config with custom main file
gcx config init --config custom.yaml     # Create config with custom name
gcx config init --force                  # Overwrite existing config

# Build binaries according to configuration
gcx build

# Publish artifacts to configured destinations
gcx publish

# Deploy artifacts using configured deployment settings
gcx deploy
gcx deploy --name production  # Deploy specific configuration

# Show current git tag version
gcx git version

# Generate a changelog between current and previous git tags
gcx release changelog
gcx release changelog --stable  # Compare with previous stable version

# Show gcx version information
gcx version
```

The changelog command generates a markdown-formatted list of changes between the current and previous git tags, including:

- List of changes with commit messages
- Author of each change
- Short commit hash
- Full changelog comparison URL

Example changelog output:

```markdown
## What's Changed

- Add new feature by @author in abc1234
- Fix documentation by @another-author in def5678

**Full Changelog**: https://github.com/user/repo/compare/v0.0.1...v0.0.2
```

### Configuration Initialization

The `config init` command creates a new `gcx.yaml` file with default settings. Available flags:

- `--os, -o`: Target operating system (default: current OS)
- `--arch, -a`: Target architecture (default: current arch)
- `--main, -m`: Path to the main Go file (default: ./cmd/app)
- `--config, -c`: Path to the configuration file (default: gcx.yaml)
- `--force, -f`: Force overwrite existing config file

Example of generated configuration:

```yaml
version: 1
out_dir: dist
builds:
  - main: ./cmd/app
    output_name: myapp
    disable_platform_suffix: false
    goos:
      - linux
    goarch:
      - amd64
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.buildDate={{.Date}}
```

## GitLab CI/CD Integration Example

```yaml
image: sxwebdev/gcx:latest

stages:
  - build
  - publish
  - deploy

variables:
  GCX_CONFIG: gcx.yaml

build:
  stage: build
  script:
    - gcx build --config $GCX_CONFIG
  artifacts:
    paths:
      - dist/

publish:
  stage: publish
  script:
    - gcx publish --config $GCX_CONFIG
  only:
    - tags

deploy:
  stage: deploy
  script:
    - gcx deploy --config $GCX_CONFIG --name production
  only:
    - tags
  when: manual
```

In this pipeline:

- The `build` stage compiles binaries, creates archives, and stores them in `dist/`
- The `publish` stage uploads artifacts to configured destinations
- The `deploy` stage (manual trigger) deploys the application to production
- Ensure all necessary environment variables are set in your GitLab CI/CD settings

## License

Distributed under the MIT License. See `LICENSE` for more information.
