package commands

import (
	"context"
	"fmt"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/urfave/cli/v2"
)

func NewStopCommand(logger boshlog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "stop",
		Usage: "Stop instant-bosh director",
		Action: func(c *cli.Context) error {
			return stopAction(logger)
		},
	}
}

func stopAction(logger boshlog.Logger) error {
	logTag := "stopCommand"
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
		logger.Info(logTag, "instant-bosh is not running")
		return nil
	}

	logger.Info(logTag, "Stopping instant-bosh container...")
	if err := dockerClient.StopContainer(ctx); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	logger.Info(logTag, "instant-bosh stopped successfully")
	return nil
}
