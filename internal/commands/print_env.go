package commands

import (
	"context"
	"fmt"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/docker"
)

func PrintEnvAction(ui boshui.UI, logger boshlog.Logger) error {
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

	if !running {
		return fmt.Errorf("instant-bosh container is not running. Please run 'ibosh start' first")
	}

	// Get director configuration
	config, err := director.GetDirectorConfig(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("failed to get director config: %w", err)
	}
	defer config.Cleanup()

	// Print environment variables to stdout (must use ui.PrintLinef which goes to outWriter/stdout)
	ui.PrintLinef("export BOSH_CLIENT=%s", config.Client)
	ui.PrintLinef("export BOSH_CLIENT_SECRET=%s", config.ClientSecret)
	ui.PrintLinef("export BOSH_ENVIRONMENT=%s", config.Environment)
	ui.PrintLinef("export BOSH_CA_CERT='%s'", config.CACert)
	ui.PrintLinef("export BOSH_ALL_PROXY=%s", config.AllProxy)

	return nil
}
