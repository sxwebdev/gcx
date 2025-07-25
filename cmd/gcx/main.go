package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/joho/godotenv"
	"github.com/melbahja/goph"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sxwebdev/gcx/internal/helpers"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

// Version values can be set at build time using ldflags.
var (
	version    = "dev"
	commitHash = "none"
	buildDate  = "none"
)

// Config represents the configuration file structure (similar to GoReleaser configuration).
type Config struct {
	Version  int             `yaml:"version"`
	OutDir   string          `yaml:"out_dir"` // Optional output directory; default is "dist"
	Before   HooksConfig     `yaml:"before,omitempty"`
	After    HooksConfig     `yaml:"after,omitempty"`
	Builds   []BuildConfig   `yaml:"builds,omitempty"`
	Archives []ArchiveConfig `yaml:"archives,omitempty"`
	Blobs    []BlobConfig    `yaml:"blobs,omitempty"`
	Deploys  []DeployConfig  `yaml:"deploys,omitempty"`
}

type HooksConfig struct {
	Hooks []string `yaml:"hooks,omitempty"`
}

type BuildConfig struct {
	Main                  string   `yaml:"main"`
	OutputName            string   `yaml:"output_name,omitempty"`
	DisablePlatformSuffix bool     `yaml:"disable_platform_suffix,omitempty"`
	Goos                  []string `yaml:"goos"`
	Goarch                []string `yaml:"goarch"`
	Goarm                 []string `yaml:"goarm,omitempty"`
	Flags                 []string `yaml:"flags,omitempty"`
	Ldflags               []string `yaml:"ldflags,omitempty"`
	Env                   []string `yaml:"env,omitempty"`
}

type ArchiveConfig struct {
	Formats      []string `yaml:"formats,omitempty"`
	NameTemplate string   `yaml:"name_template,omitempty"`
}

// ArchiveTemplateData contains data for archive name template
type ArchiveTemplateData struct {
	Binary  string
	Version string
	Os      string
	Arch    string
}

type BlobConfig struct {
	Provider string `yaml:"provider"`
	Name     string `yaml:"name"`
	// S3 config fields
	Bucket   string `yaml:"bucket,omitempty"`
	Region   string `yaml:"region,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"`
	// SSH config fields
	Server                string `yaml:"server,omitempty"`
	User                  string `yaml:"user,omitempty"`
	KeyPath               string `yaml:"key_path,omitempty"`
	InsecureIgnoreHostKey bool   `yaml:"insecure_ignore_host_key,omitempty"`
	// Common fields
	Directory string `yaml:"directory"`
}

type DeployConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	// SSH config fields
	Server                string   `yaml:"server,omitempty"`
	User                  string   `yaml:"user,omitempty"`
	KeyPath               string   `yaml:"key_path,omitempty"`
	KeyRaw                string   `yaml:"key_raw,omitempty"`
	InsecureIgnoreHostKey bool     `yaml:"insecure_ignore_host_key,omitempty"`
	Commands              []string `yaml:"commands"`
	// Alert configuration
	Alerts AlertConfig `yaml:"alerts,omitempty"`
}

// AlertConfig contains notification settings
type AlertConfig struct {
	URLs []string `yaml:"urls,omitempty"` // URLs in shoutrrr format
}

// AlertTemplateData contains data for message template
type AlertTemplateData struct {
	AppName string
	Version string
	Status  string
	Error   string
}

// ToS3Config converts BlobConfig to S3Config if provider is s3
func (c *BlobConfig) ToS3Config() *S3Config {
	if c.Provider != "s3" {
		return nil
	}
	return &S3Config{
		Bucket:    c.Bucket,
		Directory: c.Directory,
		Region:    c.Region,
		Endpoint:  c.Endpoint,
	}
}

// ToSSHConfig converts BlobConfig to SSHConfig if provider is ssh
func (c *BlobConfig) ToSSHConfig() *SSHPublishConfig {
	if c.Provider != "ssh" {
		return nil
	}
	return &SSHPublishConfig{
		Name:                  c.Name,
		Server:                c.Server,
		User:                  c.User,
		KeyPath:               c.KeyPath,
		InsecureIgnoreHostKey: c.InsecureIgnoreHostKey,
		Directory:             c.Directory,
	}
}

// ToSSHDeployConfig converts DeployConfig to SSHDeployConfig if provider is ssh
func (c *DeployConfig) ToSSHDeployConfig() *SSHDeployConfig {
	if c.Provider != "ssh" {
		return nil
	}
	return &SSHDeployConfig{
		Name:                  c.Name,
		Server:                c.Server,
		User:                  c.User,
		KeyPath:               c.KeyPath,
		KeyRaw:                c.KeyRaw,
		InsecureIgnoreHostKey: c.InsecureIgnoreHostKey,
		Commands:              c.Commands,
	}
}

// Internal config types for type safety
type S3Config struct {
	Bucket    string
	Directory string
	Region    string
	Endpoint  string
}

