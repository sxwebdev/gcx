package build

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/sxwebdev/gcx/internal/archive"
	"github.com/sxwebdev/gcx/internal/config"
	"github.com/sxwebdev/gcx/internal/git"
	"github.com/sxwebdev/gcx/internal/hook"
	"github.com/sxwebdev/gcx/internal/tmpl"
	"golang.org/x/sync/errgroup"
)

var envVarRegex = regexp.MustCompile(`{{\.Env\.([^}]+)}}`)

// ArchiveTemplateData contains data for archive name template.
type ArchiveTemplateData struct {
	Binary  string
	Version string
	Os      string
	Arch    string
}

// Run performs cross-compilation of binaries according to the configuration.
func Run(ctx context.Context, cfg *config.Config) ([]Artifact, error) {
	// Execute before hooks
	if len(cfg.Before.Hooks) > 0 {
		if err := hook.Run(ctx, cfg.Before.Hooks); err != nil {
			return nil, err
		}
	}

	outDir := cfg.OutDir

	// Clean and recreate the output directory
	if _, err := os.Stat(outDir); err == nil {
		if err := os.RemoveAll(outDir); err != nil {
			return nil, fmt.Errorf("clean output directory: %w", err)
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	currentTag := git.GetTag(ctx)
	commitHash := git.GetCommitHash(ctx)
	buildDate := time.Now().Format(time.RFC3339)

	// Extract referenced env vars from all ldflags (compiled once, not in loop)
	envVarNames := make(map[string]bool)
	for _, buildCfg := range cfg.Builds {
		for _, ldflag := range buildCfg.Ldflags {
			matches := envVarRegex.FindAllStringSubmatch(ldflag, -1)
			for _, match := range matches {
				if len(match) > 1 {
					envVarNames[match[1]] = true
				}
			}
		}
	}

	envVars := make(map[string]string)
	for name := range envVarNames {
		if value := os.Getenv(name); value != "" {
			envVars[name] = value
		}
	}

	tmplData := struct {
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

	var allArtifacts []Artifact

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	for _, buildCfg := range cfg.Builds {
		binaryBase := buildCfg.OutputName
		if binaryBase == "" {
			parts := strings.Split(buildCfg.Main, "/")
			binaryBase = parts[len(parts)-1]
		}

		usePlatformSuffix := !buildCfg.DisablePlatformSuffix

		// Process ldflags templates
		var processedLdflags []string
		for _, ldflag := range buildCfg.Ldflags {
			result, err := tmpl.Process("ldflag", ldflag, tmplData)
			if err != nil {
				return nil, fmt.Errorf("process ldflag template %q: %w", ldflag, err)
			}
			processedLdflags = append(processedLdflags, result)
		}

		eg := errgroup.Group{}
		eg.SetLimit(concurrency)

		log.Printf("Use %d CPU cores for building...\n", concurrency)

		type buildTarget struct {
			goos, goarch, goarm string
		}

		// Collect all build targets
		var targets []buildTarget
		for _, goos := range buildCfg.Goos {
			for _, goarch := range buildCfg.Goarch {
				if goarch == "arm" && goos != "linux" {
					continue
				}
				if goarch == "arm" && len(buildCfg.Goarm) > 0 {
					for _, goarm := range buildCfg.Goarm {
						targets = append(targets, buildTarget{goos, goarch, goarm})
					}
				} else {
					targets = append(targets, buildTarget{goos, goarch, ""})
				}
			}
		}

		for _, target := range targets {
			artifact := Artifact{
				BinaryName: binaryBase,
				Version:    currentTag,
				OS:         target.goos,
				Arch:       target.goarch,
				Arm:        target.goarm,
			}
			artifact.DirPath = outputDir(usePlatformSuffix, outDir, artifact)

			allArtifacts = append(allArtifacts, artifact)

			// Capture for goroutine
			t := target
			dirPath := artifact.DirPath

			eg.Go(func() error {
				envs := os.Environ()
				envs = append(envs, "GOOS="+t.goos, "GOARCH="+t.goarch)
				if t.goarm != "" {
					envs = append(envs, "GOARM="+t.goarm)
				}
				envs = append(envs, buildCfg.Env...)

				outputName := filepath.Join(dirPath, binaryBase)

				args := []string{"build"}
				args = append(args, buildCfg.Flags...)
				if len(processedLdflags) > 0 {
					args = append(args, "-ldflags", strings.Join(processedLdflags, " "))
				}
				args = append(args, "-o", outputName, buildCfg.Main)

				if t.goarm != "" {
					log.Printf("Building %s for %s/%s arm%s...", binaryBase, t.goos, t.goarch, t.goarm)
				} else {
					log.Printf("Building %s for %s/%s...", binaryBase, t.goos, t.goarch)
				}

				cmd := exec.CommandContext(ctx, "go", args...)
				cmd.Env = envs
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("build %s/%s: %w", t.goos, t.goarch, err)
				}
				return nil
			})
		}

		if err := eg.Wait(); err != nil {
			return nil, fmt.Errorf("build error: %w", err)
		}
	}

	// Create archives
	if err := createArchives(ctx, cfg, outDir, allArtifacts); err != nil {
		return nil, fmt.Errorf("create archives: %w", err)
	}

	// Execute after hooks
	if len(cfg.After.Hooks) > 0 {
		if err := hook.Run(ctx, cfg.After.Hooks); err != nil {
			return nil, err
		}
	}

	return allArtifacts, nil
}

// outputDir returns the directory path for a built artifact.
func outputDir(usePlatformSuffix bool, outDir string, a Artifact) string {
	if usePlatformSuffix {
		name := fmt.Sprintf("%s_%s_%s_%s", a.BinaryName, a.Version, a.OS, a.Arch)
		if a.Arm != "" {
			name = fmt.Sprintf("%s_%s_%s_%s_%s", a.BinaryName, a.Version, a.OS, a.Arch, a.Arm)
		}
		return filepath.Join(outDir, name)
	}
	return filepath.Join(outDir, fmt.Sprintf("%s_%s", a.BinaryName, a.Version))
}

// createArchives creates archives for all built artifacts using structured metadata.
func createArchives(ctx context.Context, cfg *config.Config, artifactsDir string, artifacts []Artifact) error {
	if len(cfg.Archives) == 0 {
		return nil
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	eg := errgroup.Group{}
	eg.SetLimit(concurrency)

	log.Printf("Use %d CPU cores for creating archives...\n", concurrency)

	var archivedDirs []string

	for _, artifact := range artifacts {
		tmplData := ArchiveTemplateData{
			Binary:  artifact.BinaryName,
			Version: artifact.Version,
			Os:      artifact.OS,
			Arch:    artifact.Arch,
		}

		for _, archiveCfg := range cfg.Archives {
			archiveName := filepath.Base(artifact.DirPath)
			if archiveCfg.NameTemplate != "" {
				result, err := tmpl.Process("archive_name", archiveCfg.NameTemplate, tmplData)
				if err != nil {
					return fmt.Errorf("process archive name template: %w", err)
				}
				archiveName = result
			}

			for _, format := range archiveCfg.Formats {
				archiver, err := archive.New(format)
				if err != nil {
					log.Printf("Unsupported archive format: %s", format)
					continue
				}

				archiveFileName := archiveName + "." + archiver.Extension()
				archivePath := filepath.Join(artifactsDir, archiveFileName)
				sourcePath := artifact.DirPath

				archivedDirs = append(archivedDirs, artifact.DirPath)

				eg.Go(func() error {
					if err := archiver.Archive(sourcePath, archivePath); err != nil {
						return fmt.Errorf("create %s archive: %w", format, err)
					}
					return nil
				})
			}
		}
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	// Remove archived source directories
	removed := make(map[string]bool)
	for _, dir := range archivedDirs {
		if removed[dir] {
			continue
		}
		removed[dir] = true
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("Warning: failed to remove source directory %s: %v", dir, err)
		}
	}

	log.Println("All archives created successfully.")
	return nil
}
