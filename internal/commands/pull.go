package commands

import (
	"context"
	"fmt"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
)

func PullAction(ui boshui.UI, logger boshlog.Logger) error {
	ctx := context.Background()

	dockerClient, err := docker.NewClient(logger)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	if err := dockerClient.PullImage(ctx); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	ui.PrintLinef("Successfully pulled latest instant-bosh image")
	return nil
}