type SSHPublishConfig struct {
	Name                  string
	Server                string
	User                  string
	KeyPath               string
	KeyRaw                string
	InsecureIgnoreHostKey bool
	Directory             string
}

// Validate checks if the SSHConfig is valid
func (c *SSHPublishConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.Server == "" {
		return fmt.Errorf("server is required")
	}
	if c.User == "" {
		return fmt.Errorf("user is required")
	}
	if c.KeyPath == "" && c.KeyRaw == "" {
		return fmt.Errorf("either key_path or key_raw is required")
	}
	if c.KeyPath != "" && c.KeyRaw != "" {
		return fmt.Errorf("only one of key_path or key_raw should be provided")
	}
	if c.Directory == "" {
		return fmt.Errorf("directory is required")
	}
	return nil
}

// Internal config types for type safety
type SSHDeployConfig struct {
	Name                  string
	Server                string
	User                  string
	KeyPath               string
	KeyRaw                string
	InsecureIgnoreHostKey bool
	Commands              []string
}

// Validate checks if the SSHCSSHDeployConfigonfig is valid
func (c *SSHDeployConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.Server == "" {
		return fmt.Errorf("server is required")
	}
	if c.User == "" {
		return fmt.Errorf("user is required")
	}
	if c.KeyPath == "" && c.KeyRaw == "" {
		return fmt.Errorf("either key_path or key_raw is required")
	}
	if c.KeyPath != "" && c.KeyRaw != "" {
		return fmt.Errorf("only one of key_path or key_raw should be provided")
	}
	if len(c.Commands) == 0 {
		return fmt.Errorf("at least one command is required")
	}
	return nil
}

// loadConfig reads the YAML configuration from the specified file.
func loadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error loading configuration: %w", err)
	}
	var cfg Config
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing configuration: %w", err)
	}
	return &cfg, nil
}

// runHooks executes the commands from the before.hooks section.
func runHooks(hooks []string) error {
	for _, hook := range hooks {
		log.Printf("Executing hook: %s", hook)
		parts := strings.Fields(hook)
		if len(parts) == 0 {
			continue
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error executing hook '%s': %w", hook, err)
		}
	}
	return nil
}

// getGitTag returns the current git tag if it exists.
// If the tag is not found or an error occurs, it logs a message and returns "0.0.0".
func getGitTag() string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to get git tag: %v. Using default value 0.0.0", err)
		return "0.0.0"
	}
	tag := strings.TrimSpace(string(out))
	if tag == "" {
		log.Println("Git tag is empty, using default value 0.0.0")
		return "0.0.0"
	}
	return tag
}

// getPreviousGitTag returns the previous git tag before the current one
func getPreviousGitTag() string {
	// Get all tags sorted by version
	cmd := exec.Command("git", "tag", "-l", "--sort=-v:refname")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to get git tags: %v. Using default value 0.0.0", err)
		return "0.0.0"
	}

	tags := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(tags) < 2 {
		log.Println("No previous tag found, using default value 0.0.0")
		return "0.0.0"
	}

	// Current tag should be the first one, so return the second one
	currentTag := getGitTag()
	for i, tag := range tags {
		if tag == currentTag && i+1 < len(tags) {
			return tags[i+1]
		}
	}

	return "0.0.0"
}

// getPreviousStableGitTag returns the previous stable git tag (without pre-release suffix)
func getPreviousStableGitTag() string {
	// Get all tags sorted by version
	cmd := exec.Command("git", "tag", "-l", "--sort=-v:refname")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to get git tags: %v. Using default value 0.0.0", err)
		return "0.0.0"
	}

	tags := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(tags) == 0 {
		log.Println("No tags found, using default value 0.0.0")
		return "0.0.0"
	}

	// Regular expression to match stable version tags (vX.Y.Z without any suffix)
	stableTagRegex := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

	currentTag := getGitTag()
	foundCurrent := false

	// Look for the first stable tag that's not the current one
	for _, tag := range tags {
		// If we haven't found current tag yet, check if this is it
		if !foundCurrent && tag == currentTag {
			foundCurrent = true
			continue
		}

		// If this is a stable tag, return it
		if stableTagRegex.MatchString(tag) {
			return tag
		}
	}

	return "0.0.0"
}

// getGitChangelog returns a markdown formatted changelog between two tags
func getGitChangelog(from, to string) (string, error) {
	// Get the repository URL
	remoteCmd := exec.Command("git", "config", "--get", "remote.origin.url")
	remoteOut, err := remoteCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}

	// Convert SSH URL to HTTPS URL if necessary
	repoURL := strings.TrimSpace(string(remoteOut))
	repoURL = strings.TrimSuffix(repoURL, ".git")
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = strings.Replace(repoURL, ":", "/", 1)
		repoURL = strings.Replace(repoURL, "git@", "https://", 1)
	}

	// If previous tag is not set, return empty changelog without error
	if from == "0.0.0" || from == "" {
		return "", nil
	}

	// Get all commits between tags
	cmd := exec.Command("git", "log",
		"--pretty=format:* %s by @%an in %h",
		fmt.Sprintf("%s..%s", from, to))
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git log: %w", err)
	}

	// Create the final markdown
	var sb strings.Builder
	sb.WriteString("## What's Changed\n\n")
	sb.WriteString(string(out) + "\n")
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("**Full Changelog**: %s/compare/%s...%s\n",
		repoURL, from, to))

	return sb.String(), nil
}

