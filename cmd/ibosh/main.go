package main

import (
	"os"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/commands"
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
						Name:    "incus",
						Usage:   "Use Incus mode with specified remote (empty or 'local' for local socket)",
						EnvVars: []string{"IBOSH_INCUS_REMOTE"},
					},
					&cli.StringFlag{
						Name:    "incus-network",
						Usage:   "Incus network name for VM connectivity",
						Value:   "incusbr0",
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

					ui, logger := initUIAndLogger(c)
					return commands.StartAction(ui, logger, c.Bool("skip-update"), c.Bool("skip-stemcell-upload"), c.String("image"))
				},
			},
			{
				Name:  "stop",
				Usage: "Stop instant-bosh director",
				Action: func(c *cli.Context) error {
					ui, logger := initUIAndLogger(c)
					return commands.StopAction(ui, logger)
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
					ui, logger := initUIAndLogger(c)
					return commands.DestroyAction(ui, logger, c.Bool("force"))
				},
			},
			{
				Name:  "env",
				Usage: "Show environment info of instant-bosh including deployed releases",
				Action: func(c *cli.Context) error {
					ui, logger := initUIAndLogger(c)
					return commands.EnvAction(ui, logger)
				},
			},
			{
				Name:  "print-env",
				Usage: "Print environment variables for BOSH CLI",
				Action: func(c *cli.Context) error {
					ui, logger := initUIAndLogger(c)
					return commands.PrintEnvAction(ui, logger)
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
					ui, logger := initUIAndLogger(c)
					return commands.LogsAction(
						ui,
						logger,
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
