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
					&cli.StringFlag{
						Name:  "image",
						Usage: "Custom image to use (e.g., ghcr.io/rkoster/instant-bosh:main-9e61f6f)",
						Value: "",
					},
				},
				Action: func(c *cli.Context) error {
					ui, logger := initUIAndLogger(c)
					return commands.StartAction(ui, logger, c.Bool("skip-update"), c.String("image"))
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
				Name:  "pull",
				Usage: "Pull latest instant-bosh image",
				Action: func(c *cli.Context) error {
					ui, logger := initUIAndLogger(c)
					return commands.PullAction(ui, logger)
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
		},
	}

	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
		os.Exit(1)
	}
}