// getGitCommitHash returns the current git commit hash
func getGitCommitHash() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to get git commit hash: %v. Using default value 'none'", err)
		return "none"
	}
	return strings.TrimSpace(string(out))
}

// getOutputFilename returns the output filename based on the configuration and platform.
func getOutputFilename(
	usePlatformSuffix bool,
	outDir, binaryBase, version, goos, goarch, goarm string,
) string {
	var outputName string
	if usePlatformSuffix {
		if goarch == "arm" && goarm != "" {
			outputName = fmt.Sprintf("%s/%s_%s_%s_%s_%s/%s", outDir, binaryBase, version, goos, goarch, goarm, binaryBase)
		} else {
			outputName = fmt.Sprintf("%s/%s_%s_%s_%s/%s", outDir, binaryBase, version, goos, goarch, binaryBase)
		}
	} else {
		outputName = fmt.Sprintf("%s/%s_%s/%s", outDir, binaryBase, version, binaryBase)
	}
	return outputName
}

// buildBinaries performs cross-compilation of binaries according to the configuration.
func buildBinaries(cfg *Config) error {
	// Execute hooks (e.g., "go mod tidy")
	if len(cfg.Before.Hooks) > 0 {
		if err := runHooks(cfg.Before.Hooks); err != nil {
			return err
		}
	}

	// Determine the output directory (default is "dist")
	outDir := cfg.OutDir
	if outDir == "" {
		outDir = "dist"
	}

	// Clean the output directory if it exists
	if _, err := os.Stat(outDir); err == nil {
		if err := os.RemoveAll(outDir); err != nil {
			return fmt.Errorf("failed to clean output directory: %w", err)
		}
	}

	// Create the build directory
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get current git tag and commit hash for ldflags
	currentTag := getGitTag()
	commitHash := getGitCommitHash()
	buildDate := time.Now().Format(time.RFC3339)

	// Extract environment variable names from ldflags
	envVarNames := make(map[string]bool)
	for _, buildCfg := range cfg.Builds {
		for _, ldflag := range buildCfg.Ldflags {
			// Find all occurrences of {{.Env.VARIABLE_NAME}}
			matches := regexp.MustCompile(`{{\.Env\.([^}]+)}}`).FindAllStringSubmatch(ldflag, -1)
			for _, match := range matches {
				if len(match) > 1 {
					envVarNames[match[1]] = true
				}
			}
		}
	}

	// Create a map of environment variables that are actually used
	envVars := make(map[string]string)
	for name := range envVarNames {
		if value := os.Getenv(name); value != "" {
			envVars[name] = value
		}
	}

	// For each build configuration
	for _, buildCfg := range cfg.Builds {
		// Determine the binary name
		var binaryBase string
		if buildCfg.OutputName != "" {
			binaryBase = buildCfg.OutputName
		} else {
			// Use the last element of the path
			parts := strings.Split(buildCfg.Main, "/")
			binaryBase = parts[len(parts)-1]
		}

		// Determine if we should add platform suffix
		usePlatformSuffix := !buildCfg.DisablePlatformSuffix

		// Process ldflags templates
		var processedLdflags []string
		for _, ldflag := range buildCfg.Ldflags {
			tmpl, err := template.New("ldflag").Parse(ldflag)
			if err != nil {
				return fmt.Errorf("failed to parse ldflag template '%s': %w", ldflag, err)
			}

			var buf strings.Builder
			data := struct {
				Version string
				Commit  string
				Date    string
				Env     map[string]string
			}{
				Version: currentTag,
				Commit:  commitHash,
				Date:    buildDate,
				Env:     envVars,
			}

			if err := tmpl.Execute(&buf, data); err != nil {
				return fmt.Errorf("failed to execute ldflag template '%s': %w", ldflag, err)
			}

			processedLdflags = append(processedLdflags, buf.String())
		}

		eg := errgroup.Group{}
		eg.SetLimit(runtime.NumCPU())

		log.Printf("Use %d CPU cores for building...\n", runtime.NumCPU())

		// Iterate over all combinations of GOOS and GOARCH
		for _, goos := range buildCfg.Goos {
			eg.Go(func() error {
				for _, goarch := range buildCfg.Goarch {
					// If the architecture is arm and OS is not linux, skip build
					if goarch == "arm" && goos != "linux" {
						continue
					}
					// If architecture is arm and goarm parameters are provided, iterate over them
					if goarch == "arm" && len(buildCfg.Goarm) > 0 {
						for _, goarm := range buildCfg.Goarm {
							envs := os.Environ()
							envs = append(envs, "GOOS="+goos, "GOARCH="+goarch, "GOARM="+goarm)
							envs = append(envs, buildCfg.Env...)
							outputName := getOutputFilename(
								usePlatformSuffix, outDir, binaryBase, currentTag, goos, goarch, goarm,
							)
							args := []string{"build"}
							args = append(args, buildCfg.Flags...)
							if len(processedLdflags) > 0 {
								args = append(args, "-ldflags", strings.Join(processedLdflags, " "))
							}
							args = append(args, "-o", outputName, buildCfg.Main)
							log.Printf("Building %s for %s/%s arm%s...", binaryBase, goos, goarch, goarm)
							cmd := exec.Command("go", args...)
							cmd.Env = envs
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							if err := cmd.Run(); err != nil {
								return fmt.Errorf("build error: %w", err)
							}
						}
					} else {
						envs := os.Environ()
						envs = append(envs, "GOOS="+goos, "GOARCH="+goarch)
						envs = append(envs, buildCfg.Env...)
						outputName := getOutputFilename(
							usePlatformSuffix, outDir, binaryBase, currentTag, goos, goarch, "",
						)
						args := []string{"build"}
						args = append(args, buildCfg.Flags...)
						if len(processedLdflags) > 0 {
							args = append(args, "-ldflags", strings.Join(processedLdflags, " "))
						}
						args = append(args, "-o", outputName, buildCfg.Main)
						log.Printf("Building %s for %s/%s...", binaryBase, goos, goarch)
						cmd := exec.Command("go", args...)
						cmd.Env = envs
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						if err := cmd.Run(); err != nil {
							return fmt.Errorf("build error: %w", err)
						}
					}
				}
				return nil
			})
		}

		if err := eg.Wait(); err != nil {
			return fmt.Errorf("build error: %w", err)
		}
	}

	// Create archives after successful build
	if err := createArchives(cfg, outDir); err != nil {
		return fmt.Errorf("failed to create archives: %w", err)
	}

	// Execute after hooks
	if len(cfg.After.Hooks) > 0 {
		if err := runHooks(cfg.After.Hooks); err != nil {
			return err
		}
	}

	return nil
}

