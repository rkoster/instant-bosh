package commands

import (
	"context"
	"fmt"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/incus"
)

func PrintEnvAction(ui boshui.UI, logger boshlog.Logger) error {
	return PrintEnvActionWithFactories(ui, logger, &docker.DefaultClientFactory{}, &incus.DefaultClientFactory{}, &director.DefaultConfigProvider{})
}

func PrintEnvActionWithFactories(ui UI, logger boshlog.Logger, dockerClientFactory docker.ClientFactory, incusClientFactory incus.ClientFactory, configProvider director.ConfigProvider) error {
	ctx := context.Background()

	dockerClient, err := dockerClientFactory.NewClient(logger, "")
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}

	if running {
		config, err := configProvider.GetDirectorConfig(ctx, dockerClient, docker.ContainerName)
		if err != nil {
			return fmt.Errorf("failed to get director config: %w", err)
		}

		ui.PrintLinef("export BOSH_CLIENT=%s", config.Client)
		ui.PrintLinef("export BOSH_CLIENT_SECRET=%s", config.ClientSecret)
		ui.PrintLinef("export BOSH_ENVIRONMENT=%s", config.Environment)
		ui.PrintLinef("export BOSH_CA_CERT='%s'", config.CACert)
		ui.PrintLinef("export BOSH_ALL_PROXY=%s", config.AllProxy)
		return nil
	}

	incusClient, err := incusClientFactory.NewClient(logger, "local", incus.DefaultProject, "")
	if err != nil {
		return fmt.Errorf("failed to create incus client: %w", err)
	}
	defer incusClient.Close()

	running, err = incusClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}

	if !running {
		return fmt.Errorf("instant-bosh container is not running. Please run 'ibosh start' first")
	}

	config, err := configProvider.GetDirectorConfig(ctx, incusClient, incus.ContainerName)
	if err != nil {
		return fmt.Errorf("failed to get director config: %w", err)
	}

	ui.PrintLinef("export BOSH_CLIENT=%s", config.Client)
	ui.PrintLinef("export BOSH_CLIENT_SECRET=%s", config.ClientSecret)
	ui.PrintLinef("export BOSH_ENVIRONMENT=%s", config.Environment)
	ui.PrintLinef("export BOSH_CA_CERT='%s'", config.CACert)
	ui.PrintLinef("export BOSH_ALL_PROXY=%s", config.AllProxy)

	return nil
}
