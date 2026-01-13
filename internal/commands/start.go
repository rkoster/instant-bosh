package commands

import (
	"context"
	"fmt"
	"time"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/container"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/incus"
	"github.com/rkoster/instant-bosh/internal/stemcell"
)

type StartOptions struct {
	SkipUpdate         bool
	SkipStemcellUpload bool
	CustomImage        string
	// CPI mode: "docker" or "incus"
	CPI string
	// Incus mode parameters
	IncusRemote      string
	IncusNetwork     string
	IncusStoragePool string
	IncusProject     string
}

func StartAction(ui boshui.UI, logger boshlog.Logger, opts StartOptions) error {
	return StartActionWithFactories(
		ui,
		logger,
		&docker.DefaultClientFactory{},
		&director.DefaultConfigProvider{},
		&director.DefaultDirectorFactory{},
		opts,
	)
}

func StartActionWithFactories(
	ui UI,
	logger boshlog.Logger,
	clientFactory docker.ClientFactory,
	configProvider director.ConfigProvider,
	directorFactory director.DirectorFactory,
	opts StartOptions,
) error {
	if err := PrintLogo(); err != nil {
		logger.Debug("startCommand", "Failed to print logo: %v", err)
	}

	ctx := context.Background()

	if opts.CPI == "incus" {
		return startIncusMode(ctx, ui, logger, configProvider, directorFactory, opts)
	}

	return startDockerMode(ctx, ui, logger, clientFactory, configProvider, directorFactory, opts)
}

// startContainer idempotently ensures a container is running with the target image.
// If a container is already running with the target image, it does nothing.
// If a container is running with a different image, it recreates it.
// If a stopped container exists, it removes and recreates it.
func startContainer(
	ctx context.Context,
	dockerClient *docker.Client,
	ui UI,
	logger boshlog.Logger,
	configProvider director.ConfigProvider,
	directorFactory director.DirectorFactory,
	skipStemcellUpload bool,
) error {
	// Check if container is running
	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}

	if running {
		// Check if running container uses target image
		imageDifferent, err := dockerClient.IsContainerImageDifferent(ctx, docker.ContainerName)
		if err != nil {
			return fmt.Errorf("failed to check if container image is different: %w", err)
		}

		if !imageDifferent {
			ui.PrintLinef("instant-bosh is already running")
			ui.PrintLinef("")
			ui.PrintLinef("To configure your BOSH CLI environment, run:")
			ui.PrintLinef("  eval \"$(ibosh print-env)\"")
			return nil
		}

		// Need to recreate - stop and remove
		ui.PrintLinef("Stopping and removing current container...")
		if err := dockerClient.StopContainer(ctx); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}

		// Wait for auto-removal
		maxWaitTime := 30 * time.Second
		pollInterval := 200 * time.Millisecond
		deadline := time.Now().Add(maxWaitTime)

		for time.Now().Before(deadline) {
			exists, err := dockerClient.ContainerExists(ctx)
			if err != nil {
				return fmt.Errorf("failed to check if container exists: %w", err)
			}
			if !exists {
				break
			}
			time.Sleep(pollInterval)
		}

		stillExists, err := dockerClient.ContainerExists(ctx)
		if err != nil {
			return fmt.Errorf("failed to verify container removal: %w", err)
		}
		if stillExists {
			return fmt.Errorf("container removal timed out after %v", maxWaitTime)
		}
	}

	// Check if stopped container exists (always remove if it does)
	containerExists, err := dockerClient.ContainerExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if container exists: %w", err)
	}

	if containerExists {
		// Get the image name of the stopped container to inform user
		imageName, err := dockerClient.GetContainerImageName(ctx, docker.ContainerName)
		if err != nil {
			logger.Debug("startContainer", "Failed to get stopped container image name: %v", err)
			ui.PrintLinef("Removing stopped container...")
		} else {
			ui.PrintLinef("Removing stopped container (was using image: %s)...", imageName)
		}

		if err := dockerClient.RemoveContainer(ctx, docker.ContainerName); err != nil {
			return fmt.Errorf("failed to remove container: %w", err)
		}
	}

	// Create and start container
	// Check if volumes exist
	storeExists, err := dockerClient.VolumeExists(ctx, docker.VolumeStore)
	if err != nil {
		return fmt.Errorf("failed to check if volume %s exists: %w", docker.VolumeStore, err)
	}
	dataExists, err := dockerClient.VolumeExists(ctx, docker.VolumeData)
	if err != nil {
		return fmt.Errorf("failed to check if volume %s exists: %w", docker.VolumeData, err)
	}

	if storeExists && dataExists {
		ui.PrintLinef("Using existing volumes...")
	} else {
		ui.PrintLinef("Creating volumes...")
		if !storeExists {
			if err := dockerClient.CreateVolume(ctx, docker.VolumeStore); err != nil {
				return fmt.Errorf("failed to create volume %s: %w", docker.VolumeStore, err)
			}
		}
		if !dataExists {
			if err := dockerClient.CreateVolume(ctx, docker.VolumeData); err != nil {
				return fmt.Errorf("failed to create volume %s: %w", docker.VolumeData, err)
			}
		}
	}

	// Check if network exists
	networkExists, err := dockerClient.NetworkExists(ctx, docker.NetworkName)
	if err != nil {
		return fmt.Errorf("failed to check if network exists: %w", err)
	}

	if networkExists {
		ui.PrintLinef("Using existing network...")
	} else {
		ui.PrintLinef("Creating network...")
		if err := dockerClient.CreateNetwork(ctx); err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
	}

	ui.PrintLinef("Starting instant-bosh container...")
	if err := dockerClient.StartContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Create a cancellable context for log streaming
	logCtx, cancelLogs := context.WithCancel(ctx)
	defer cancelLogs()

	go func() {
		StreamMainComponentLogsFromDocker(logCtx, dockerClient, ui)
	}()

	// Wait for BOSH to be ready
	if err := dockerClient.WaitForBoshReady(ctx, 5*time.Minute); err != nil {
		return fmt.Errorf("BOSH failed to become ready: %w", err)
	}

	cancelLogs()
	time.Sleep(100 * time.Millisecond)

	ui.PrintLinef("instant-bosh is ready!")

	ui.PrintLinef("Applying cloud-config...")
	if err := applyCloudConfig(ctx, dockerClient, docker.ContainerName, logger, configProvider, directorFactory); err != nil {
		return fmt.Errorf("failed to apply cloud-config: %w", err)
	}

	if !skipStemcellUpload {
		ui.PrintLinef("Uploading light stemcells...")
		if err := uploadLightStemcells(ctx, dockerClient, ui, logger, configProvider, directorFactory); err != nil {
			ui.PrintLinef("Warning: Failed to upload light stemcells: %v", err)
			ui.PrintLinef("You can manually upload stemcells with: ibosh upload-stemcell <image-ref>")
		}
	}

	ui.PrintLinef("")
	ui.PrintLinef("To configure your BOSH CLI environment, run:")
	ui.PrintLinef("  eval \"$(ibosh print-env)\"")

	return nil
}