// publishArtifacts uploads artifacts to configured destinations
func publishArtifacts(cfg *Config, publishName string) error {
	// Determine the artifacts directory (default is "dist")
	artifactsDir := cfg.OutDir
	if artifactsDir == "" {
		artifactsDir = "dist"
	}

	// Get the current git tag as the version.
	tag := getGitTag()

	tmplData := map[string]string{
		"Version": tag,
	}

	var blobs []BlobConfig

	// If publishName is specified, execute only that publish
	if publishName != "" {
		var found bool
		for _, config := range cfg.Blobs {
			if config.Name == publishName {
				blobs = append(blobs, config)
				found = true
			}
		}
		if !found {
			return fmt.Errorf("publish configuration '%s' not found", publishName)
		}
	} else {
		// Otherwise, execute all publish configurations
		blobs = cfg.Blobs
	}

	// Process each blob configuration
	for _, blob := range blobs {
		fmt.Println("Publishing to:", blob.Name)

		switch blob.Provider {
		case "s3":
			if err := publishToS3(blob.ToS3Config(), artifactsDir, tmplData); err != nil {
				return fmt.Errorf("s3 publish error: %w", err)
			}
		case "ssh":
			if err := publishToSSH(blob.ToSSHConfig(), artifactsDir, tmplData); err != nil {
				return fmt.Errorf("ssh publish error: %w", err)
			}
		default:
			log.Printf("Skipping unknown provider: %s", blob.Provider)
		}
	}
	return nil
}

// publishToS3 uploads artifacts to S3 storage
func publishToS3(cfg *S3Config, artifactsDir string, tmplData map[string]string) error {
	if cfg == nil {
		return fmt.Errorf("s3 configuration is required for s3 provider")
	}

	// Read AWS credentials from environment variables
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set")
	}

	// Process template for the publish directory
	tmpl, err := template.New("directory").Parse(cfg.Directory)
	if err != nil {
		return fmt.Errorf("error parsing directory template: %w", err)
	}
	var dirBuffer strings.Builder
	if err = tmpl.Execute(&dirBuffer, tmplData); err != nil {
		return fmt.Errorf("error executing directory template: %w", err)
	}
	remoteDir := dirBuffer.String()

	// Parse endpoint to extract host
	urlData, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return fmt.Errorf("error parsing endpoint: %w", err)
	}

	// Create an S3 client using minio-go
	s3Client, err := minio.New(urlData.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: strings.HasPrefix(cfg.Endpoint, "https"),
		Region: cfg.Region,
	})
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Check if the bucket exists and create it if necessary
	ctx := context.Background()
	exists, err := s3Client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return fmt.Errorf("bucket check error: %w", err)
	}
	if !exists {
		log.Printf("Bucket %s does not exist, creating...", cfg.Bucket)
		if err = s3Client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	// Upload all files from the artifacts directory
	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", artifactsDir, err)
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		localFilePath := filepath.Join(artifactsDir, file.Name())
		remotePath := filepath.Join(remoteDir, file.Name())
		log.Printf("Uploading %s to s3://%s/%s", localFilePath, cfg.Bucket, remotePath)
		f, err := os.Open(localFilePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", localFilePath, err)
		}
		stat, err := f.Stat()
		if err != nil {
			f.Close()
			return fmt.Errorf("failed to get file info for %s: %w", localFilePath, err)
		}
		_, err = s3Client.PutObject(ctx, cfg.Bucket, remotePath, f, stat.Size(), minio.PutObjectOptions{})
		f.Close()
		if err != nil {
			return fmt.Errorf("failed to upload file %s: %w", localFilePath, err)
		}
	}
	return nil
}

