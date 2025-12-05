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

func StartAction(ui boshui.UI, logger boshlog.Logger, skipUpdate bool) error {
	// Display the instant-bosh logo
	if err := PrintLogo(); err != nil {
		logger.Debug("startCommand", "Failed to print logo: %v", err)
		// Continue even if logo fails to print
	}

	ctx := context.Background()

	dockerClient, err := docker.NewClient(logger)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}
	if running {
		ui.PrintLinef("instant-bosh is already running")
		ui.PrintLinef("")
		ui.PrintLinef("To configure your BOSH CLI environment, run:")
		ui.PrintLinef("  eval \"$(ibosh print-env)\"")
		return nil
	}

	imageExists, err := dockerClient.ImageExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if image exists: %w", err)
	}
	
	// Check for updates if skip-update flag is not set and image exists
	var updateAvailable bool
	var updateCheckSucceeded bool
	if !skipUpdate && imageExists {
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
			
			// Pull the new image
			if err := dockerClient.PullImage(ctx); err != nil {
				return fmt.Errorf("failed to pull updated image: %w", err)
			}
			
			// Check if container exists (but not running, since we checked that earlier)
			containerExists, err := dockerClient.ContainerExists(ctx)
			if err != nil {
				return fmt.Errorf("failed to check if container exists: %w", err)
			}
			
			// Remove the old container if it exists
			if containerExists {
				ui.PrintLinef("Removing old container...")
				if err := dockerClient.RemoveContainer(ctx, docker.ContainerName); err != nil {
					return fmt.Errorf("failed to remove old container: %w", err)
				}
			}
		}
	}
	
	// Scenario 1: Image does not exist locally
	if !imageExists {
		ui.PrintLinef("Image not found locally, pulling...")
		if err := dockerClient.PullImage(ctx); err != nil {
			return fmt.Errorf("failed to pull image: %w", err)
		}
	}

	// Scenario 2: Image exists, but skipUpdate flag is set
	if imageExists && skipUpdate {
		ui.PrintLinef("Skipping update check (--skip-update flag set)")
	}

	// Scenario 3: Image exists, update check performed successfully, and image is up to date
	// Don't print this message if the update check failed (updateCheckSucceeded is false)
	if imageExists && !skipUpdate && updateCheckSucceeded && !updateAvailable {
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
