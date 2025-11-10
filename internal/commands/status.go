package commands

import (
	"context"
	"fmt"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
)

func StatusAction(ui boshui.UI, logger boshlog.Logger) error {
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

	ui.PrintLinef("instant-bosh status:")
	if running {
		ui.PrintLinef("  State: Running")
		ui.PrintLinef("  Container: %s", docker.ContainerName)
		ui.PrintLinef("  Network: %s", docker.NetworkName)
		ui.PrintLinef("  IP: %s", docker.ContainerIP)
		ui.PrintLinef("  Director Port: %s", docker.DirectorPort)
		ui.PrintLinef("  SSH Port: %s", docker.SSHPort)
	} else {
		ui.PrintLinef("  State: Stopped")
	}

	containers, err := dockerClient.GetContainersOnNetwork(ctx)
	if err != nil {
		ui.PrintLinef("")
		ui.PrintLinef("Containers on network: Unable to retrieve (network may not exist)")
		return nil
	}

	ui.PrintLinef("")
	ui.PrintLinef("Containers on %s network:", docker.NetworkName)
	if len(containers) == 0 {
		ui.PrintLinef("  None")
	} else {
		for _, containerName := range containers {
			ui.PrintLinef("  - %s", containerName)
		}
	}

	return nil
}
