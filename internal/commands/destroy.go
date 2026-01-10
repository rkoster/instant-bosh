package commands

import (
	"context"
	"fmt"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/incus"
)

func DestroyAction(ui boshui.UI, logger boshlog.Logger, force bool) error {
	return DestroyActionWithFactory(ui, logger, &docker.DefaultClientFactory{}, force)
}

func DestroyActionWithFactory(ui UI, logger boshlog.Logger, clientFactory docker.ClientFactory, force bool) error {
	logTag := "destroyCommand"
	ctx := context.Background()

	if !force {
		ui.PrintLinef("This will remove the instant-bosh container, all containers on the instant-bosh network,")
		ui.PrintLinef("and all associated volumes and networks.")
		ui.PrintLinef("")
		err := ui.AskForConfirmation()
		if err != nil {
			ui.PrintLinef("Destroy operation cancelled")
			return nil
		}
	}

	dockerClient, err := clientFactory.NewClient(logger, "")
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	dockerExists, err := dockerClient.ContainerExists(ctx)
	if err != nil {
		logger.Debug(logTag, "Failed to check Docker container existence: %v", err)
		dockerExists = false
	}

	ui.PrintLinef("Getting containers on instant-bosh network...")
	containers, err := dockerClient.GetContainersOnNetwork(ctx)
	if err != nil {
		logger.Debug(logTag, "Failed to get containers on network (network may not exist): %v", err)
	} else {
		for _, containerName := range containers {
			if containerName == docker.ContainerName {
				continue
			}
			ui.PrintLinef("Removing container %s...", containerName)
			if err := dockerClient.RemoveContainer(ctx, containerName); err != nil {
				ui.ErrorLinef("  Failed to remove container %s: %s", containerName, err)
				logger.Warn(logTag, "Failed to remove container %s: %v", containerName, err)
			}
		}
	}

	if dockerExists {
		ui.PrintLinef("Removing instant-bosh container...")
		if err := dockerClient.RemoveContainer(ctx, docker.ContainerName); err != nil {
			ui.ErrorLinef("  Failed to remove container: %s", err)
			logger.Warn(logTag, "Failed to remove container: %v", err)
		}
	}

	ui.PrintLinef("Removing volumes...")
	if err := dockerClient.RemoveVolume(ctx, docker.VolumeStore); err != nil {
		ui.ErrorLinef("  Failed to remove volume %s: %s", docker.VolumeStore, err)
		logger.Warn(logTag, "Failed to remove volume %s: %v", docker.VolumeStore, err)
	}
	if err := dockerClient.RemoveVolume(ctx, docker.VolumeData); err != nil {
		ui.ErrorLinef("  Failed to remove volume %s: %s", docker.VolumeData, err)
		logger.Warn(logTag, "Failed to remove volume %s: %v", docker.VolumeData, err)
	}

	ui.PrintLinef("Removing network...")
	if err := dockerClient.RemoveNetwork(ctx); err != nil {
		ui.ErrorLinef("  Failed to remove network: %s", err)
		logger.Warn(logTag, "Failed to remove network: %v", err)
	}

	incusFactory := &incus.DefaultClientFactory{}
	incusClient, err := incusFactory.NewClient(logger, "", "", "", "", "")
	if err != nil {
		logger.Debug(logTag, "Failed to create Incus client: %v", err)
	} else {
		defer incusClient.Close()

		incusExists, err := incusClient.ContainerExists(ctx)
		if err != nil {
			logger.Debug(logTag, "Failed to check Incus container existence: %v", err)
			incusExists = false
		}

		if incusExists {
			ui.PrintLinef("Destroying Incus mode resources...")

			ui.PrintLinef("Removing instant-bosh container...")
			if err := incusClient.RemoveContainer(ctx, incus.ContainerName); err != nil {
				ui.ErrorLinef("  Failed to remove container: %s", err)
				logger.Warn(logTag, "Failed to remove container: %v", err)
			}

			ui.PrintLinef("Note: Incus network and VMs must be manually cleaned up")
		}
	}

	if !dockerExists && (incusClient == nil || !func() bool {
		exists, _ := incusClient.ContainerExists(ctx)
		return exists
	}()) {
		ui.PrintLinef("No instant-bosh resources found to destroy")
		return nil
	}

	ui.PrintLinef("Destroy complete")
	return nil
}
