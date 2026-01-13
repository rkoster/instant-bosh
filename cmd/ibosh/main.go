package main

import (
	"context"
	"fmt"
	"os"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/cpi"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/incus"
	"github.com/urfave/cli/v2"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func initUIAndLogger(c *cli.Context) (boshui.UI, boshlog.Logger) {
	logLevel := boshlog.LevelError
	if c.Bool("debug") {
		logLevel = boshlog.LevelDebug
	}
	logger := boshlog.NewWriterLogger(logLevel, os.Stderr)
	writerUI := boshui.NewWriterUI(os.Stdout, os.Stderr, logger)
	ui := boshui.NewColorUI(writerUI)
	return ui, logger
}

func detectAndCreateCPI(ctx context.Context, logger boshlog.Logger) (cpi.CPI, error) {
	dockerClient, err := docker.NewClient(logger, "")
	if err == nil {
		dockerCPI := cpi.NewDockerCPI(dockerClient)
		if running, _ := dockerCPI.IsRunning(ctx); running {
			return dockerCPI, nil
		}
		dockerClient.Close()
	}

	incusClient, err := incus.NewClient(logger, "", "default", "", "default", "")
	if err == nil {
		incusCPI := cpi.NewIncusCPI(incusClient)
		if running, _ := incusCPI.IsRunning(ctx); running {
			return incusCPI, nil
		}
		incusClient.Close()
	}

	return nil, fmt.Errorf("no running instant-bosh director found")
}

func createCPIForStart(ctx context.Context, logger boshlog.Logger, cpiMode string, incusRemote, incusNetwork, incusStoragePool, incusProject, customImage string) (cpi.CPI, error) {
	if cpiMode == "incus" {
		incusClient, err := incus.NewClient(logger, incusRemote, incusProject, incusNetwork, incusStoragePool, customImage)
		if err != nil {
			return nil, fmt.Errorf("failed to create incus client: %w", err)
		}
		return cpi.NewIncusCPI(incusClient), nil
	}

	dockerClient, err := docker.NewClient(logger, customImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return cpi.NewDockerCPI(dockerClient), nil
}

func main() {
	app := &cli.App{
		Name:    "ibosh",
		Usage:   "instant-bosh CLI",
		Version: version,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Usage:   "Enable debug logging",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start instant-bosh director",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "skip-update",
						Usage: "Skip checking for image updates",
						Value: false,
					},
					&cli.BoolFlag{
						Name:  "skip-stemcell-upload",
						Usage: "Skip automatic stemcell upload",
						Value: false,
					},
					&cli.StringFlag{
						Name:  "image",
						Usage: "Custom image to use (e.g., ghcr.io/rkoster/instant-bosh:main-9e61f6f)",
						Value: "",
					},
					&cli.StringFlag{
						Name:    "cpi",
						Usage:   "CPI to use: 'docker' or 'incus'",
						Value:   "docker",
						EnvVars: []string{"IBOSH_CPI"},
					},
					&cli.StringFlag{
						Name:    "incus-remote",
						Usage:   "Incus remote name (uses default remote from 'incus remote list' if not specified)",
						EnvVars: []string{"IBOSH_INCUS_REMOTE"},
					},
					&cli.StringFlag{
						Name:    "incus-network",
						Usage:   "Incus network name for VM connectivity (default: instant-bosh-incus)",
						Value:   "",
						EnvVars: []string{"IBOSH_INCUS_NETWORK"},
					},
					&cli.StringFlag{
						Name:    "incus-storage-pool",
						Usage:   "Incus storage pool name",
						Value:   "default",
						EnvVars: []string{"IBOSH_INCUS_STORAGE_POOL"},
					},
					&cli.StringFlag{
						Name:    "incus-project",
						Usage:   "Incus project name",
						Value:   "default",
						EnvVars: []string{"IBOSH_INCUS_PROJECT"},
					},
				},
				Action: func(c *cli.Context) error {
					if c.Bool("skip-update") && c.String("image") != "" {
						return cli.Exit("Error: --skip-update and --image flags are mutually exclusive", 1)
					}

					cpiMode := c.String("cpi")
					if cpiMode != "docker" && cpiMode != "incus" {
						return cli.Exit("Error: --cpi must be 'docker' or 'incus'", 1)
					}

					ctx := context.Background()
					ui, logger := initUIAndLogger(c)

					cpiInstance, err := createCPIForStart(
						ctx,
						logger,
						cpiMode,
						c.String("incus-remote"),
						c.String("incus-network"),
						c.String("incus-storage-pool"),
						c.String("incus-project"),
						c.String("image"),
					)
					if err != nil {
						return cli.Exit(fmt.Sprintf("Error creating CPI: %v", err), 1)
					}
					defer cpiInstance.Close()

					opts := commands.StartOptions{
						SkipUpdate:         c.Bool("skip-update"),
						SkipStemcellUpload: c.Bool("skip-stemcell-upload"),
						CustomImage:        c.String("image"),
					}

					return commands.StartAction(
						ui,
						logger,
						cpiInstance,
						&director.DefaultConfigProvider{},
						&director.DefaultDirectorFactory{},
						opts,
					)
				},
			},
			{
				Name:  "stop",
				Usage: "Stop instant-bosh director",
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					ui, logger := initUIAndLogger(c)

					cpiInstance, err := detectAndCreateCPI(ctx, logger)
					if err != nil {
						return cli.Exit(fmt.Sprintf("Error: %v", err), 1)
					}
					defer cpiInstance.Close()

					return commands.StopAction(ui, logger, cpiInstance)
				},
			},
			{
				Name:  "destroy",
				Usage: "Destroy instant-bosh director and all data",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "force",
						Aliases: []string{"f"},
						Usage:   "Skip confirmation prompt",
					},
				},
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					ui, logger := initUIAndLogger(c)

					cpiInstance, err := detectAndCreateCPI(ctx, logger)
					if err != nil {
						return cli.Exit(fmt.Sprintf("Error: %v", err), 1)
					}
					defer cpiInstance.Close()

					return commands.DestroyAction(ui, logger, cpiInstance, c.Bool("force"))
				},
			},
			{
				Name:  "env",
				Usage: "Show environment info of instant-bosh including deployed releases",
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					ui, logger := initUIAndLogger(c)

					cpiInstance, err := detectAndCreateCPI(ctx, logger)
					if err != nil {
						return cli.Exit(fmt.Sprintf("Error: %v", err), 1)
					}
					defer cpiInstance.Close()

					return commands.EnvAction(ui, logger, cpiInstance)
				},
			},
			{
				Name:  "print-env",
				Usage: "Print environment variables for BOSH CLI",
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					ui, logger := initUIAndLogger(c)

					cpiInstance, err := detectAndCreateCPI(ctx, logger)
					if err != nil {
						return cli.Exit(fmt.Sprintf("Error: %v", err), 1)
					}
					defer cpiInstance.Close()

					return commands.PrintEnvAction(ui, logger, cpiInstance, &director.DefaultConfigProvider{})
				},
			},
			{
				Name:  "logs",
				Usage: "Show logs from the instant-bosh container",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "list-components",
						Usage: "List all available log components",
					},
					&cli.StringSliceFlag{
						Name:    "component",
						Aliases: []string{"c"},
						Usage:   "Filter logs by component (can be specified multiple times)",
					},
					&cli.BoolFlag{
						Name:    "follow",
						Aliases: []string{"f"},
						Usage:   "Follow log output",
						Value:   false,
					},
					&cli.StringFlag{
						Name:    "tail",
						Aliases: []string{"n"},
						Usage:   "Number of lines to show from the end of the logs",
						Value:   "all",
					},
				},
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					ui, logger := initUIAndLogger(c)

					cpiInstance, err := detectAndCreateCPI(ctx, logger)
					if err != nil {
						return cli.Exit(fmt.Sprintf("Error: %v", err), 1)
					}
					defer cpiInstance.Close()

					return commands.LogsAction(
						ui,
						logger,
						cpiInstance,
						c.Bool("list-components"),
						c.StringSlice("component"),
						c.Bool("follow"),
						c.String("tail"),
					)
				},
			},
			{
				Name:      "upload-stemcell",
				Usage:     "Upload a light stemcell from a container image",
				ArgsUsage: "<image-reference>",
				Description: `Upload a light stemcell to the BOSH director.

Examples:
  ibosh upload-stemcell ghcr.io/cloudfoundry/ubuntu-noble-stemcell:latest
  ibosh upload-stemcell ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165
  ibosh upload-stemcell ghcr.io/cloudfoundry/ubuntu-jammy-stemcell:1.234

The command will:
  1. Resolve 'latest' tags to actual version numbers
  2. Get the image digest for verification
  3. Create a light stemcell tarball
  4. Upload it to the BOSH director (if not already present)

Works offline if the image is already pulled locally.`,
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						return cli.Exit("Error: image reference required", 1)
					}
					imageRef := c.Args().First()
					ui, logger := initUIAndLogger(c)
					return commands.UploadStemcellAction(ui, logger, imageRef)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
		os.Exit(1)
	}
}
