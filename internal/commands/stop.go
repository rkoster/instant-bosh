package commands

import (
	"context"
	"fmt"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/cpi"
)

func StopAction(ui UI, logger boshlog.Logger, cpiInstance cpi.CPI) error {
	ctx := context.Background()

	running, err := cpiInstance.IsRunning(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if container is running: %w", err)
	}

	if !running {
		ui.PrintLinef("instant-bosh is not running")
		return nil
	}

	ui.PrintLinef("Stopping instant-bosh container...")
	if err := cpiInstance.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	ui.PrintLinef("instant-bosh stopped successfully")
	return nil
}
