package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/sxwebdev/gcx/internal/build"
	"github.com/sxwebdev/gcx/internal/config"
	"github.com/sxwebdev/gcx/internal/deploy"
	"github.com/sxwebdev/gcx/internal/git"
	"github.com/sxwebdev/gcx/internal/publish"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

var (
	version    = "dev"
	commitHash = "none"
	buildDate  = "none"
)

func main() {
	// SIGKILL removed — it cannot be caught
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer cancel()

	// Load .env file; warn if file exists but has errors
	if err := godotenv.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) && !os.IsNotExist(err) {
			log.Printf("Warning: failed to load .env file: %v", err)
		}
	}

	configFlag := &cli.StringFlag{
		Name:    "config",
		Aliases: []string{"c"},
		Usage:   "Path to the YAML configuration file",
		Value:   "gcx.yaml",
	}

	app := &cli.Command{
		Name:  "gcx",
		Usage: "A tool for cross-compiling and publishing Go binaries",
		Commands: []*cli.Command{
			{
				Name:  "build",
				Usage: "Compiles binaries",
				Flags: []cli.Flag{configFlag},
				Action: func(ctx context.Context, c *cli.Command) error {
					cfg, err := config.Load(c.String("config"))
					if err != nil {
						return err
					}
					if _, err := build.Run(ctx, cfg); err != nil {
						return err
					}
					return nil
				},
			},
			{
				Name:  "publish",
				Usage: "Publishes artifacts based on the configuration",
				Flags: []cli.Flag{
					configFlag,
					&cli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "Name of the publish configuration to execute",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					cfg, err := config.Load(c.String("config"))
					if err != nil {
						return err
					}
					return publish.Run(ctx, cfg, c.String("name"))
				},
			},
			{
				Name:  "deploy",
				Usage: "Deploys artifacts based on the configuration",
				Flags: []cli.Flag{
					configFlag,
					&cli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "Name of the deploy configuration to execute",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					cfg, err := config.Load(c.String("config"))
					if err != nil {
						return err
					}
					return deploy.Run(ctx, cfg, c.String("name"))
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
						Action: func(ctx context.Context, c *cli.Command) error {
							currentTag := git.GetTag(ctx)
							var previousTag string
							if c.Bool("stable") {
								previousTag = git.GetPreviousStableTag(ctx)
							} else {
								previousTag = git.GetPreviousTag(ctx)
							}
							changelog, err := git.GetChangelog(ctx, previousTag, currentTag)
							if err != nil {
								return fmt.Errorf("generate changelog: %w", err)
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
						Action: func(ctx context.Context, _ *cli.Command) error {
							tag := git.GetTag(ctx)
							fmt.Printf("Current git version: %s\n", tag)
							return nil
						},
					},
				},
			},
			{
				Name:  "version",
				Usage: "Displays the current version",
				Action: func(_ context.Context, _ *cli.Command) error {
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

							cfg := &config.Config{
								OutDir: "dist",
								Builds: []config.BuildConfig{
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
							encoder := yaml.NewEncoder(buf)
							defer func() { _ = encoder.Close() }()

							encoder.SetIndent(2)

							if err := encoder.Encode(cfg); err != nil {
								return fmt.Errorf("marshal config: %w", err)
							}

							if err := os.WriteFile(configPath, buf.Bytes(), 0o644); err != nil {
								return fmt.Errorf("write config file: %w", err)
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
