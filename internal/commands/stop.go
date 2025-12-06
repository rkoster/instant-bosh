package commands

import (
	"context"
	"fmt"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
)

func StopAction(ui boshui.UI, logger boshlog.Logger) error {
	ctx := context.Background()

	dockerClient, err := docker.NewClient(logger, "")
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}
	if !running {
		ui.PrintLinef("instant-bosh is not running")
		return nil
	}

	ui.PrintLinef("Stopping instant-bosh container...")
	if err := dockerClient.StopContainer(ctx); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	ui.PrintLinef("instant-bosh stopped successfully")
	return nil
}
