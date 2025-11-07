package main

import (
	"fmt"
	"os"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/urfave/cli/v2"
)

func main() {
	logger := boshlog.NewWriterLogger(boshlog.LevelDebug, os.Stderr)

	app := &cli.App{
		Name:  "ibosh",
		Usage: "instant-bosh CLI",
		Commands: []*cli.Command{
			commands.NewStartCommand(logger),
			commands.NewStopCommand(logger),
			commands.NewDestroyCommand(logger),
			commands.NewStatusCommand(logger),
			commands.NewPullCommand(logger),
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
