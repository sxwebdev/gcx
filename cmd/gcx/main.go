package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/joho/godotenv"
	"github.com/melbahja/goph"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

// Version values can be set at build time using ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = time.Now().Format(time.RFC3339)
)

// Config represents the configuration file structure (similar to GoReleaser configuration).
type Config struct {
	Version  int             `yaml:"version"`
	OutDir   string          `yaml:"out_dir"` // Optional output directory; default is "dist"
	Before   HooksConfig     `yaml:"before"`
	After    HooksConfig     `yaml:"after"`
	Builds   []BuildConfig   `yaml:"builds"`
	Archives []ArchiveConfig `yaml:"archives"`
	Blobs    []BlobConfig    `yaml:"blobs"`
	Deploys  []DeployConfig  `yaml:"deploys"`
}

type HooksConfig struct {
	Hooks []string `yaml:"hooks"`
}

type BuildConfig struct {
	Main    string   `yaml:"main"`
	Env     []string `yaml:"env"`
	Goos    []string `yaml:"goos"`
	Goarch  []string `yaml:"goarch"`
	Goarm   []string `yaml:"goarm"`
	Flags   []string `yaml:"flags"`
	Ldflags []string `yaml:"ldflags"`
}

type ArchiveConfig struct {
	Formats      []string `yaml:"formats"`
	NameTemplate string   `yaml:"name_template"`
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
	// S3 config fields
	Bucket   string `yaml:"bucket,omitempty"`
	Region   string `yaml:"region,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"`
	// SSH config fields
	Server  string `yaml:"server,omitempty"`
	User    string `yaml:"user,omitempty"`
	KeyPath string `yaml:"key_path,omitempty"`
	// Common fields
	Directory string `yaml:"directory"`
}

type DeployConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	// SSH config fields
	Server   string   `yaml:"server,omitempty"`
	User     string   `yaml:"user,omitempty"`
	KeyPath  string   `yaml:"key_path,omitempty"`
	Commands []string `yaml:"commands"`
	// Alert configuration
	Alerts AlertConfig `yaml:"alerts"`
}

