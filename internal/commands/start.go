package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/cpi"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/logwriter"
	"github.com/rkoster/instant-bosh/internal/stemcell"
)

// StartOptions has been moved to internal/cpi/cpi.go
// This type alias preserves backward compatibility
type StartOptions = cpi.StartOptions

func StartAction(
	ui UI,
	logger boshlog.Logger,
	cpiInstance cpi.CPI,
	configProvider director.ConfigProvider,
	directorFactory director.DirectorFactory,
	opts StartOptions,
) error {
	if err := PrintLogo(); err != nil {
		logger.Debug("startCommand", "Failed to print logo: %v", err)
	}

	ctx := context.Background()

	// TODO: Push image update/upgrade logic into CPI interface in future iteration
	// For now, we check if the CPI wraps a Docker client to access Docker-specific features
	if dockerClient, ok := unwrapDockerClient(cpiInstance); ok {
		if err := handleDockerImageManagement(ctx, ui, logger, dockerClient, opts); err != nil {
			return err
		}
	}

	if err := cpiInstance.EnsurePrerequisites(ctx); err != nil {
		return fmt.Errorf("failed to ensure prerequisites: %w", err)
	}

	running, err := cpiInstance.IsRunning(ctx)
	if err != nil {
		return err
	}

	if running {
		ui.PrintLinef("instant-bosh is already running")
		printEnvInstructions(ui, cpiInstance)
		return nil
	}

	exists, err := cpiInstance.Exists(ctx)
	if err != nil {
		return err
	}

	if exists {
		ui.PrintLinef("Removing stopped container...")
		if err := cpiInstance.Destroy(ctx); err != nil {
			return fmt.Errorf("failed to remove stopped container: %w", err)
		}
	}

	ui.PrintLinef("Starting instant-bosh container...")
	if err := cpiInstance.Start(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Follow logs during startup to show progress and capture for error reporting
	logCtx, cancelLogs := context.WithCancel(ctx)
	defer cancelLogs()

	// Create a buffer to capture logs for error reporting (last 100 lines)
	logBuffer := logwriter.NewLogBuffer(100)

	// Show only main component logs during startup for clean progress output
	// Note: All logs are still captured in logBuffer for error reporting
	uiConfig := logwriter.Config{
		MessageOnly: true,
		Components:  []string{"main"},
	}
	uiLogWriter := logwriter.New(&uiWriter{ui: ui}, uiConfig)

	// MultiWriter to write to both UI (filtered) and buffer (all logs)
	multiWriter := io.MultiWriter(uiLogWriter, logBuffer)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Allow tests to catch panics properly
				panic(r)
			}
		}()
		// Use "all" to show any available history plus new logs as they stream
		cpiInstance.FollowLogsWithOptions(logCtx, true, "all", multiWriter, multiWriter)
	}()

	ui.PrintLinef("Waiting for BOSH to be ready...")
	if err := cpiInstance.WaitForReady(ctx, 5*time.Minute); err != nil {
		cancelLogs()
		time.Sleep(100 * time.Millisecond) // Give goroutine time to finish

		// Print buffered logs on failure with full formatting
		ui.PrintLinef("")
		ui.PrintLinef("--- Container logs (last %d lines) ---", logBuffer.Len())
		colorize := isTerminal(os.Stdout.Fd())
		for _, line := range logBuffer.FormattedLines(colorize) {
			ui.PrintLinef("%s", line)
		}
		ui.PrintLinef("--- End of container logs ---")
		ui.PrintLinef("")

		return fmt.Errorf("BOSH failed to become ready: %w", err)
	}

	// Stop log following
	cancelLogs()
	time.Sleep(100 * time.Millisecond) // Give goroutine time to finish

	ui.PrintLinef("instant-bosh is ready!")

	ui.PrintLinef("Applying cloud-config...")
	if err := applyCloudConfig(ctx, cpiInstance, logger, configProvider, directorFactory); err != nil {
		return fmt.Errorf("failed to apply cloud-config: %w", err)
	}

	if !opts.SkipStemcellUpload {
		ui.PrintLinef("Uploading light stemcells...")
		if dockerClient, ok := unwrapDockerClient(cpiInstance); ok {
			if err := uploadLightStemcells(ctx, dockerClient, ui, logger, configProvider, directorFactory); err != nil {
				ui.PrintLinef("Warning: Failed to upload light stemcells: %v", err)
				ui.PrintLinef("You can manually upload stemcells with: ibosh upload-stemcell <image-ref>")
			}
		} else {
			ui.PrintLinef("Note: Stemcell upload not yet implemented for this CPI")
		}
	}

	ui.PrintLinef("")
	printEnvInstructions(ui, cpiInstance)

	return nil
}

