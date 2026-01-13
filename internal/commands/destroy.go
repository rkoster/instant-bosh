package commands

import (
	"context"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/cpi"
)

func DestroyAction(ui UI, logger boshlog.Logger, cpiInstance cpi.CPI, force bool) error {
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

	exists, err := cpiInstance.Exists(ctx)
	if err != nil {
		logger.Debug("destroyCommand", "Failed to check container existence: %v", err)
		exists = false
	}

	if !exists {
		ui.PrintLinef("No instant-bosh resources found to destroy")
		return nil
	}

	ui.PrintLinef("Destroying instant-bosh resources...")
	if err := cpiInstance.Destroy(ctx); err != nil {
		ui.ErrorLinef("Failed to destroy resources: %s", err)
		return err
	}

	ui.PrintLinef("Destroy complete")
	return nil
}
