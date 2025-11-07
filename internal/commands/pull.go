package commands

import (
	"context"
	"fmt"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/urfave/cli/v2"
)

func NewPullCommand(logger boshlog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "pull",
		Usage: "Pull the latest instant-bosh image",
		Action: func(c *cli.Context) error {
			return pullAction(logger)
		},
	}
}

func pullAction(logger boshlog.Logger) error {
	logTag := "pullCommand"
	ctx := context.Background()

	dockerClient, err := docker.NewClient(logger)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	if err := dockerClient.PullImage(ctx); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	logger.Info(logTag, "Successfully pulled latest instant-bosh image")
	return nil
}
