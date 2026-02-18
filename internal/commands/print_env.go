package commands

import (
	"context"
	"fmt"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/cpi"
	"github.com/rkoster/instant-bosh/internal/director"
)

func PrintEnvAction(ui UI, logger boshlog.Logger, cpiInstance cpi.CPI, configProvider director.ConfigProvider) error {
	ctx := context.Background()

	running, err := cpiInstance.IsRunning(ctx)
	if err != nil {
		return err
	}

	if !running {
		return fmt.Errorf("instant-bosh container is not running. Please run 'ibosh start' first")
	}

	config, err := configProvider.GetDirectorConfig(ctx, cpiInstance, cpiInstance.GetContainerName())
	if err != nil {
		return fmt.Errorf("failed to get director config: %w", err)
	}

	ui.PrintLinef("export BOSH_CLIENT=%s", config.Client)
	ui.PrintLinef("export BOSH_CLIENT_SECRET=%s", config.ClientSecret)
	ui.PrintLinef("export BOSH_ENVIRONMENT=%s", config.Environment)
	ui.PrintLinef("export BOSH_CA_CERT='%s'", config.CACert)
	if config.AllProxy != "" {
		ui.PrintLinef("export BOSH_ALL_PROXY=%s", config.AllProxy)
	}

	// Config-server environment variables for ibosh creds commands
	ui.PrintLinef("export CONFIG_SERVER_URL=%s", config.ConfigServerURL)
	ui.PrintLinef("export CONFIG_SERVER_CLIENT=%s", config.ConfigServerClient)
	ui.PrintLinef("export CONFIG_SERVER_SECRET=%s", config.ConfigServerSecret)
	ui.PrintLinef("export CONFIG_SERVER_CA_CERT='%s'", config.ConfigServerCACert)
	ui.PrintLinef("export UAA_URL=%s", config.UAAURL)
	ui.PrintLinef("export UAA_CA_CERT='%s'", config.UAACACert)

	return nil
}