func printEnvInstructions(ui UI, cpiInstance cpi.CPI) {
	ui.PrintLinef("To configure your BOSH CLI environment, run:")
	prefix := "ibosh"
	if _, ok := cpiInstance.(*cpi.DockerCPI); ok {
		prefix = "ibosh docker"
	} else if _, ok := cpiInstance.(*cpi.IncusCPI); ok {
		prefix = "ibosh incus"
	}
	ui.PrintLinef("  eval \"$(%s print-env)\"", prefix)
}

func unwrapDockerClient(cpiInstance cpi.CPI) (*docker.Client, bool) {
	if dockerCPI, ok := cpiInstance.(*cpi.DockerCPI); ok {
		return dockerCPI.GetDockerClient(), true
	}
	return nil, false
}

func handleDockerImageManagement(
	ctx context.Context,
	ui UI,
	logger boshlog.Logger,
	dockerClient *docker.Client,
	opts StartOptions,
) error {
	targetImage := docker.ImageName
	if opts.CustomImage != "" {
		targetImage = opts.CustomImage
	}

	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}

	if running {
		imageDifferent, err := dockerClient.IsContainerImageDifferent(ctx, docker.ContainerName)
		if err != nil {
			return fmt.Errorf("checking if container image is different: %w", err)
		}

		if imageDifferent {
			if opts.SkipUpdate {
				return nil
			}

			ui.PrintLinef("Checking for image updates for %s...", targetImage)

			currentImageName, err := dockerClient.GetContainerImageName(ctx, docker.ContainerName)
			if err != nil {
				return fmt.Errorf("getting current container image: %w", err)
			}

			targetImageExists, err := dockerClient.ImageExists(ctx)
			if err != nil {
				return fmt.Errorf("checking if target image exists: %w", err)
			}

			if !targetImageExists {
				ui.PrintLinef("Pulling new image %s...", targetImage)
				if err := dockerClient.PullImage(ctx); err != nil {
					return fmt.Errorf("pulling new image: %w", err)
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

			if err := ui.AskForConfirmation(); err != nil {
				ui.PrintLinef("Upgrade cancelled. No changes were made to the running container.")
				return nil
			}

			ui.PrintLinef("")
			ui.PrintLinef("Upgrading to new image...")

			ui.PrintLinef("Stopping and removing current container...")
			if err := dockerClient.StopContainer(ctx); err != nil {
				return fmt.Errorf("stopping container: %w", err)
			}

			maxWaitTime := 30 * time.Second
			pollInterval := 200 * time.Millisecond
			deadline := time.Now().Add(maxWaitTime)

			for time.Now().Before(deadline) {
				exists, err := dockerClient.ContainerExists(ctx)
				if err != nil {
					return fmt.Errorf("checking if container exists: %w", err)
				}
				if !exists {
					break
				}
				time.Sleep(pollInterval)
			}

			stillExists, err := dockerClient.ContainerExists(ctx)
			if err != nil {
				return fmt.Errorf("verifying container removal: %w", err)
			}
			if stillExists {
				return fmt.Errorf("container removal timed out after %v", maxWaitTime)
			}
		}

		return nil
	}

	imageExists, err := dockerClient.ImageExists(ctx)
	if err != nil {
		return fmt.Errorf("checking if image exists: %w", err)
	}

	if !imageExists {
		ui.PrintLinef("Image not found locally, pulling...")
		if err := dockerClient.PullImage(ctx); err != nil {
			return fmt.Errorf("pulling image: %w", err)
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
				return fmt.Errorf("pulling updated image: %w", err)
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

	return nil
}

func applyCloudConfig(
	ctx context.Context,
	cpiInstance cpi.CPI,
	logger boshlog.Logger,
	configProvider director.ConfigProvider,
	directorFactory director.DirectorFactory,
) error {
	config, err := configProvider.GetDirectorConfig(ctx, cpiInstance, cpiInstance.GetContainerName())
	if err != nil {
		return fmt.Errorf("getting director config: %w", err)
	}
	defer config.Cleanup()

	directorClient, err := directorFactory.NewDirector(config, logger)
	if err != nil {
		return fmt.Errorf("creating director client: %w", err)
	}

	if err := directorClient.UpdateCloudConfig("default", cpiInstance.GetCloudConfigBytes()); err != nil {
		return fmt.Errorf("updating cloud-config: %w", err)
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