func applyCloudConfig(
	ctx context.Context,
	containerClient container.Client,
	containerName string,
	logger boshlog.Logger,
	configProvider director.ConfigProvider,
	directorFactory director.DirectorFactory,
) error {
	config, err := configProvider.GetDirectorConfig(ctx, containerClient, containerName)
	if err != nil {
		return fmt.Errorf("failed to get director config: %w", err)
	}
	defer config.Cleanup()

	// Create BOSH director client
	directorClient, err := directorFactory.NewDirector(config, logger)
	if err != nil {
		return fmt.Errorf("failed to create director client: %w", err)
	}

	// Apply cloud-config
	if err := directorClient.UpdateCloudConfig("default", cloudConfigYAMLBytes); err != nil {
		return fmt.Errorf("failed to update cloud-config: %w", err)
	}
	logger.Debug("startCommand", "Cloud-config applied successfully")

	return nil
}

// Default stemcell images to upload automatically
var defaultStemcellImages = []string{
	"ghcr.io/cloudfoundry/ubuntu-noble-stemcell:latest",
}

// uploadLightStemcells uploads default light stemcells to the BOSH director
func uploadLightStemcells(
	ctx context.Context,
	dockerClient *docker.Client,
	ui UI,
	logger boshlog.Logger,
	configProvider director.ConfigProvider,
	directorFactory director.DirectorFactory,
) error {
	config, err := configProvider.GetDirectorConfig(ctx, dockerClient, docker.ContainerName)
	if err != nil {
		return fmt.Errorf("getting director config: %w", err)
	}
	defer config.Cleanup()

	// Create BOSH director client
	directorClient, err := directorFactory.NewDirector(config, logger)
	if err != nil {
		return fmt.Errorf("creating director client: %w", err)
	}

	// Get list of existing stemcells
	existingStemcells, err := directorClient.Stemcells()
	if err != nil {
		return fmt.Errorf("listing existing stemcells: %w", err)
	}

	// Build a map for quick lookup
	existingMap := make(map[string]bool)
	for _, s := range existingStemcells {
		key := fmt.Sprintf("%s/%s", s.Name(), s.Version().String())
		existingMap[key] = true
	}

	// Upload each default stemcell
	for _, imageRef := range defaultStemcellImages {
		uploaded, err := uploadStemcellIfNeeded(ctx, dockerClient, directorClient, ui, logger, imageRef, existingMap)
		if err != nil {
			// Log warning but continue with other stemcells
			ui.PrintLinef("  Warning: %s: %v", imageRef, err)
			continue
		}
		if uploaded {
			metadata, _ := dockerClient.GetImageMetadata(ctx, imageRef)
			if metadata != nil {
				os, _ := stemcell.ParseOSFromImageRef(metadata.Repository)
				if os != "" {
					key := fmt.Sprintf("%s/%s", stemcell.BuildStemcellName(os), metadata.Tag)
					existingMap[key] = true
				}
			}
		}
	}

	return nil
}

