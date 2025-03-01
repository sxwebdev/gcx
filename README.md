# gcx

**gcx** is a lightweight CLI tool for cross-compiling Go binaries and publishing them to cloud storage (e.g., S3). It reads build and release settings from a YAML configuration file (similar to GoReleaser), manages secrets via environment variables or a `.env` file, and automatically uses the current Git tag as the version.

## Features

- **Cross-compilation:** Build Go binaries for multiple OS/architecture combinations.
- **Automated publishing:** Upload build artifacts to S3 (including self-hosted endpoints).
- **Configuration driven:** Use a YAML config file (`.gcx.yaml`) to define build, archive, and publish settings.
- **Versioning:** Automatically determine the version using the current Git tag.
- **CI/CD friendly:** Easily integrate with CI pipelines (e.g., GitLab CI).

## Installation

You can download the pre-built binary from the [releases](#) page or build it from source:

```bash
git clone https://github.com/sxwebdev/gcx.git
cd gcx
go build -ldflags "-X main.version=$(git describe --tags --abbrev=0 2>/dev/null || echo 0.0.0)" -o gcx .
```

Alternatively, you can use the Docker image available on Docker Hub:

```bash
docker pull sxwebdev/gcx:latest
```

## Configuration

Create a YAML configuration file named `.gcx.yaml` in your project root. An example configuration:

```yaml
version: 2

before:
  hooks:
    - go mod tidy

builds:
  - main: ./cmd/myapp
    env:
      - CGO_ENABLED=0
    goos:
      - linux
    goarch:
      - amd64
    flags:
      - -trimpath
    ldflags:
      - -s -w -X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}

archives:
  - formats: ["tar.gz"]
    name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

blobs:
  - provider: s3
    bucket: your-bucket-name
    directory: "releases/{{.ProjectID}}/{{.Version}}"
    region: us-west-1
    endpoint: https://s3.example.com
```

### Template Variables

- **Version:** Automatically set from the current Git tag. If no tag is found, defaults to `0.0.0` (with a log message).
- **ProjectID:** If the `PROJECT_ID` environment variable is not set, the tool uses the name of the current working directory.

## Environment Variables

Set the following environment variables (either in your system or in a `.env` file):

- `AWS_ACCESS_KEY_ID` - Your AWS access key.
- `AWS_SECRET_ACCESS_KEY` - Your AWS secret key.
- `PROJECT_ID` (optional) - Your project identifier. If not provided, the current directory name is used.

You can also set additional variables required for your build or publish process.

## CLI Usage

Once installed, you can run the following commands:

- **Build binaries:**

  ```bash
  gcx build --config .gcx.yaml
  ```

  This command runs any pre-build hooks (e.g., `go mod tidy`) and compiles binaries for the specified targets, storing them in the `dist/` directory.

- **Publish artifacts:**

  ```bash
  gcx publish --config .gcx.yaml
  ```

  This command uploads all files from the `dist/` directory to the configured S3 bucket using the specified settings.

- **Show version:**

  ```bash
  gcx version
  ```

  This prints the current version, commit, and build date of the tool.

## GitLab CI/CD Integration Example

If the `gcx` image is available on Docker Hub, you can integrate it into your GitLab CI pipeline as follows:

```yaml
image: sxwebdev/gcx:latest

stages:
  - build
  - publish

variables:
  GCX_CONFIG: .gcx.yaml

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
```

In this pipeline:

- The `build` stage compiles binaries and stores them in `dist/`.
- The `publish` stage (triggered only when a Git tag is created) uploads the artifacts to S3.
- Ensure that all necessary environment variables (AWS credentials, etc.) are set in your GitLab CI/CD settings.

## License

Distributed under the MIT License. See `LICENSE` for more information.
