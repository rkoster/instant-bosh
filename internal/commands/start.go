package commands

import (
	"context"
	"fmt"
	"time"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/docker"
)

func StartAction(ui boshui.UI, logger boshlog.Logger, skipUpdate bool, customImage string) error {
	if err := PrintLogo(); err != nil {
		logger.Debug("startCommand", "Failed to print logo: %v", err)
	}

	ctx := context.Background()

	// =================================================================
	// PHASE 1: IMAGE MANAGEMENT
	// =================================================================

	targetImage := docker.ImageName
	if customImage != "" {
		targetImage = customImage
	}

	dockerClient, err := docker.NewClient(logger, customImage)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	// --- Subphase 1A: Handle running container with different image ---
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
			if skipUpdate {
				ui.PrintLinef("instant-bosh is already running")
				ui.PrintLinef("")
				ui.PrintLinef("To configure your BOSH CLI environment, run:")
				ui.PrintLinef("  eval \"$(ibosh print-env)\"")
				return nil
			}

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

	// --- Subphase 1B: Image management for non-running or upgrade scenarios ---
	if !running || proceedWithUpgrade {
		imageExists, err := dockerClient.ImageExists(ctx)
		if err != nil {
			return fmt.Errorf("failed to check if image exists: %w", err)
		}

		if !imageExists {
			ui.PrintLinef("Image not found locally, pulling...")
			if err := dockerClient.PullImage(ctx); err != nil {
				return fmt.Errorf("failed to pull image: %w", err)
			}
		} else if !skipUpdate && customImage == "" && !proceedWithUpgrade {
			ui.PrintLinef("Checking for image updates...")
			updateAvailable, err := dockerClient.CheckForImageUpdate(ctx)
			if err != nil {
				logger.Debug("startCommand", "Failed to check for updates: %v", err)
				ui.PrintLinef("Warning: Failed to check for updates, continuing with existing image")
			} else if updateAvailable {
				ui.PrintLinef("New image version available! Updating...")

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
				ui.PrintLinef("Image is up to date")
			}
		} else if skipUpdate && !proceedWithUpgrade {
			ui.PrintLinef("Skipping update check (--skip-update flag set)")
		} else if customImage != "" && !proceedWithUpgrade {
			ui.PrintLinef("Using custom image: %s", customImage)
		}
	}

	// =================================================================
	// PHASE 2: CONTAINER LIFECYCLE
	// =================================================================

	return startContainer(ctx, dockerClient, ui, logger)
}

// startContainer idempotently ensures a container is running with the target image.
// If a container is already running with the target image, it does nothing.
// If a container is running with a different image, it recreates it.
// If a stopped container exists, it removes and recreates it.
func startContainer(
	ctx context.Context,
	dockerClient *docker.Client,
	ui boshui.UI,
	logger boshlog.Logger,
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
	ui.PrintLinef("Creating volumes...")
	if err := dockerClient.CreateVolume(ctx, docker.VolumeStore); err != nil {
		logger.Debug("startContainer", "Volume %s may already exist: %v", docker.VolumeStore, err)
	}
	if err := dockerClient.CreateVolume(ctx, docker.VolumeData); err != nil {
		logger.Debug("startContainer", "Volume %s may already exist: %v", docker.VolumeData, err)
	}

	ui.PrintLinef("Creating network...")
	if err := dockerClient.CreateNetwork(ctx); err != nil {
		logger.Debug("startContainer", "Network may already exist: %v", err)
	}

	ui.PrintLinef("Starting instant-bosh container...")
	if err := dockerClient.StartContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Create a cancellable context for log streaming
	logCtx, cancelLogs := context.WithCancel(ctx)
	defer cancelLogs()

	go func() {
		StreamMainComponentLogs(logCtx, dockerClient, ui)
	}()

	// Wait for BOSH to be ready
	if err := dockerClient.WaitForBoshReady(ctx, 5*time.Minute); err != nil {
		return fmt.Errorf("BOSH failed to become ready: %w", err)
	}

	cancelLogs()
	time.Sleep(100 * time.Millisecond)

	ui.PrintLinef("instant-bosh is ready!")

	ui.PrintLinef("Applying cloud-config...")
	if err := applyCloudConfig(ctx, dockerClient, logger); err != nil {
		return fmt.Errorf("failed to apply cloud-config: %w", err)
	}

	ui.PrintLinef("")
	ui.PrintLinef("To configure your BOSH CLI environment, run:")
	ui.PrintLinef("  eval \"$(ibosh print-env)\"")

	return nil
}

func applyCloudConfig(ctx context.Context, dockerClient *docker.Client, logger boshlog.Logger) error {
	// Get director configuration
	config, err := director.GetDirectorConfig(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("failed to get director config: %w", err)
	}
	defer config.Cleanup()

	// Create BOSH director client
	directorClient, err := director.NewDirector(config, logger)
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
