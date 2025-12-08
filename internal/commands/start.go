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
	// Display the instant-bosh logo
	if err := PrintLogo(); err != nil {
		logger.Debug("startCommand", "Failed to print logo: %v", err)
		// Continue even if logo fails to print
	}

	ctx := context.Background()

	dockerClient, err := docker.NewClient(logger, customImage)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}
	
	// If container is running, check if we need to upgrade to a different image
	if running {
		// Check if the running container uses a different image than what we want
		imageDifferent, err := dockerClient.IsContainerImageDifferent(ctx, docker.ContainerName)
		if err != nil {
			logger.Debug("startCommand", "Failed to check if container image is different: %v", err)
		}
		
		// Determine if we need to upgrade
		needsUpgrade := false
		upgradeReason := ""
		
		if imageDifferent && err == nil {
			needsUpgrade = true
			if customImage != "" {
				upgradeReason = fmt.Sprintf("upgrading to custom image: %s", customImage)
			} else {
				upgradeReason = "upgrading to different image version"
			}
		}
		
		// If no upgrade needed, just inform user and exit
		if !needsUpgrade {
			ui.PrintLinef("instant-bosh is already running")
			ui.PrintLinef("")
			ui.PrintLinef("To configure your BOSH CLI environment, run:")
			ui.PrintLinef("  eval \"$(ibosh print-env)\"")
			return nil
		}
		
		// Get current container's image ID for comparison
		currentImageID, err := dockerClient.GetContainerImageID(ctx, docker.ContainerName)
		if err != nil {
			return fmt.Errorf("failed to get current container image: %w", err)
		}
		
		// Pull the new image if needed (to ensure it's available for diff comparison)
		targetImageExists, err := dockerClient.ImageExists(ctx)
		if err != nil {
			return fmt.Errorf("failed to check if target image exists: %w", err)
		}
		
		if !targetImageExists {
			ui.PrintLinef("Pulling new image %s...", dockerClient.GetImageName())
			if err := dockerClient.PullImage(ctx); err != nil {
				return fmt.Errorf("failed to pull new image: %w", err)
			}
		}
		
		// Show manifest diff between current and new images
		ui.PrintLinef("")
		ui.PrintLinef("Comparing BOSH manifests between current and new versions...")
		diff, err := dockerClient.ShowManifestDiff(ctx, currentImageID, dockerClient.GetImageName())
		if err != nil {
			logger.Debug("startCommand", "Failed to show manifest diff: %v", err)
			ui.PrintLinef("Warning: Could not compare manifests: %v", err)
		} else if diff != "" {
			ui.PrintLinef("")
			ui.PrintLinef("=== BOSH Manifest Changes ===")
			ui.PrintLinef(diff)
			ui.PrintLinef("=============================")
		} else {
			ui.PrintLinef("No differences in BOSH manifest")
		}
		
		// Ask for user confirmation
		ui.PrintLinef("")
		ui.PrintLinef("instant-bosh is currently running with a different image.")
		ui.PrintLinef("Upgrade will stop the current container and start a new one with the updated image.")
		ui.PrintLinef("All data in volumes will be preserved.")
		ui.PrintLinef("")
		ui.PrintLinef("Continue with upgrade?")
		
		err = ui.AskForConfirmation()
		if err != nil {
			ui.PrintLinef("Upgrade cancelled")
			return nil
		}
		
		// Proceed with upgrade: stop and remove the running container
		ui.PrintLinef("")
		ui.PrintLinef("Stopping current container...")
		if err := dockerClient.StopContainer(ctx); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}
		
		ui.PrintLinef("Removing old container (%s)...", upgradeReason)
		if err := dockerClient.RemoveContainer(ctx, docker.ContainerName); err != nil {
			return fmt.Errorf("failed to remove old container: %w", err)
		}
		
		ui.PrintLinef("Upgrading to new image...")
		// Continue with normal container creation flow below
	}

	// Check if a stopped container exists
	containerExists, err := dockerClient.ContainerExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if container exists: %w", err)
	}

	imageExists, err := dockerClient.ImageExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if image exists: %w", err)
	}
	
	// Track whether we need to recreate the container
	var needsRecreation bool
	var recreationReason string
	
	// Check for updates if skip-update flag is not set, image exists, and no custom image is specified
	var updateAvailable bool
	var updateCheckSucceeded bool
	usingCustomImage := customImage != ""
	
	if !skipUpdate && imageExists && !usingCustomImage {
		ui.PrintLinef("Checking for image updates...")
		updateAvailable, err = dockerClient.CheckForImageUpdate(ctx)
		if err != nil {
			logger.Debug("startCommand", "Failed to check for updates: %v", err)
			ui.PrintLinef("Warning: Failed to check for updates, continuing with existing image")
			updateAvailable = false
			updateCheckSucceeded = false
		} else {
			updateCheckSucceeded = true
		}
		
		if updateAvailable {
			ui.PrintLinef("New image version available! Updating...")
			
			// Get the current image ID before pulling the new one
			currentImageID, err := dockerClient.GetCurrentImageID(ctx)
			if err != nil {
				logger.Debug("startCommand", "Failed to get current image ID: %v", err)
			}
			
			// Pull the new image
			if err := dockerClient.PullImage(ctx); err != nil {
				return fmt.Errorf("failed to pull updated image: %w", err)
			}
			
			// Show manifest diff if we have the old image ID
			if currentImageID != "" {
				ui.PrintLinef("Comparing BOSH manifests between versions...")
				diff, err := dockerClient.ShowManifestDiff(ctx, currentImageID, dockerClient.GetImageName())
				if err != nil {
					logger.Debug("startCommand", "Failed to show manifest diff: %v", err)
					ui.PrintLinef("Warning: Could not compare manifests: %v", err)
				} else if diff != "" {
					ui.PrintLinef("")
					ui.PrintLinef("=== BOSH Manifest Changes ===")
					ui.PrintLinef(diff)
					ui.PrintLinef("=============================")
					ui.PrintLinef("")
				} else {
					ui.PrintLinef("No differences in BOSH manifest")
				}
			}
			
			needsRecreation = true
			recreationReason = "new image version available"
		}
	}
	
	// Check if container exists but uses a different image than what we want
	// This handles:
	// - Custom image specified that differs from existing container
	// - New image already available locally that differs from existing container
	if containerExists && imageExists && !needsRecreation {
		imageDifferent, err := dockerClient.IsContainerImageDifferent(ctx, docker.ContainerName)
		if err != nil {
			logger.Debug("startCommand", "Failed to check if container image is different: %v", err)
		} else if imageDifferent {
			needsRecreation = true
			if usingCustomImage {
				recreationReason = fmt.Sprintf("using custom image: %s", customImage)
			} else {
				recreationReason = "container using different image than desired"
			}
		}
	}
	
	// Remove the old container if it needs recreation
	if containerExists && needsRecreation {
		ui.PrintLinef("Removing old container (%s)...", recreationReason)
		if err := dockerClient.RemoveContainer(ctx, docker.ContainerName); err != nil {
			return fmt.Errorf("failed to remove old container: %w", err)
		}
	}
	
	// Scenario 1: Image does not exist locally
	if !imageExists {
		ui.PrintLinef("Image not found locally, pulling...")
		if err := dockerClient.PullImage(ctx); err != nil {
			return fmt.Errorf("failed to pull image: %w", err)
		}
	}

	// Scenario 2: Using custom image
	if imageExists && usingCustomImage {
		ui.PrintLinef("Using custom image: %s", customImage)
	}

	// Scenario 3: Image exists, but skipUpdate flag is set
	if imageExists && skipUpdate && !usingCustomImage {
		ui.PrintLinef("Skipping update check (--skip-update flag set)")
	}

	// Scenario 4: Image exists, update check performed successfully, and image is up to date
	// Don't print this message if the update check failed (updateCheckSucceeded is false)
	if imageExists && !skipUpdate && updateCheckSucceeded && !updateAvailable && !usingCustomImage {
		ui.PrintLinef("Image is up to date")
	}

	ui.PrintLinef("Creating volumes...")
	if err := dockerClient.CreateVolume(ctx, docker.VolumeStore); err != nil {
		logger.Debug("startCommand", "Volume %s may already exist: %v", docker.VolumeStore, err)
	}
	if err := dockerClient.CreateVolume(ctx, docker.VolumeData); err != nil {
		logger.Debug("startCommand", "Volume %s may already exist: %v", docker.VolumeData, err)
	}

	ui.PrintLinef("Creating network...")
	if err := dockerClient.CreateNetwork(ctx); err != nil {
		logger.Debug("startCommand", "Network may already exist: %v", err)
	}

	ui.PrintLinef("Starting instant-bosh container...")
	if err := dockerClient.StartContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Create a cancellable context for log streaming
	logCtx, cancelLogs := context.WithCancel(ctx)
	defer cancelLogs()

	// Start log streaming in a goroutine to show startup progress
	go func() {
		// We ignore errors here because cancellation will cause an expected error
		StreamMainComponentLogs(logCtx, dockerClient, ui)
	}()

	// Wait for BOSH to be ready (this is the primary readiness check)
	if err := dockerClient.WaitForBoshReady(ctx, 5*time.Minute); err != nil {
		return fmt.Errorf("BOSH failed to become ready: %w", err)
	}

	// Cancel log streaming once BOSH is ready
	cancelLogs()

	// Give the goroutine a brief moment to finish writing any buffered output
	time.Sleep(100 * time.Millisecond)

	ui.PrintLinef("instant-bosh is ready!")

	// Apply cloud-config
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