// publishToSSH uploads artifacts to remote server via SSH
func publishToSSH(cfg *SSHPublishConfig, artifactsDir string, tmplData map[string]string) error {
	if cfg == nil {
		return fmt.Errorf("ssh configuration is required for ssh provider")
	}

	// Validate SSH configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid SSH configuration: %w", err)
	}

	// Process template for the publish directory
	tmpl, err := template.New("directory").Parse(cfg.Directory)
	if err != nil {
		return fmt.Errorf("error parsing directory template: %w", err)
	}
	var dirBuffer strings.Builder
	if err = tmpl.Execute(&dirBuffer, tmplData); err != nil {
		return fmt.Errorf("error executing directory template: %w", err)
	}
	remoteDir := dirBuffer.String()

	// Check if known_hosts file exists and create it if it doesn't
	if !cfg.InsecureIgnoreHostKey {
		if err := checkKnonwnHost(cfg.Server); err != nil {
			return fmt.Errorf("failed to check known_hosts file: %w", err)
		}
	}

	// Create SSH client
	var auth goph.Auth
	if cfg.KeyRaw != "" {
		auth, err = goph.RawKey(cfg.KeyRaw, "")
		if err != nil {
			return fmt.Errorf("failed to load SSH key: %w", err)
		}
	} else {
		path, err := helpers.ExpandPath(cfg.KeyPath)
		if err != nil {
			return fmt.Errorf("failed to expand key path: %w", err)
		}

		auth, err = goph.Key(path, "")
		if err != nil {
			return fmt.Errorf("failed to load SSH key: %w", err)
		}
	}

	var client *goph.Client
	if cfg.InsecureIgnoreHostKey {
		client, err = goph.NewUnknown(cfg.User, cfg.Server, auth)
		if err != nil {
			return fmt.Errorf("failed to create insecure SSH client: %w", err)
		}
	} else {
		client, err = goph.New(cfg.User, cfg.Server, auth)
		if err != nil {
			return fmt.Errorf("failed to create SSH client: %w", err)
		}
	}
	defer client.Close()

	// Create remote directory if it doesn't exist
	if _, err := client.Run(fmt.Sprintf("mkdir -p %s", remoteDir)); err != nil {
		return fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Upload all files from the artifacts directory
	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", artifactsDir, err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		localFilePath := filepath.Join(artifactsDir, file.Name())
		remotePath := filepath.Join(remoteDir, file.Name())
		log.Printf("Uploading %s to %s:%s", localFilePath, cfg.Server, remotePath)

		if err := client.Upload(localFilePath, remotePath); err != nil {
			return fmt.Errorf("failed to upload file %s: %w", localFilePath, err)
		}
	}

	return nil
}

