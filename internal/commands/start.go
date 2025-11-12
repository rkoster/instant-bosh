package commands

import (
	"context"
	"fmt"
	"time"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/docker"
	"gopkg.in/yaml.v3"
)

func StartAction(ui boshui.UI, logger boshlog.Logger) error {
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
	if !imageExists {
		ui.PrintLinef("Image not found locally, pulling...")
		if err := dockerClient.PullImage(ctx); err != nil {
			return fmt.Errorf("failed to pull image: %w", err)
		}
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

	// Apply runtime-config
	ui.PrintLinef("Applying runtime-config...")
	if err := applyRuntimeConfig(ctx, dockerClient, logger); err != nil {
		return fmt.Errorf("failed to apply runtime-config: %w", err)
	}

	ui.PrintLinef("")
	ui.PrintLinef("To configure your BOSH CLI environment, run:")
	ui.PrintLinef("  eval \"$(ibosh print-env)\"")

	return nil
}

// applyConfig is a helper function to apply either cloud-config or runtime-config
func applyConfig(ctx context.Context, dockerClient *docker.Client, logger boshlog.Logger, configType, configName string, configYAML []byte) error {
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

	// Update the appropriate config type
	switch configType {
	case "cloud":
		if err := directorClient.UpdateCloudConfig(configName, configYAML); err != nil {
			return fmt.Errorf("failed to update cloud-config: %w", err)
		}
		logger.Debug("startCommand", "Cloud-config applied successfully")
	case "runtime":
		if err := directorClient.UpdateRuntimeConfig(configName, configYAML); err != nil {
			return fmt.Errorf("failed to update runtime-config: %w", err)
		}
		logger.Debug("startCommand", "Runtime-config applied successfully")
	default:
		return fmt.Errorf("unknown config type: %s", configType)
	}

	return nil
}

func applyCloudConfig(ctx context.Context, dockerClient *docker.Client, logger boshlog.Logger) error {
	return applyConfig(ctx, dockerClient, logger, "cloud", "default", cloudConfigYAMLBytes)
}

func applyRuntimeConfig(ctx context.Context, dockerClient *docker.Client, logger boshlog.Logger) error {
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

	// Parse runtime config to extract releases
	var runtimeConfig struct {
		Releases []struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
			URL     string `yaml:"url"`
			SHA1    string `yaml:"sha1"`
		} `yaml:"releases"`
	}

	if err := yaml.Unmarshal(runtimeConfigYAMLBytes, &runtimeConfig); err != nil {
		return fmt.Errorf("failed to parse runtime config: %w", err)
	}

	// Upload releases before applying runtime config
	for _, release := range runtimeConfig.Releases {
		if release.URL != "" {
			logger.Debug("startCommand", "Uploading release %s/%s from %s", release.Name, release.Version, release.URL)

			// Check if release already exists
			hasRelease, err := directorClient.HasRelease(release.Name, release.Version, boshdir.OSVersionSlug{})
			if err != nil {
				return fmt.Errorf("failed to check if release exists: %w", err)
			}

			if !hasRelease {
				// Upload the release using the URL
				if err := directorClient.UploadReleaseURL(release.URL, release.SHA1, false, false); err != nil {
					return fmt.Errorf("failed to upload release %s/%s: %w", release.Name, release.Version, err)
				}
				logger.Debug("startCommand", "Successfully uploaded release %s/%s", release.Name, release.Version)
			} else {
				logger.Debug("startCommand", "Release %s/%s already exists, skipping upload", release.Name, release.Version)
			}
		}
	}

	// Now apply the runtime config
	if err := directorClient.UpdateRuntimeConfig("enable-ssh", runtimeConfigYAMLBytes); err != nil {
		return fmt.Errorf("failed to update runtime-config: %w", err)
	}
	logger.Debug("startCommand", "Runtime-config applied successfully")

	return nil
}