func startDockerMode(
	ctx context.Context,
	ui UI,
	logger boshlog.Logger,
	clientFactory docker.ClientFactory,
	configProvider director.ConfigProvider,
	directorFactory director.DirectorFactory,
	opts StartOptions,
) error {
	targetImage := docker.ImageName
	if opts.CustomImage != "" {
		targetImage = opts.CustomImage
	}

	dockerClient, err := clientFactory.NewClient(logger, opts.CustomImage)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}

	var proceedWithUpgrade bool

	if running {
		imageDifferent, err := dockerClient.IsContainerImageDifferent(ctx, docker.ContainerName)
		if err != nil {
			return fmt.Errorf("failed to check if container image is different: %w", err)
		}

		if imageDifferent {
			if opts.SkipUpdate {
				ui.PrintLinef("instant-bosh is already running")
				ui.PrintLinef("")
				ui.PrintLinef("To configure your BOSH CLI environment, run:")
				ui.PrintLinef("  eval \"$(ibosh print-env)\"")
				return nil
			}

			ui.PrintLinef("Checking for image updates for %s...", targetImage)

			currentImageName, err := dockerClient.GetContainerImageName(ctx, docker.ContainerName)
			if err != nil {
				return fmt.Errorf("failed to get current container image: %w", err)
			}

			targetImageExists, err := dockerClient.ImageExists(ctx)
			if err != nil {
				return fmt.Errorf("failed to check if target image exists: %w", err)
			}

			if !targetImageExists {
				ui.PrintLinef("Pulling new image %s...", targetImage)
				if err := dockerClient.PullImage(ctx); err != nil {
					return fmt.Errorf("failed to pull new image: %w", err)
				}
			}

			diff, err := dockerClient.ShowManifestDiff(ctx, currentImageName, targetImage)
			if err != nil {
				logger.Debug("startCommand", "Failed to show manifest diff: %v", err)
				ui.PrintLinef("Warning: Could not compare manifests: %v", err)
			} else if diff != "" {
				ui.PrintLinef("")
				ui.PrintLinef("Manifest changes:")
				ui.PrintLinef("")
				ui.PrintLinef(diff)
			} else {
				ui.PrintLinef("No differences in BOSH manifest")
			}

			ui.PrintLinef("")
			ui.PrintLinef("Continue with upgrade?")

			err = ui.AskForConfirmation()
			if err != nil {
				ui.PrintLinef("Upgrade cancelled. No changes were made to the running container.")
				return nil
			}

			ui.PrintLinef("")
			ui.PrintLinef("Upgrading to new image...")
			proceedWithUpgrade = true
		}
	}

	if !running && !proceedWithUpgrade {
		imageExists, err := dockerClient.ImageExists(ctx)
		if err != nil {
			return fmt.Errorf("failed to check if image exists: %w", err)
		}

		if !imageExists {
			ui.PrintLinef("Image not found locally, pulling...")
			if err := dockerClient.PullImage(ctx); err != nil {
				return fmt.Errorf("failed to pull image: %w", err)
			}
		} else if !opts.SkipUpdate && opts.CustomImage == "" {
			ui.PrintLinef("Checking for image updates for %s...", targetImage)
			updateAvailable, err := dockerClient.CheckForImageUpdate(ctx)
			if err != nil {
				logger.Debug("startCommand", "Failed to check for updates: %v", err)
				ui.PrintLinef("Warning: Failed to check for updates, continuing with existing image")
			} else if updateAvailable {
				ui.PrintLinef("Image %s has a newer revision available! Updating...", targetImage)

				currentImageName := dockerClient.GetImageName()

				if err := dockerClient.PullImage(ctx); err != nil {
					return fmt.Errorf("failed to pull updated image: %w", err)
				}

				diff, err := dockerClient.ShowManifestDiff(ctx, currentImageName, targetImage)
				if err != nil {
					logger.Debug("startCommand", "Failed to show manifest diff: %v", err)
					ui.PrintLinef("Warning: Could not compare manifests: %v", err)
				} else if diff != "" {
					ui.PrintLinef("")
					ui.PrintLinef("Manifest changes:")
					ui.PrintLinef("")
					ui.PrintLinef(diff)
				} else {
					ui.PrintLinef("No differences in BOSH manifest")
				}
			} else {
				ui.PrintLinef("Image %s is at the latest version", targetImage)
			}
		} else if opts.SkipUpdate {
			ui.PrintLinef("Skipping update check (--skip-update flag set)")
		} else if opts.CustomImage != "" {
			ui.PrintLinef("Using custom image: %s", opts.CustomImage)
		}
	}

	return startContainer(ctx, dockerClient, ui, logger, configProvider, directorFactory, opts.SkipStemcellUpload)
}