// createArchives creates archives for all built binaries
func createArchives(cfg *Config, artifactsDir string) error {
	if len(cfg.Archives) == 0 {
		return nil
	}

	// Read all files in artifacts directory
	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		return fmt.Errorf("failed to read artifacts directory: %w", err)
	}

	eg := errgroup.Group{}
	eg.SetLimit(runtime.NumCPU())

	log.Printf("Use %d CPU cores for creating archives...\n", runtime.NumCPU())

	// Track which files were archived
	archivedFiles := make(map[string]bool)

	// Create archives for each file/directory according to configuration
	for _, file := range files {
		fileName := file.Name()

		parts := strings.Split(fileName, "_")
		if len(parts) < 4 {
			continue
		}

		binaryName := parts[0]
		version := parts[1]
		os := parts[2]
		arch := parts[3]

		// Template data
		tmplData := ArchiveTemplateData{
			Binary:  binaryName,
			Version: version,
			Os:      os,
			Arch:    arch,
		}

		// For each archive configuration
		for _, archive := range cfg.Archives {
			// Create archive name from template
			archiveName := fileName
			if archive.NameTemplate != "" {
				tmpl, err := template.New("archive").Parse(archive.NameTemplate)
				if err != nil {
					return fmt.Errorf("failed to parse archive name template: %w", err)
				}

				var nameBuffer strings.Builder
				if err := tmpl.Execute(&nameBuffer, tmplData); err != nil {
					return fmt.Errorf("failed to execute archive name template: %w", err)
				}

				archiveName = nameBuffer.String()
			}

			// For each archive format
			for _, format := range archive.Formats {
				archiveFileName := archiveName + "." + format
				archivePath := filepath.Join(artifactsDir, archiveFileName)
				sourcePath := filepath.Join(artifactsDir, fileName)

				switch format {
				case "tar.gz":
					// Mark the source file/directory as archived
					archivedFiles[fileName] = true

					eg.Go(func() error {
						if err := createTarGz(sourcePath, archivePath); err != nil {
							return fmt.Errorf("failed to create tar.gz archive: %w", err)
						}

						return nil
					})
				default:
					log.Printf("Unsupported archive format: %s", format)
				}
			}
		}
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("error creating archives: %w", err)
	}

	// Remove all source files/directories that were archived
	for _, file := range files {
		fileName := file.Name()
		if archivedFiles[fileName] {
			filePath := filepath.Join(artifactsDir, fileName)
			if file.IsDir() {
				if err := os.RemoveAll(filePath); err != nil {
					log.Printf("Warning: failed to remove source directory %s: %v", filePath, err)
				}
			} else {
				if err := os.Remove(filePath); err != nil {
					log.Printf("Warning: failed to remove source file %s: %v", filePath, err)
				}
			}
		}
	}

	log.Printf("All archives created successfully.")

	return nil
}

// createTarGz creates a tar.gz archive from a file or directory
func createTarGz(srcPath, destFile string) error {
	// Create archive file
	archive, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}
	defer archive.Close()

	// Create gzip writer
	gw := gzip.NewWriter(archive)
	defer gw.Close()

	// Create tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Check if source is file or directory
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("failed to get source info: %w", err)
	}

	if srcInfo.IsDir() {
		// Archive directory - use directory name as base
		dirName := filepath.Base(srcPath)
		return addDirToTar(tw, srcPath, dirName)
	} else {
		// Archive single file
		return addFileToTar(tw, srcPath, filepath.Base(srcPath))
	}
}

// addFileToTar adds a single file to tar archive
func addFileToTar(tw *tar.Writer, filePath, nameInTar string) error {
	// Open source file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer file.Close()

	// Get file info
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Create tar header
	header := &tar.Header{
		Name:    nameInTar,
		Size:    stat.Size(),
		Mode:    int64(stat.Mode()),
		ModTime: stat.ModTime(),
	}

	// Write header
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	// Copy file contents to archive
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("failed to write file to tar: %w", err)
	}

	return nil
}

// addDirToTar recursively adds directory contents to tar archive
func addDirToTar(tw *tar.Writer, dirPath, baseInTar string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path for tar
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		var nameInTar string
		if relPath == "." {
			// This is the root directory itself
			nameInTar = baseInTar
		} else {
			// Combine with base path in tar
			nameInTar = filepath.Join(baseInTar, relPath)
		}

		// Handle directories
		if info.IsDir() {
			header := &tar.Header{
				Name:     nameInTar + "/",
				Mode:     int64(info.Mode()),
				ModTime:  info.ModTime(),
				Typeflag: tar.TypeDir,
			}
			return tw.WriteHeader(header)
		}

		// Handle regular files
		return addFileToTar(tw, path, nameInTar)
	})
}

// deployArtifacts executes deployment according to the configuration
func deployArtifacts(cfg *Config, deployName string) error {
	if len(cfg.Deploys) == 0 {
		return fmt.Errorf("no deploy configurations found")
	}

	// If deployName is specified, execute only that deploy
	if deployName != "" {
		for _, deploy := range cfg.Deploys {
			if deploy.Name == deployName {
				return executeDeploy(&deploy)
			}
		}
		return fmt.Errorf("deploy configuration '%s' not found", deployName)
	}

	// Execute all deploys
	for _, deploy := range cfg.Deploys {
		if err := executeDeploy(&deploy); err != nil {
			return fmt.Errorf("deploy '%s' failed: %w", deploy.Name, err)
		}
	}

	return nil
}

// sendAlert sends notification through shoutrrr
func sendAlert(urls []string, tmplData AlertTemplateData) error {
	if len(urls) == 0 {
		return nil
	}

	// Create message template
	const msgTemplate = `
Deployment Status Update
Application: {{.AppName}}
Version: {{.Version}}
Status: {{.Status}}
{{if .Error}}Error: {{.Error}}{{end}}
`

	tmpl, err := template.New("alert").Parse(msgTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse alert template: %w", err)
	}

	var msgBuffer strings.Builder
	if err := tmpl.Execute(&msgBuffer, tmplData); err != nil {
		return fmt.Errorf("failed to execute alert template: %w", err)
	}

	// Create sender for all URLs
	sender, err := shoutrrr.CreateSender(urls...)
	if err != nil {
		return fmt.Errorf("failed to create alert sender: %w", err)
	}

	// Send notification
	errs := sender.Send(msgBuffer.String(), nil)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Printf("failed to send alert: %v", err)
		}
		return fmt.Errorf("failed to send alerts")
	}

	return nil
}

