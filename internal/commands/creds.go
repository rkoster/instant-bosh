package commands

import (
	"github.com/rkoster/instant-bosh/internal/configserver"
)

// CredsGetAction retrieves a credential by name from the config-server
func CredsGetAction(ui UI, name string, outputJSON bool) error {
	client, err := configserver.NewClientFromEnv()
	if err != nil {
		return err
	}

	cred, err := client.Get(name)
	if err != nil {
		return err
	}

	if outputJSON {
		ui.PrintLinef("%s", configserver.FormatValueJSON(cred.Value))
	} else {
		ui.PrintLinef("%s", configserver.FormatValue(cred.Value))
	}

	return nil
}

// CredsFindAction lists credentials from the config-server
// Note: config-server doesn't support listing, so we provide helpful guidance
func CredsFindAction(ui UI, pathPrefix string) error {
	// Config-server doesn't support listing credentials like CredHub does.
	// Provide helpful instructions for users.
	ui.PrintLinef("Config-server does not support listing credentials.")
	ui.PrintLinef("")
	ui.PrintLinef("To see available credentials, use the BOSH CLI:")
	ui.PrintLinef("  bosh variables -d <deployment>")
	ui.PrintLinef("")
	ui.PrintLinef("For example:")
	ui.PrintLinef("  bosh variables -d cf")
	ui.PrintLinef("")
	ui.PrintLinef("Then retrieve a specific credential with:")
	ui.PrintLinef("  ibosh creds get /instant-bosh/<deployment>/<variable-name>")
	ui.PrintLinef("")
	ui.PrintLinef("Example:")
	ui.PrintLinef("  ibosh creds get /instant-bosh/cf/cf_admin_password")

	return nil
}