func startIncusMode(
	ctx context.Context,
	ui UI,
	logger boshlog.Logger,
	configProvider director.ConfigProvider,
	directorFactory director.DirectorFactory,
	opts StartOptions,
) error {
	incusFactory := &incus.DefaultClientFactory{}

	incusClient, err := incusFactory.NewClient(logger, opts.IncusRemote, opts.IncusProject, opts.IncusNetwork, opts.IncusStoragePool, opts.CustomImage)
	if err != nil {
		return fmt.Errorf("failed to create incus client: %w", err)
	}
	defer incusClient.Close()

	running, err := incusClient.IsContainerRunning(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if container is running: %w", err)
	}

	if running {
		ui.PrintLinef("instant-bosh is already running")
		ui.PrintLinef("")
		ui.PrintLinef("To configure your BOSH CLI environment, run:")
		ui.PrintLinef("  eval \"$(ibosh print-env)\"")
		return nil
	}

	exists, err := incusClient.ContainerExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if container exists: %w", err)
	}

	if exists {
		ui.PrintLinef("Removing stopped container...")
		if err := incusClient.RemoveContainer(ctx, incus.ContainerName); err != nil {
			return fmt.Errorf("failed to remove container: %w", err)
		}
	}

	networkExists, err := incusClient.NetworkExists(ctx, incusClient.NetworkName())
	if err != nil {
		return fmt.Errorf("failed to check if network exists: %w", err)
	}

	if networkExists {
		ui.PrintLinef("Using existing network...")
	} else {
		ui.PrintLinef("Creating network...")
		if err := incusClient.CreateNetwork(ctx); err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
	}

	ui.PrintLinef("Starting instant-bosh container...")
	if err := incusClient.StartContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	ui.PrintLinef("Waiting for BOSH to be ready...")
	time.Sleep(60 * time.Second)

	ui.PrintLinef("instant-bosh is ready!")

	ui.PrintLinef("Applying cloud-config...")
	config, err := configProvider.GetDirectorConfig(ctx, incusClient, incus.ContainerName)
	if err != nil {
		return fmt.Errorf("failed to get director config: %w", err)
	}
	defer config.Cleanup()

	directorClient, err := directorFactory.NewDirector(config, logger)
	if err != nil {
		return fmt.Errorf("failed to create director client: %w", err)
	}

	if err := directorClient.UpdateCloudConfig("default", incusCloudConfigYAMLBytes); err != nil {
		return fmt.Errorf("failed to update cloud-config: %w", err)
	}
	logger.Debug("startCommand", "Cloud-config applied successfully")

	if !opts.SkipStemcellUpload {
		ui.PrintLinef("Note: Stemcell upload for Incus mode not yet implemented")
	}

	ui.PrintLinef("")
	ui.PrintLinef("To configure your BOSH CLI environment, run:")
	ui.PrintLinef("  eval \"$(ibosh print-env)\"")

	return nil
}