// executeDeploy executes a single deployment configuration
func executeDeploy(deploy *DeployConfig) error {
	log.Printf("Executing deploy: %s", deploy.Name)

	// Get current version for notifications
	version := getGitTag()

	// Prepare notification data
	alertData := AlertTemplateData{
		AppName: deploy.Name,
		Version: version,
	}

	var deployErr error
	switch deploy.Provider {
	case "ssh":
		deployErr = executeSSHDeploy(deploy.ToSSHDeployConfig())
	default:
		deployErr = fmt.Errorf("unsupported deploy provider: %s", deploy.Provider)
	}

	// Send notification based on result
	if deployErr != nil {
		alertData.Status = "Failed"
		alertData.Error = deployErr.Error()
		// Send error notification
		if err := sendAlert(deploy.Alerts.URLs, alertData); err != nil {
			log.Printf("Failed to send failure alert: %v", err)
		}
		return deployErr
	}

	// Send success notification
	alertData.Status = "Success"
	if err := sendAlert(deploy.Alerts.URLs, alertData); err != nil {
		log.Printf("Failed to send success alert: %v", err)
	}

	return nil
}

// executeSSHDeploy executes deployment commands over SSH
func executeSSHDeploy(cfg *SSHDeployConfig) error {
	if cfg == nil {
		return fmt.Errorf("ssh configuration is required for ssh provider")
	}

	// Validate SSH configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid SSH configuration: %w", err)
	}

	// Check if known_hosts file exists and create it if it doesn't
	if !cfg.InsecureIgnoreHostKey {
		if err := checkKnonwnHost(cfg.Server); err != nil {
			return fmt.Errorf("failed to check known_hosts file: %w", err)
		}
	}

	// Create SSH client
	var auth goph.Auth
	var err error
	if cfg.KeyRaw != "" {
		auth, err = goph.RawKey(cfg.KeyRaw, "")
		if err != nil {
			return fmt.Errorf("failed to load SSH key: %w", err)
		}
	} else {
		path, err := helpers.ExpandPath(cfg.KeyPath)
		if err != nil {
			return fmt.Errorf("failed to expand key path: %w", err)
		}

		auth, err = goph.Key(path, "")
		if err != nil {
			return fmt.Errorf("failed to load SSH key: %w", err)
		}
	}

	var client *goph.Client
	if cfg.InsecureIgnoreHostKey {
		client, err = goph.NewUnknown(cfg.User, cfg.Server, auth)
		if err != nil {
			return fmt.Errorf("failed to create insecure SSH client: %w", err)
		}
	} else {
		client, err = goph.New(cfg.User, cfg.Server, auth)
		if err != nil {
			return fmt.Errorf("failed to create SSH client: %w", err)
		}
	}
	defer client.Close()

	// Execute each command
	for _, cmd := range cfg.Commands {
		log.Printf("Executing command: %s", cmd)
		out, err := client.Run(cmd)
		if err != nil {
			return fmt.Errorf("command '%s' failed: %w", cmd, err)
		}
		log.Printf("Command output:\n%s", string(out))
	}

	return nil
}

