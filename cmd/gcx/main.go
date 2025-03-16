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

// ArchiveTemplateData содержит данные для шаблона имени архива
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

// loadConfig reads the YAML configuration from the specified file.
func loadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
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
			return fmt.Errorf("error executing hook '%s': %v", hook, err)
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
		return err
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
							return fmt.Errorf("build error: %v", err)
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
						return fmt.Errorf("build error: %v", err)
					}
				}
			}
		}
	}

	// Create archives after successful build
	if err := createArchives(cfg, outDir); err != nil {
		return fmt.Errorf("failed to create archives: %v", err)
	}

	// Execute after hooks
	if len(cfg.After.Hooks) > 0 {
		if err := runHooks(cfg.After.Hooks); err != nil {
			return err
		}
	}

	return nil
}

// publishArtifacts uploads artifacts (from the output directory) to S3 according to the configuration.
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
				return fmt.Errorf("s3 publish error: %v", err)
			}
		case "ssh":
			if err := publishToSSH(blob.ToSSHConfig(), artifactsDir, tmplData); err != nil {
				return fmt.Errorf("ssh publish error: %v", err)
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
		return fmt.Errorf("error parsing directory template: %v", err)
	}
	var dirBuffer strings.Builder
	if err = tmpl.Execute(&dirBuffer, tmplData); err != nil {
		return fmt.Errorf("error executing directory template: %v", err)
	}
	remoteDir := dirBuffer.String()

	// Parse endpoint to extract host
	urlData, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return fmt.Errorf("error parsing endpoint: %v", err)
	}

	// Create an S3 client using minio-go
	s3Client, err := minio.New(urlData.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: strings.HasPrefix(cfg.Endpoint, "https"),
		Region: cfg.Region,
	})
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %v", err)
	}

	// Check if the bucket exists and create it if necessary
	ctx := context.Background()
	exists, err := s3Client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return fmt.Errorf("bucket check error: %v", err)
	}
	if !exists {
		log.Printf("Bucket %s does not exist, creating...", cfg.Bucket)
		if err = s3Client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return fmt.Errorf("failed to create bucket: %v", err)
		}
	}

	// Upload all files from the artifacts directory
	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", artifactsDir, err)
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
			return fmt.Errorf("failed to open file %s: %v", localFilePath, err)
		}
		stat, err := f.Stat()
		if err != nil {
			f.Close()
			return fmt.Errorf("failed to get file info for %s: %v", localFilePath, err)
		}
		_, err = s3Client.PutObject(ctx, cfg.Bucket, remotePath, f, stat.Size(), minio.PutObjectOptions{})
		f.Close()
		if err != nil {
			return fmt.Errorf("failed to upload file %s: %v", localFilePath, err)
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
		return fmt.Errorf("error parsing directory template: %v", err)
	}
	var dirBuffer strings.Builder
	if err = tmpl.Execute(&dirBuffer, tmplData); err != nil {
		return fmt.Errorf("error executing directory template: %v", err)
	}
	remoteDir := dirBuffer.String()

	// Create SSH client
	auth, err := goph.Key(cfg.KeyPath, "")
	if err != nil {
		return fmt.Errorf("failed to load SSH key: %v", err)
	}

	client, err := goph.New(cfg.User, cfg.Server, auth)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %v", err)
	}
	defer client.Close()

	// Create remote directory if it doesn't exist
	if _, err := client.Run(fmt.Sprintf("mkdir -p %s", remoteDir)); err != nil {
		return fmt.Errorf("failed to create remote directory: %v", err)
	}

	// Upload all files from the artifacts directory
	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", artifactsDir, err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		localFilePath := filepath.Join(artifactsDir, file.Name())
		remotePath := filepath.Join(remoteDir, file.Name())
		log.Printf("Uploading %s to %s:%s", localFilePath, cfg.Server, remotePath)

		if err := client.Upload(localFilePath, remotePath); err != nil {
			return fmt.Errorf("failed to upload file %s: %v", localFilePath, err)
		}
	}

	return nil
}

// createArchives создает архивы для всех собранных бинарных файлов
func createArchives(cfg *Config, artifactsDir string) error {
	if len(cfg.Archives) == 0 {
		return nil
	}

	// Получаем текущую версию
	version := getGitTag()

	// Читаем все файлы в директории артефактов
	files, err := os.ReadDir(artifactsDir)
	if err != nil {
		return fmt.Errorf("failed to read artifacts directory: %v", err)
	}

	// Для каждого файла создаем архивы согласно конфигурации
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Парсим имя файла для получения информации о платформе
		fileName := file.Name()
		parts := strings.Split(fileName, "_")
		if len(parts) < 3 {
			continue
		}

		binary := parts[0]
		os := parts[1]
		arch := parts[2]

		// Данные для шаблона
		tmplData := ArchiveTemplateData{
			Binary:  binary,
			Version: version,
			Os:      os,
			Arch:    arch,
		}

		// Для каждой конфигурации архива
		for _, archive := range cfg.Archives {
			// Создаем имя архива из шаблона
			tmpl, err := template.New("archive").Parse(archive.NameTemplate)
			if err != nil {
				return fmt.Errorf("failed to parse archive name template: %v", err)
			}

			var nameBuffer strings.Builder
			if err := tmpl.Execute(&nameBuffer, tmplData); err != nil {
				return fmt.Errorf("failed to execute archive name template: %v", err)
			}

			// Для каждого формата архива
			for _, format := range archive.Formats {
				archiveName := nameBuffer.String() + "." + format
				archivePath := filepath.Join(artifactsDir, archiveName)

				switch format {
				case "tar.gz":
					if err := createTarGz(filepath.Join(artifactsDir, fileName), archivePath); err != nil {
						return fmt.Errorf("failed to create tar.gz archive: %v", err)
					}
				// Здесь можно добавить поддержку других форматов архивов
				default:
					log.Printf("Unsupported archive format: %s", format)
				}
			}
		}
	}

	return nil
}

// createTarGz создает tar.gz архив из файла
func createTarGz(srcFile, destFile string) error {
	// Создаем файл архива
	archive, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("failed to create archive file: %v", err)
	}
	defer archive.Close()

	// Создаем gzip writer
	gw := gzip.NewWriter(archive)
	defer gw.Close()

	// Создаем tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Открываем исходный файл
	file, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer file.Close()

	// Получаем информацию о файле
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	// Создаем заголовок tar
	header := &tar.Header{
		Name:    filepath.Base(srcFile),
		Size:    stat.Size(),
		Mode:    int64(stat.Mode()),
		ModTime: stat.ModTime(),
	}

	// Записываем заголовок
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %v", err)
	}

	// Копируем содержимое файла в архив
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("failed to write file to tar: %v", err)
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
						Value:   ".gcx.yaml",
					},
				},
				Action: func(c *cli.Context) error {
					configPath := c.String("config")
					cfg, err := loadConfig(configPath)
					if err != nil {
						return fmt.Errorf("error loading configuration: %v", err)
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
						Value:   ".gcx.yaml",
					},
				},
				Action: func(c *cli.Context) error {
					configPath := c.String("config")
					cfg, err := loadConfig(configPath)
					if err != nil {
						return fmt.Errorf("error loading configuration: %v", err)
					}
					return publishArtifacts(cfg)
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
