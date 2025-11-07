package commands

import (
	"context"
	"fmt"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/urfave/cli/v2"
)

func NewStatusCommand(logger boshlog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show status of instant-bosh and containers on the network",
		Action: func(c *cli.Context) error {
			return statusAction(logger)
		},
	}
}

func statusAction(logger boshlog.Logger) error {
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

	fmt.Println("instant-bosh status:")
	if running {
		fmt.Println("  State: Running")
		fmt.Printf("  Container: %s\n", docker.ContainerName)
		fmt.Printf("  Network: %s\n", docker.NetworkName)
		fmt.Printf("  IP: %s\n", docker.ContainerIP)
		fmt.Printf("  Director Port: %s\n", docker.DirectorPort)
		fmt.Printf("  SSH Port: %s\n", docker.SSHPort)
	} else {
		fmt.Println("  State: Stopped")
	}

	containers, err := dockerClient.GetContainersOnNetwork(ctx)
	if err != nil {
		fmt.Printf("\nContainers on network: Unable to retrieve (network may not exist)\n")
		return nil
	}

	fmt.Printf("\nContainers on %s network:\n", docker.NetworkName)
	if len(containers) == 0 {
		fmt.Println("  None")
	} else {
		for _, containerName := range containers {
			fmt.Printf("  - %s\n", containerName)
		}
	}

	return nil
}