func checkKnonwnHost(server string) error {
	// Check if known_hosts file exists and create it if it doesn't
	knownHostsPath, err := helpers.ExpandPath("~/.ssh/known_hosts")
	if err != nil {
		return fmt.Errorf("failed to expand known hosts path: %w", err)
	}

	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		// Create ~/.ssh directory if it doesn't exist
		sshDir := filepath.Dir(knownHostsPath)
		if err := os.MkdirAll(sshDir, 0o700); err != nil {
			return fmt.Errorf("failed to create .ssh directory: %w", err)
		}

		// Create empty known_hosts file
		if err := os.WriteFile(knownHostsPath, []byte{}, 0o600); err != nil {
			return fmt.Errorf("failed to create known_hosts file: %w", err)
		}

		// Run ssh-keyscan to add the server to known_hosts
		cmd := exec.Command("ssh-keyscan", "-H", server)
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("ssh-keyscan failed: %w", err)
		}

		// Append the output to the known_hosts file
		f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("failed to open known_hosts file: %w", err)
		}
		defer f.Close()

		if _, err := f.Write(output); err != nil {
			return fmt.Errorf("failed to write to known_hosts file: %w", err)
		}
	}

	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL)
	defer cancel()

	// Load environment variables from .env file, if it exists.
	godotenv.Load()

	app := &cli.Command{
		Name:  "gcx",
		Usage: "A tool for cross-compiling and publishing Go binaries",
		Commands: []*cli.Command{
			{
				Name:  "build",
				Usage: "Compiles binaries",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "Path to the YAML configuration file",
						Value:   "gcx.yaml",
					},
				},
				Action: func(_ context.Context, c *cli.Command) error {
					configPath := c.String("config")
					cfg, err := loadConfig(configPath)
					if err != nil {
						return fmt.Errorf("error loading configuration: %w", err)
					}
					return buildBinaries(cfg)
				},
			},
			{
				Name:  "publish",
				Usage: "Publishes artifacts based on the configuration",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "Path to the YAML configuration file",
						Value:   "gcx.yaml",
					},
					&cli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "Name of the publish configuration to execute",
					},
				},
				Action: func(_ context.Context, c *cli.Command) error {
					configPath := c.String("config")
					cfg, err := loadConfig(configPath)
					if err != nil {
						return fmt.Errorf("error loading configuration: %w", err)
					}
					return publishArtifacts(cfg, c.String("name"))
				},
			},
			{
				Name:  "deploy",
				Usage: "Deploys artifacts based on the configuration",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "Path to the YAML configuration file",
						Value:   "gcx.yaml",
					},
					&cli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "Name of the deploy configuration to execute",
					},
				},
				Action: func(_ context.Context, c *cli.Command) error {
					configPath := c.String("config")
					cfg, err := loadConfig(configPath)
					if err != nil {
						return fmt.Errorf("error loading configuration: %w", err)
					}
					return deployArtifacts(cfg, c.String("name"))
				},
			},
			{
				Name:  "release",
				Usage: "Release related commands",
				Commands: []*cli.Command{
					{
						Name:  "changelog",
						Usage: "Generate a changelog between the current and previous git tags",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "stable",
								Aliases: []string{"s"},
								Usage:   "Compare with previous stable version (vX.Y.Z without pre-release suffix)",
							},
						},
						Action: func(_ context.Context, c *cli.Command) error {
							currentTag := getGitTag()
							var previousTag string
							if c.Bool("stable") {
								previousTag = getPreviousStableGitTag()
							} else {
								previousTag = getPreviousGitTag()
							}

							changelog, err := getGitChangelog(previousTag, currentTag)
							if err != nil {
								return fmt.Errorf("failed to generate changelog: %w", err)
							}

							fmt.Println(changelog)
							return nil
						},
					},
				},
			},
			{
				Name:  "git",
				Usage: "Git related commands",
				Commands: []*cli.Command{
					{
						Name:  "version",
						Usage: "Displays the current git tag version",
						Action: func(_ context.Context, c *cli.Command) error {
							tag := getGitTag()
							fmt.Printf("Current git version: %s\n", tag)
							return nil
						},
					},
				},
			},
			{
				Name:  "version",
				Usage: "Displays the current version",
				Action: func(_ context.Context, c *cli.Command) error {
					fmt.Printf("gcx version: %s\ncommit: %s\nbuild date: %s\n", version, commitHash, buildDate)
					return nil
				},
			},
			{
				Name:  "config",
				Usage: "Configuration related commands",
				Commands: []*cli.Command{
					{
						Name:  "init",
						Usage: "Initialize a new gcx.yaml configuration file",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "os",
								Aliases: []string{"o"},
								Usage:   "Target operating system",
								Value:   runtime.GOOS,
							},
							&cli.StringFlag{
								Name:    "arch",
								Aliases: []string{"a"},
								Usage:   "Target architecture",
								Value:   runtime.GOARCH,
							},
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "Force overwrite existing config file",
								Value:   false,
							},
							&cli.StringFlag{
								Name:    "config",
								Aliases: []string{"c"},
								Usage:   "Path to the configuration file",
								Value:   "gcx.yaml",
							},
							&cli.StringFlag{
								Name:    "main",
								Aliases: []string{"m"},
								Usage:   "Path to the main Go file",
								Value:   "./cmd/app",
							},
						},
						Action: func(_ context.Context, c *cli.Command) error {
							configPath := c.String("config")
							if _, err := os.Stat(configPath); err == nil && !c.Bool("force") {
								return fmt.Errorf("%s already exists. Use --force / -f to overwrite", configPath)
							}

							config := &Config{
								Version: 1,
								OutDir:  "dist",
								Builds: []BuildConfig{
									{
										Main:   c.String("main"),
										Goos:   []string{c.String("os")},
										Goarch: []string{c.String("arch")},
										Flags:  []string{"-trimpath"},
										Ldflags: []string{
											"-s -w",
											"-X main.version={{.Version}}",
											"-X main.commit={{.Commit}}",
											"-X main.buildDate={{.Date}}",
										},
									},
								},
							}

							buf := bytes.NewBuffer(nil)
							marshaller := yaml.NewEncoder(buf)
							defer marshaller.Close()

							marshaller.SetIndent(2)

							if err := marshaller.Encode(config); err != nil {
								return fmt.Errorf("failed to marshal config: %w", err)
							}

							if err := os.WriteFile(configPath, buf.Bytes(), 0o644); err != nil {
								return fmt.Errorf("failed to write config file: %w", err)
							}

							fmt.Printf("Created %s with default configuration\n", configPath)
							return nil
						},
					},
				},
			},
		},
	}

	if err := app.Run(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}
