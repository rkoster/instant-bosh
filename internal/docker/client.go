package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	ContainerName  = "instant-bosh"
	NetworkName    = "instant-bosh"
	VolumeStore    = "instant-bosh-store"
	VolumeData     = "instant-bosh-data"
	ImageName      = "instant-bosh:latest"
	NetworkSubnet  = "10.245.0.0/16"
	NetworkGateway = "10.245.0.1"
	ContainerIP    = "10.245.0.10"
	DirectorPort   = "25555"
	SSHPort        = "2222"
)

type Client struct {
	cli    *client.Client
	logger boshlog.Logger
	logTag string
}

func NewClient(logger boshlog.Logger) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}

	return &Client{
		cli:    cli,
		logger: logger,
		logTag: "dockerClient",
	}, nil
}

func (c *Client) Close() error {
	return c.cli.Close()
}

func (c *Client) CreateVolume(ctx context.Context, name string) error {
	c.logger.Debug(c.logTag, "Creating volume %s", name)
	_, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name: name,
	})
	if err != nil {
		return fmt.Errorf("creating volume %s: %w", name, err)
	}
	return nil
}

func (c *Client) CreateNetwork(ctx context.Context) error {
	c.logger.Debug(c.logTag, "Creating network %s", NetworkName)
	_, err := c.cli.NetworkCreate(ctx, NetworkName, network.CreateOptions{
		IPAM: &network.IPAM{
			Config: []network.IPAMConfig{
				{
					Subnet:  NetworkSubnet,
					Gateway: NetworkGateway,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("creating network: %w", err)
	}
	return nil
}

func (c *Client) StartContainer(ctx context.Context) error {
	c.logger.Debug(c.logTag, "Creating container %s", ContainerName)

	config := &container.Config{
		Image: ImageName,
		Cmd: []string{
			"-v", "internal_ip=" + ContainerIP,
			"-v", "internal_cidr=" + NetworkSubnet,
			"-v", "internal_gw=" + NetworkGateway,
			"-v", "director_name=instant-bosh",
			"-v", "network=" + NetworkName,
		},
	}

	hostConfig := &container.HostConfig{
		Privileged:  true,
		AutoRemove:  true,
		NetworkMode: container.NetworkMode(NetworkName),
		PortBindings: nat.PortMap{
			"25555/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: DirectorPort}},
			"22/tcp":    []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: SSHPort}},
		},
		Binds: []string{
			"/var/run/docker.sock:/var/run/docker.sock",
			VolumeStore + ":/var/vcap/store",
			VolumeData + ":/var/vcap/data",
		},
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			NetworkName: {
				IPAddress: ContainerIP,
			},
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, ContainerName)
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	c.logger.Debug(c.logTag, "Starting container %s", resp.ID)
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	return nil
}

func (c *Client) StopContainer(ctx context.Context) error {
	c.logger.Debug(c.logTag, "Stopping container %s", ContainerName)
	timeout := 10
	if err := c.cli.ContainerStop(ctx, ContainerName, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("stopping container: %w", err)
	}
	return nil
}

func (c *Client) ContainerExists(ctx context.Context) (bool, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("name", "^/"+ContainerName+"$"),
		),
	})
	if err != nil {
		return false, fmt.Errorf("listing containers: %w", err)
	}
	return len(containers) > 0, nil
}

func (c *Client) IsContainerRunning(ctx context.Context) (bool, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("name", "^/"+ContainerName+"$"),
		),
	})
	if err != nil {
		return false, fmt.Errorf("listing containers: %w", err)
	}
	return len(containers) > 0, nil
}

func (c *Client) RemoveContainer(ctx context.Context) error {
	c.logger.Debug(c.logTag, "Removing container %s", ContainerName)
	if err := c.cli.ContainerRemove(ctx, ContainerName, container.RemoveOptions{
		Force: true,
	}); err != nil {
		return fmt.Errorf("removing container: %w", err)
	}
	return nil
}

func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	c.logger.Debug(c.logTag, "Removing volume %s", name)
	if err := c.cli.VolumeRemove(ctx, name, true); err != nil {
		return fmt.Errorf("removing volume %s: %w", name, err)
	}
	return nil
}

func (c *Client) RemoveNetwork(ctx context.Context) error {
	c.logger.Debug(c.logTag, "Removing network %s", NetworkName)
	if err := c.cli.NetworkRemove(ctx, NetworkName); err != nil {
		return fmt.Errorf("removing network: %w", err)
	}
	return nil
}

func (c *Client) GetContainersOnNetwork(ctx context.Context) ([]string, error) {
	networkResource, err := c.cli.NetworkInspect(ctx, NetworkName, network.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspecting network: %w", err)
	}

	var containerNames []string
	for _, endpoint := range networkResource.Containers {
		containerNames = append(containerNames, endpoint.Name)
	}
	return containerNames, nil
}

func (c *Client) GetContainerLogs(ctx context.Context, containerName string, tail string) (string, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	}

	logs, err := c.cli.ContainerLogs(ctx, containerName, options)
	if err != nil {
		return "", fmt.Errorf("getting container logs: %w", err)
	}
	defer logs.Close()

	logBytes, err := io.ReadAll(logs)
	if err != nil {
		return "", fmt.Errorf("reading logs: %w", err)
	}

	return string(logBytes), nil
}

func (c *Client) WaitForBoshReady(ctx context.Context, maxWait time.Duration) error {
	c.logger.Info(c.logTag, "Waiting for BOSH to be ready...")

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		running, err := c.IsContainerRunning(ctx)
		if err != nil {
			return err
		}
		if !running {
			logs, _ := c.GetContainerLogs(ctx, ContainerName, "100")
			return fmt.Errorf("container stopped unexpectedly. Last logs:\n%s", logs)
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for BOSH to start after %v", maxWait)
}