// AlertConfig contains notification settings
type AlertConfig struct {
	URLs []string `yaml:"urls"` // URLs in shoutrrr format
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
func (c *BlobConfig) ToSSHConfig() *SSHConfig {
	if c.Provider != "ssh" {
		return nil
	}
	return &SSHConfig{
		Server:    c.Server,
		User:      c.User,
		KeyPath:   c.KeyPath,
		Directory: c.Directory,
	}
}

// ToSSHDeployConfig converts DeployConfig to SSHDeployConfig if provider is ssh
func (c *DeployConfig) ToSSHDeployConfig() *SSHDeployConfig {
	if c.Provider != "ssh" {
		return nil
	}
	return &SSHDeployConfig{
		Name:     c.Name,
		Server:   c.Server,
		User:     c.User,
		KeyPath:  c.KeyPath,
		Commands: c.Commands,
	}
}

// Internal config types for type safety
type S3Config struct {
	Bucket    string
	Directory string
	Region    string
	Endpoint  string
}

type SSHConfig struct {
	Server    string
	User      string
	KeyPath   string
	Directory string
}

// Internal config types for type safety
type SSHDeployConfig struct {
	Name     string
	Server   string
	User     string
	KeyPath  string
	Commands []string
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

	// Create the build directory
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// For each build configuration
	for _, buildCfg := range cfg.Builds {
		// Determine the binary name from the last element of the path (e.g., "./cmd/rovercore" → "rovercore")
		parts := strings.Split(buildCfg.Main, "/")
		binaryBase := parts[len(parts)-1]

		// Iterate over all combinations of GOOS and GOARCH
		for _, goos := range buildCfg.Goos {
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
						outputName := fmt.Sprintf("%s/%s_%s_%s_arm%s", outDir, binaryBase, goos, goarch, goarm)
						args := []string{"build"}
						args = append(args, buildCfg.Flags...)
						if len(buildCfg.Ldflags) > 0 {
							args = append(args, "-ldflags", strings.Join(buildCfg.Ldflags, " "))
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
					outputName := fmt.Sprintf("%s/%s_%s_%s", outDir, binaryBase, goos, goarch)
					args := []string{"build"}
					args = append(args, buildCfg.Flags...)
					if len(buildCfg.Ldflags) > 0 {
						args = append(args, "-ldflags", strings.Join(buildCfg.Ldflags, " "))
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
func publishArtifacts(cfg *Config) error {
	// Determine the artifacts directory (default is "dist")
	artifactsDir := cfg.OutDir
	if artifactsDir == "" {
		artifactsDir = "dist"
	}

	// Get the current git tag as the version.
	tag := getGitTag()

	// Determine ProjectID: if the environment variable is not set, use the name of the working directory.
	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		wd, err := os.Getwd()
		if err != nil {
			log.Printf("Failed to determine working directory: %v", err)
			projectID = "default"
		} else {
			projectID = filepath.Base(wd)
		}
	}

	tmplData := map[string]string{
		"Version":   tag,
		"ProjectID": projectID,
	}

	// Process each blob configuration
	for _, blob := range cfg.Blobs {
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
func publishToSSH(cfg *SSHConfig, artifactsDir string, tmplData map[string]string) error {
	if cfg == nil {
		return fmt.Errorf("ssh configuration is required for ssh provider")
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

	// Create SSH client
	auth, err := goph.Key(cfg.KeyPath, "")
	if err != nil {
		return fmt.Errorf("failed to load SSH key: %w", err)
	}

	client, err := goph.New(cfg.User, cfg.Server, auth)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
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

	// Get current version
	version := getGitTag()

	// Read all files in artifacts directory
	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		return fmt.Errorf("failed to read artifacts directory: %w", err)
	}

	// Create archives for each file according to configuration
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Parse filename to get platform information
		fileName := file.Name()
		parts := strings.Split(fileName, "_")
		if len(parts) < 3 {
			continue
		}

		binary := parts[0]
		os := parts[1]
		arch := parts[2]

		// Template data
		tmplData := ArchiveTemplateData{
			Binary:  binary,
			Version: version,
			Os:      os,
			Arch:    arch,
		}

		// For each archive configuration
		for _, archive := range cfg.Archives {
			// Create archive name from template
			tmpl, err := template.New("archive").Parse(archive.NameTemplate)
			if err != nil {
				return fmt.Errorf("failed to parse archive name template: %w", err)
			}

			var nameBuffer strings.Builder
			if err := tmpl.Execute(&nameBuffer, tmplData); err != nil {
				return fmt.Errorf("failed to execute archive name template: %w", err)
			}

			// For each archive format
			for _, format := range archive.Formats {
				archiveName := nameBuffer.String() + "." + format
				archivePath := filepath.Join(artifactsDir, archiveName)

				switch format {
				case "tar.gz":
					if err := createTarGz(filepath.Join(artifactsDir, fileName), archivePath); err != nil {
						return fmt.Errorf("failed to create tar.gz archive: %w", err)
					}
				// Here you can add support for other archive formats
				default:
					log.Printf("Unsupported archive format: %s", format)
				}
			}
		}
	}

	return nil
}

// createTarGz creates a tar.gz archive from a file
func createTarGz(srcFile, destFile string) error {
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

	// Open source file
	file, err := os.Open(srcFile)
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
		Name:    filepath.Base(srcFile),
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

	// Create SSH client
	auth, err := goph.Key(cfg.KeyPath, "")
	if err != nil {
		return fmt.Errorf("failed to load SSH key: %w", err)
	}

	client, err := goph.New(cfg.User, cfg.Server, auth)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
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

func main() {
	// Load environment variables from .env file, if it exists.
	godotenv.Load()

	app := &cli.App{
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
				Action: func(c *cli.Context) error {
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
				},
				Action: func(c *cli.Context) error {
					configPath := c.String("config")
					cfg, err := loadConfig(configPath)
					if err != nil {
						return fmt.Errorf("error loading configuration: %w", err)
					}
					return publishArtifacts(cfg)
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
				Action: func(c *cli.Context) error {
					configPath := c.String("config")
					cfg, err := loadConfig(configPath)
					if err != nil {
						return fmt.Errorf("error loading configuration: %w", err)
					}
					return deployArtifacts(cfg, c.String("name"))
				},
			},
			{
				Name:  "git",
				Usage: "Git related commands",
				Subcommands: []*cli.Command{
					{
						Name:  "version",
						Usage: "Displays the current git tag version",
						Action: func(c *cli.Context) error {
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
				Action: func(c *cli.Context) error {
					fmt.Printf("gcx version: %s\ncommit: %s\ndate: %s\n", version, commit, date)
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
