package main

import (
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

func createDockerCPI(logger boshlog.Logger, customImage string) (cpi.CPI, error) {
	dockerClient, err := docker.NewClient(logger, customImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return cpi.NewDockerCPI(dockerClient), nil
}

func createIncusCPI(logger boshlog.Logger, incusRemote, incusProject, incusNetwork, incusStoragePool, customImage string) (cpi.CPI, error) {
	incusClient, err := incus.NewClient(logger, incusRemote, incusProject, incusNetwork, incusStoragePool, customImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create incus client: %w", err)
	}
	return cpi.NewIncusCPI(incusClient), nil
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
			// Docker subcommands
			{
				Name:  "docker",
				Usage: "Docker backend commands",
				Subcommands: []*cli.Command{
					{
						Name:  "start",
						Usage: "Start instant-bosh director with Docker backend",
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
						},
						Action: func(c *cli.Context) error {
							if c.Bool("skip-update") && c.String("image") != "" {
								return cli.Exit("Error: --skip-update and --image flags are mutually exclusive", 1)
							}

							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createDockerCPI(logger, c.String("image"))
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Docker CPI: %v", err), 1)
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
						Usage: "Stop instant-bosh director (Docker)",
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createDockerCPI(logger, "")
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Docker CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							return commands.StopAction(ui, logger, cpiInstance)
						},
					},
					{
						Name:  "destroy",
						Usage: "Destroy instant-bosh director and all data (Docker)",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "Skip confirmation prompt",
							},
						},
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createDockerCPI(logger, "")
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Docker CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							return commands.DestroyAction(ui, logger, cpiInstance, c.Bool("force"))
						},
					},
					{
						Name:  "logs",
						Usage: "Show logs from the instant-bosh container (Docker)",
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
							cpiInstance, err := createDockerCPI(logger, "")
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Docker CPI: %v", err), 1)
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
						Name:  "env",
						Usage: "Show environment info of instant-bosh including deployed releases (Docker)",
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createDockerCPI(logger, "")
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Docker CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							return commands.EnvAction(ui, logger, cpiInstance)
						},
					},
					{
						Name:  "print-env",
						Usage: "Print environment variables for BOSH CLI (Docker)",
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createDockerCPI(logger, "")
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Docker CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							return commands.PrintEnvAction(ui, logger, cpiInstance, &director.DefaultConfigProvider{})
						},
					},
					{
						Name:      "upload-stemcell",
						Usage:     "Upload a light stemcell from a container image (Docker only)",
						ArgsUsage: "<image-reference>",
						Description: `Upload a light stemcell to the BOSH director.

Examples:
  ibosh docker upload-stemcell ghcr.io/cloudfoundry/ubuntu-noble-stemcell:latest
  ibosh docker upload-stemcell ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165
  ibosh docker upload-stemcell ghcr.io/cloudfoundry/ubuntu-jammy-stemcell:1.234

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
			},
			// Incus subcommands
			{
				Name:  "incus",
				Usage: "Incus backend commands",
				Subcommands: []*cli.Command{
					{
						Name:  "start",
						Usage: "Start instant-bosh director with Incus backend",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "remote",
								Usage:   "Incus remote name (uses default remote from 'incus remote list' if not specified)",
								EnvVars: []string{"IBOSH_INCUS_REMOTE"},
							},
							&cli.StringFlag{
								Name:    "network",
								Usage:   "Incus network name for VM connectivity (default: ibosh)",
								Value:   "",
								EnvVars: []string{"IBOSH_INCUS_NETWORK"},
							},
							&cli.StringFlag{
								Name:    "storage-pool",
								Usage:   "Incus storage pool name",
								Value:   "local",
								EnvVars: []string{"IBOSH_INCUS_STORAGE_POOL"},
							},
							&cli.StringFlag{
								Name:    "project",
								Usage:   "Incus project name",
								Value:   "ibosh",
								EnvVars: []string{"IBOSH_INCUS_PROJECT"},
							},
							&cli.StringFlag{
								Name:  "image",
								Usage: "Custom image to use",
								Value: "",
							},
						},
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createIncusCPI(
								logger,
								c.String("remote"),
								c.String("project"),
								c.String("network"),
								c.String("storage-pool"),
								c.String("image"),
							)
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Incus CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							opts := commands.StartOptions{
								SkipUpdate:         false, // Incus doesn't support skip-update
								SkipStemcellUpload: true,  // Incus doesn't use stemcell upload yet
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
						Usage: "Stop instant-bosh director (Incus)",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "remote",
								Usage:   "Incus remote name (uses default remote from 'incus remote list' if not specified)",
								EnvVars: []string{"IBOSH_INCUS_REMOTE"},
							},
							&cli.StringFlag{
								Name:    "project",
								Usage:   "Incus project name",
								Value:   "ibosh",
								EnvVars: []string{"IBOSH_INCUS_PROJECT"},
							},
						},
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createIncusCPI(
								logger,
								c.String("remote"),
								c.String("project"),
								"", // network not needed for stop
								"", // storage-pool not needed for stop
								"", // image not needed for stop
							)
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Incus CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							return commands.StopAction(ui, logger, cpiInstance)
						},
					},
					{
						Name:  "destroy",
						Usage: "Destroy instant-bosh director and all data (Incus)",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "Skip confirmation prompt",
							},
							&cli.StringFlag{
								Name:    "remote",
								Usage:   "Incus remote name (uses default remote from 'incus remote list' if not specified)",
								EnvVars: []string{"IBOSH_INCUS_REMOTE"},
							},
							&cli.StringFlag{
								Name:    "project",
								Usage:   "Incus project name",
								Value:   "ibosh",
								EnvVars: []string{"IBOSH_INCUS_PROJECT"},
							},
						},
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createIncusCPI(
								logger,
								c.String("remote"),
								c.String("project"),
								"", // network not needed for destroy
								"", // storage-pool not needed for destroy
								"", // image not needed for destroy
							)
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Incus CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							return commands.DestroyAction(ui, logger, cpiInstance, c.Bool("force"))
						},
					},
					{
						Name:  "env",
						Usage: "Show environment info of instant-bosh including deployed releases (Incus)",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "remote",
								Usage:   "Incus remote name (uses default remote from 'incus remote list' if not specified)",
								EnvVars: []string{"IBOSH_INCUS_REMOTE"},
							},
							&cli.StringFlag{
								Name:    "project",
								Usage:   "Incus project name",
								Value:   "ibosh",
								EnvVars: []string{"IBOSH_INCUS_PROJECT"},
							},
						},
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createIncusCPI(
								logger,
								c.String("remote"),
								c.String("project"),
								"", // network not needed for env
								"", // storage-pool not needed for env
								"", // image not needed for env
							)
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Incus CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							return commands.EnvAction(ui, logger, cpiInstance)
						},
					},
					{
						Name:  "print-env",
						Usage: "Print environment variables for BOSH CLI (Incus)",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "remote",
								Usage:   "Incus remote name (uses default remote from 'incus remote list' if not specified)",
								EnvVars: []string{"IBOSH_INCUS_REMOTE"},
							},
							&cli.StringFlag{
								Name:    "project",
								Usage:   "Incus project name",
								Value:   "ibosh",
								EnvVars: []string{"IBOSH_INCUS_PROJECT"},
							},
						},
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createIncusCPI(
								logger,
								c.String("remote"),
								c.String("project"),
								"", // network not needed for print-env
								"", // storage-pool not needed for print-env
								"", // image not needed for print-env
							)
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Incus CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							return commands.PrintEnvAction(ui, logger, cpiInstance, &director.DefaultConfigProvider{})
						},
					},
					{
						Name:  "logs",
						Usage: "Show logs from the instant-bosh container (Incus)",
						Flags: []cli.Flag{
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
							&cli.StringFlag{
								Name:    "remote",
								Usage:   "Incus remote name (uses default remote from 'incus remote list' if not specified)",
								EnvVars: []string{"IBOSH_INCUS_REMOTE"},
							},
							&cli.StringFlag{
								Name:    "project",
								Usage:   "Incus project name",
								Value:   "ibosh",
								EnvVars: []string{"IBOSH_INCUS_PROJECT"},
							},
						},
						Action: func(c *cli.Context) error {
							ui, logger := initUIAndLogger(c)
							cpiInstance, err := createIncusCPI(
								logger,
								c.String("remote"),
								c.String("project"),
								"", // network not needed for logs
								"", // storage-pool not needed for logs
								"", // image not needed for logs
							)
							if err != nil {
								return cli.Exit(fmt.Sprintf("Error creating Incus CPI: %v", err), 1)
							}
							defer cpiInstance.Close()

							return commands.LogsAction(ui, logger, cpiInstance, false, []string{}, c.Bool("follow"), c.String("tail"))
						},
					},
					{
						Name:      "upload-stemcell",
						Usage:     "Upload a stemcell from bosh.io for Incus CPI",
						ArgsUsage: "<os> [version]",
						Description: `Upload a stemcell to the BOSH director from bosh.io.

The Incus CPI uses OpenStack stemcells from bosh.io.

Arguments:
  os       - The OS name (e.g., ubuntu-jammy, ubuntu-noble)
  version  - The stemcell version (default: latest)

Examples:
  ibosh incus upload-stemcell ubuntu-jammy          # Upload latest ubuntu-jammy
  ibosh incus upload-stemcell ubuntu-jammy 1.542    # Upload specific version
  ibosh incus upload-stemcell ubuntu-noble latest   # Upload latest ubuntu-noble

The command will:
  1. Query bosh.io API to resolve the stemcell URL
  2. Tell the BOSH director to download and upload the stemcell
  3. Show progress (stemcells are ~500MB, so this may take a few minutes)`,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "remote",
								Usage:   "Incus remote name (uses default remote from 'incus remote list' if not specified)",
								EnvVars: []string{"IBOSH_INCUS_REMOTE"},
							},
							&cli.StringFlag{
								Name:    "project",
								Usage:   "Incus project name",
								Value:   "ibosh",
								EnvVars: []string{"IBOSH_INCUS_PROJECT"},
							},
						},
						Action: func(c *cli.Context) error {
							if c.NArg() < 1 {
								return cli.Exit("Error: OS name required (e.g., ubuntu-jammy)", 1)
							}
							osName := c.Args().First()
							version := "latest"
							if c.NArg() >= 2 {
								version = c.Args().Get(1)
							}
							ui, logger := initUIAndLogger(c)
							return commands.UploadIncusStemcellAction(
								ui,
								logger,
								c.String("remote"),
								c.String("project"),
								osName,
								version,
							)
						},
					},
				},
			},
			// Credentials commands (requires eval "$(ibosh docker/incus print-env)")
			{
				Name:    "creds",
				Aliases: []string{"credentials"},
				Usage:   "Manage credentials in config-server",
				Description: `Interact with the config-server to manage credentials.

Requires BOSH environment to be configured first:
  eval "$(ibosh docker print-env)"   # or ibosh incus print-env

Examples:
  ibosh creds find                    # List all credentials
  ibosh creds find --path /cf         # List credentials under /cf
  ibosh creds get /cf/admin_password  # Get a specific credential
  ibosh creds delete /cf/cc_public_tls  # Delete a credential`,
				Subcommands: []*cli.Command{
					{
						Name:      "get",
						Usage:     "Get a credential by name",
						ArgsUsage: "<name>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "json",
								Aliases: []string{"j"},
								Usage:   "Output as JSON",
							},
						},
						Action: func(c *cli.Context) error {
							if c.NArg() < 1 {
								return cli.Exit("Error: credential name required", 1)
							}
							ui, _ := initUIAndLogger(c)
							return commands.CredsGetAction(ui, c.Args().First(), c.Bool("json"))
						},
					},
					{
						Name:  "find",
						Usage: "List credentials",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "path",
								Aliases: []string{"p"},
								Usage:   "Filter by path prefix (e.g., /cf)",
							},
						},
						Action: func(c *cli.Context) error {
							ui, _ := initUIAndLogger(c)
							return commands.CredsFindAction(ui, c.String("path"))
						},
					},
					{
						Name:      "delete",
						Usage:     "Delete a credential by name",
						ArgsUsage: "<name>",
						Action: func(c *cli.Context) error {
							if c.NArg() < 1 {
								return cli.Exit("Error: credential name required", 1)
							}
							ui, _ := initUIAndLogger(c)
							return commands.CredsDeleteAction(ui, c.Args().First())
						},
					},
				},
			},
			// CF commands (requires eval "$(ibosh docker/incus print-env)")
			{
				Name:  "cf",
				Usage: "Deploy and manage Cloud Foundry",
				Description: `Deploy and manage Cloud Foundry on instant-bosh.

Requires BOSH environment to be configured first:
  eval "$(ibosh docker print-env)"   # or ibosh incus print-env

Examples:
  ibosh cf deploy                     # Deploy CF (auto-selects router IP)
  ibosh cf deploy --router-ip 10.245.0.34  # Deploy with specific router IP
  ibosh cf delete                     # Delete CF deployment
  ibosh cf print-env                  # Print CF CLI env vars
  ibosh cf login                      # Login to CF as admin`,
				Subcommands: []*cli.Command{
					{
						Name:  "deploy",
						Usage: "Deploy Cloud Foundry",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "router-ip",
								Usage: "Static IP for the router (auto-selected from cloud-config if not specified)",
							},
							&cli.StringFlag{
								Name:  "system-domain",
								Usage: "System domain for CF (defaults to <router-ip>.sslip.io)",
							},
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be deployed without deploying",
							},
							&cli.BoolFlag{
								Name:  "skip-stemcell-upload",
								Usage: "Skip automatic stemcell upload (use if stemcells are already uploaded)",
							},
							&cli.BoolFlag{
								Name:  "delete-creds",
								Usage: "Delete all deployment credentials before deploying (forces regeneration)",
							},
						},
						Action: func(c *cli.Context) error {
							ui, _ := initUIAndLogger(c)
							opts := commands.CFDeployOptions{
								RouterIP:           c.String("router-ip"),
								SystemDomain:       c.String("system-domain"),
								DryRun:             c.Bool("dry-run"),
								SkipStemcellUpload: c.Bool("skip-stemcell-upload"),
								DeleteCreds:        c.Bool("delete-creds"),
							}
							return commands.CFDeployAction(ui, opts)
						},
					},
					{
						Name:  "delete",
						Usage: "Delete CF deployment",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "Skip confirmation prompt",
							},
						},
						Action: func(c *cli.Context) error {
							ui, _ := initUIAndLogger(c)
							return commands.CFDeleteAction(ui, c.Bool("force"))
						},
					},
					{
						Name:  "print-env",
						Usage: "Print CF CLI environment variables",
						Action: func(c *cli.Context) error {
							ui, _ := initUIAndLogger(c)
							return commands.CFPrintEnvAction(ui)
						},
					},
					{
						Name:  "login",
						Usage: "Login to CF as admin",
						Action: func(c *cli.Context) error {
							ui, _ := initUIAndLogger(c)
							return commands.CFLoginAction(ui)
						},
					},
				},
			},
			// Deprecated commands with helpful error messages
			{
				Name:  "start",
				Usage: "DEPRECATED: Use 'ibosh docker start' or 'ibosh incus start'",
				Action: func(c *cli.Context) error {
					return cli.Exit(`Error: Command structure has changed. Please use:
  ibosh docker start    - Start with Docker backend
  ibosh incus start     - Start with Incus backend

Run 'ibosh --help' for more information.`, 1)
				},
				Hidden: true,
			},
			{
				Name:  "stop",
				Usage: "DEPRECATED: Use 'ibosh docker stop' or 'ibosh incus stop'",
				Action: func(c *cli.Context) error {
					return cli.Exit(`Error: Command structure has changed. Please use:
  ibosh docker stop    - Stop Docker backend
  ibosh incus stop     - Stop Incus backend

Run 'ibosh --help' for more information.`, 1)
				},
				Hidden: true,
			},
			{
				Name:  "destroy",
				Usage: "DEPRECATED: Use 'ibosh docker destroy' or 'ibosh incus destroy'",
				Action: func(c *cli.Context) error {
					return cli.Exit(`Error: Command structure has changed. Please use:
  ibosh docker destroy    - Destroy Docker backend
  ibosh incus destroy     - Destroy Incus backend

Run 'ibosh --help' for more information.`, 1)
				},
				Hidden: true,
			},
			{
				Name:  "env",
				Usage: "DEPRECATED: Use 'ibosh docker env' or 'ibosh incus env'",
				Action: func(c *cli.Context) error {
					return cli.Exit(`Error: Command structure has changed. Please use:
  ibosh docker env    - Show Docker backend environment
  ibosh incus env     - Show Incus backend environment

Run 'ibosh --help' for more information.`, 1)
				},
				Hidden: true,
			},
			{
				Name:  "print-env",
				Usage: "DEPRECATED: Use 'ibosh docker print-env' or 'ibosh incus print-env'",
				Action: func(c *cli.Context) error {
					return cli.Exit(`Error: Command structure has changed. Please use:
  ibosh docker print-env    - Print Docker backend environment
  ibosh incus print-env     - Print Incus backend environment

Run 'ibosh --help' for more information.`, 1)
				},
				Hidden: true,
			},
			{
				Name:  "logs",
				Usage: "DEPRECATED: Use 'ibosh docker logs'",
				Action: func(c *cli.Context) error {
					return cli.Exit(`Error: Command structure has changed. Please use:
  ibosh docker logs    - Show Docker backend logs

Run 'ibosh --help' for more information.`, 1)
				},
				Hidden: true,
			},
			{
				Name:  "upload-stemcell",
				Usage: "DEPRECATED: Use 'ibosh docker upload-stemcell'",
				Action: func(c *cli.Context) error {
					return cli.Exit(`Error: Command structure has changed. Please use:
  ibosh docker upload-stemcell <image>    - Upload stemcell (Docker only)

Run 'ibosh --help' for more information.`, 1)
				},
				Hidden: true,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
		os.Exit(1)
	}
}
