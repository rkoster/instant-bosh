package commands

import (
	"context"
	"fmt"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/incus"
)

func StopAction(ui boshui.UI, logger boshlog.Logger) error {
	return StopActionWithFactory(ui, logger, &docker.DefaultClientFactory{})
}

func StopActionWithFactory(ui UI, logger boshlog.Logger, clientFactory docker.ClientFactory) error {
	ctx := context.Background()

	dockerClient, err := clientFactory.NewClient(logger, "")
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	dockerRunning, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}

	if dockerRunning {
		ui.PrintLinef("Stopping instant-bosh container (Docker mode)...")
		if err := dockerClient.StopContainer(ctx); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}
		ui.PrintLinef("instant-bosh stopped successfully")
		return nil
	}

	incusFactory := &incus.DefaultClientFactory{}
	incusClient, err := incusFactory.NewClient(logger, "", "", "", "", "")
	if err != nil {
		ui.PrintLinef("instant-bosh is not running")
		return nil
	}
	defer incusClient.Close()

	incusRunning, err := incusClient.IsContainerRunning(ctx)
	if err != nil {
		ui.PrintLinef("instant-bosh is not running")
		return nil
	}

	if incusRunning {
		ui.PrintLinef("Stopping instant-bosh container (Incus mode)...")
		if err := incusClient.StopContainer(ctx); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}
		ui.PrintLinef("instant-bosh stopped successfully")
		return nil
	}

	ui.PrintLinef("instant-bosh is not running")
	return nil
}
