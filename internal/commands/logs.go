package commands

import (
	"context"
	"os"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
)

func LogsAction(logger boshlog.Logger, follow bool, tail string) error {
	logTag := "logsCommand"

	ctx := context.Background()
	dockerClient, err := docker.NewClient(logger)
	if err != nil {
		return err
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

	return dockerClient.FollowContainerLogs(ctx, docker.ContainerName, follow, tail, os.Stdout, os.Stderr)
}
