# gcx

**gcx** is a lightweight CLI tool for cross-compiling Go binaries and publishing them to cloud storage (e.g., S3). It reads build and release settings from a YAML configuration file (similar to GoReleaser), manages secrets via environment variables or a `.env` file, and automatically uses the current Git tag as the version.

## Features

- **Cross-compilation:** Build Go binaries for multiple OS/architecture combinations.
- **Automated publishing:** Upload build artifacts to S3 (including self-hosted endpoints) or SSH.
- **Configuration driven:** Use a YAML config file (`gcx.yaml`) to define build, archive, and publish settings.
- **Versioning:** Automatically determine the version using the current Git tag.
- **CI/CD friendly:** Easily integrate with CI pipelines (e.g., GitLab CI).
- **Hooks system:** Execute commands before and after build process.
- **Archiving:** Create archives (tar.gz) of your binaries with customizable naming.
- **Deployment:** Deploy your artifacts to servers via SSH with custom commands.

## Installation

You can download the pre-built binary from the [releases](https://github.com/sxwebdev/gcx/releases) page or build it from source:

```bash
git clone https://github.com/sxwebdev/gcx.git
cd gcx
go build -ldflags "-X main.version=$(git describe --tags --abbrev=0)" -o gcx .
```

Alternatively, you can use the Docker image available on Docker Hub:

```bash
docker pull sxwebdev/gcx:latest
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
      - -s -w -X main.version={{.Version}} -X main.commit={{.Commit}}

# Archive configuration
archives:
  - formats: ["tar.gz"]
    name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

# Artifact publishing configuration
blobs:
  - provider: s3
    bucket: your-bucket-name
    directory: "releases/{{.ProjectID}}/{{.Version}}"
    region: us-west-1
    endpoint: https://s3.example.com

  - provider: ssh
    server: "storage.example.com"
    user: "deployer"
    key_path: "~/.ssh/deploy_key"
    directory: "/var/www/releases/{{.ProjectID}}/{{.Version}}"

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
- **ProjectID:** Project identifier (from env or directory name)

## Environment Variables

Set the following environment variables (either in your system or in a `.env` file):

- `AWS_ACCESS_KEY_ID` - Your AWS access key (for S3 provider)
- `AWS_SECRET_ACCESS_KEY` - Your AWS secret key (for S3 provider)
- `PROJECT_ID` (optional) - Your project identifier. If not provided, the current directory name is used.

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

- **Build binaries:**

  ```bash
  gcx build --config gcx.yaml
  ```

  This command:

  1. Runs pre-build hooks
  2. Compiles binaries for specified targets
  3. Creates archives if configured
  4. Runs post-build hooks
  5. Stores results in the output directory

- **Publish artifacts:**

  ```bash
  gcx publish --config gcx.yaml
  ```

  Uploads all files from the output directory to configured destinations (S3 or SSH).

- **Deploy:**

  ```bash
  # Deploy all configurations
  gcx deploy --config gcx.yaml

  # Deploy specific configuration
  gcx deploy --config gcx.yaml --name production
  ```

  Executes deployment commands on target servers via SSH.

- **Show version:**

  ```bash
  gcx version
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
