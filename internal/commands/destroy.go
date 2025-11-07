package commands

import (
	"context"
	"fmt"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/urfave/cli/v2"
)

func NewDestroyCommand(logger boshlog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "destroy",
		Usage: "Destroy instant-bosh and all associated resources",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Skip confirmation prompt",
			},
		},
		Action: func(c *cli.Context) error {
			return destroyAction(logger, c.Bool("force"))
		},
	}
}

func destroyAction(logger boshlog.Logger, force bool) error {
	logTag := "destroyCommand"
	ctx := context.Background()

	if !force {
		fmt.Println("This will remove the instant-bosh container, all containers on the instant-bosh network,")
		fmt.Println("and all associated volumes and networks.")
		fmt.Print("Are you sure? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		if response != "yes" {
			logger.Info(logTag, "Destroy operation cancelled")
			return nil
		}
	}

	dockerClient, err := docker.NewClient(logger)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	logger.Info(logTag, "Getting containers on instant-bosh network...")
	containers, err := dockerClient.GetContainersOnNetwork(ctx)
	if err != nil {
		logger.Debug(logTag, "Failed to get containers on network (network may not exist): %v", err)
	} else {
		for _, containerName := range containers {
			if containerName == docker.ContainerName {
				continue
			}
			logger.Info(logTag, "Removing container %s...", containerName)
			if err := dockerClient.RemoveContainer(ctx); err != nil {
				logger.Warn(logTag, "Failed to remove container %s: %v", containerName, err)
			}
		}
	}

	exists, err := dockerClient.ContainerExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		logger.Info(logTag, "Removing instant-bosh container...")
		if err := dockerClient.RemoveContainer(ctx); err != nil {
			logger.Warn(logTag, "Failed to remove container: %v", err)
		}
	}

	logger.Info(logTag, "Removing volumes...")
	if err := dockerClient.RemoveVolume(ctx, docker.VolumeStore); err != nil {
		logger.Warn(logTag, "Failed to remove volume %s: %v", docker.VolumeStore, err)
	}
	if err := dockerClient.RemoveVolume(ctx, docker.VolumeData); err != nil {
		logger.Warn(logTag, "Failed to remove volume %s: %v", docker.VolumeData, err)
	}

	logger.Info(logTag, "Removing network...")
	if err := dockerClient.RemoveNetwork(ctx); err != nil {
		logger.Warn(logTag, "Failed to remove network: %v", err)
	}

	logger.Info(logTag, "Destroy complete")
	return nil
}
