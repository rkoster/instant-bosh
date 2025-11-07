package commands

import (
	"context"
	"fmt"
	"time"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/urfave/cli/v2"
)

func NewStartCommand(logger boshlog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "start",
		Usage: "Start instant-bosh director",
		Action: func(c *cli.Context) error {
			return startAction(logger)
		},
	}
}

func startAction(logger boshlog.Logger) error {
	logTag := "startCommand"
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
	if running {
		logger.Info(logTag, "instant-bosh is already running")
		return nil
	}

	imageExists, err := dockerClient.ImageExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if image exists: %w", err)
	}
	if !imageExists {
		logger.Info(logTag, "Image not found locally, pulling...")
		if err := dockerClient.PullImage(ctx); err != nil {
			return fmt.Errorf("failed to pull image: %w", err)
		}
	}

	logger.Info(logTag, "Creating volumes...")
	if err := dockerClient.CreateVolume(ctx, docker.VolumeStore); err != nil {
		logger.Debug(logTag, "Volume %s may already exist: %v", docker.VolumeStore, err)
	}
	if err := dockerClient.CreateVolume(ctx, docker.VolumeData); err != nil {
		logger.Debug(logTag, "Volume %s may already exist: %v", docker.VolumeData, err)
	}

	logger.Info(logTag, "Creating network...")
	if err := dockerClient.CreateNetwork(ctx); err != nil {
		logger.Debug(logTag, "Network may already exist: %v", err)
	}

	logger.Info(logTag, "Starting instant-bosh container...")
	if err := dockerClient.StartContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	if err := dockerClient.WaitForBoshReady(ctx, 5*time.Minute); err != nil {
		return fmt.Errorf("BOSH failed to become ready: %w", err)
	}

	logger.Info(logTag, "instant-bosh is ready!")
	fmt.Println("\nTo configure your BOSH CLI environment, run:")
	fmt.Println("  eval \"$(make print-env)\"")

	return nil
}
