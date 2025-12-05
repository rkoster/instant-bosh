package docker

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const (
	ContainerName  = "instant-bosh"
	NetworkName    = "instant-bosh"
	VolumeStore    = "instant-bosh-store"
	VolumeData     = "instant-bosh-data"
	ImageName      = "ghcr.io/rkoster/instant-bosh:latest"
	NetworkSubnet  = "10.245.0.0/16"
	NetworkGateway = "10.245.0.1"
	ContainerIP    = "10.245.0.10"
	DirectorPort   = "25555"
	SSHPort        = "2222"
)

type Client struct {
	cli        *client.Client
	logger     boshlog.Logger
	logTag     string
	socketPath string
}

// getDockerHost attempts to get the Docker host from the current context
// by inspecting the Docker CLI context (if available). Falls back to empty string
// if the Docker CLI is not available or the context inspection fails.
func getDockerHost() string {
	// Check if docker CLI is available
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		// Docker CLI not available, caller will use client.FromEnv
		return ""
	}

	// Try to get the current context's Docker host
	cmd := exec.Command(dockerPath, "context", "inspect", "-f", "{{.Endpoints.docker.Host}}")
	output, err := cmd.Output()
	if err != nil {
		// Context inspection failed, caller will use client.FromEnv
		return ""
	}

	host := strings.TrimSpace(string(output))
	if host == "" {
		return ""
	}

	return host
}

func NewClient(logger boshlog.Logger) (*Client, error) {
	// Try to get Docker host from current context if Docker CLI is available
	dockerHost := getDockerHost()

	var cli *client.Client
	var err error

	if dockerHost != "" {
		// Use the host from the Docker context
		cli, err = client.NewClientWithOpts(
			client.WithHost(dockerHost),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			return nil, fmt.Errorf("creating docker client with host %s: %w", dockerHost, err)
		}
	} else {
		// Fall back to client.FromEnv which reads DOCKER_HOST environment variable
		cli, err = client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			return nil, fmt.Errorf("creating docker client: %w", err)
		}
	}

	// Extract socket path from Docker daemon host for mounting into containers.
	// DaemonHost() returns the host the client is connected to (e.g., "unix:///path/to/docker.sock").
	socketPath := "/var/run/docker.sock" // default fallback
	daemonHost := cli.DaemonHost()
	if len(daemonHost) > 7 && daemonHost[:7] == "unix://" {
		socketPath = daemonHost[7:] // strip "unix://" prefix
	}

	return &Client{
		cli:        cli,
		logger:     logger,
		logTag:     "dockerClient",
		socketPath: socketPath,
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
		ExposedPorts: nat.PortSet{
			"25555/tcp": struct{}{},
			"22/tcp":    struct{}{},
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
			// NOTE: The socket bind mount should always be /var/run/docker.sock:/var/run/docker.sock
			// because Docker runs in a VM and the socket INSIDE the VM is always at /var/run/docker.sock.
			// The host socket path (c.socketPath) is used by the Docker client to connect to the daemon
			// from the host, but inside the VM, the socket is at the standard location.
			"/var/run/docker.sock:/var/run/docker.sock",
			VolumeStore + ":/var/vcap/store",
			VolumeData + ":/var/vcap/data",
		},
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			NetworkName: {
				IPAMConfig: &network.EndpointIPAMConfig{
					IPv4Address: ContainerIP,
				},
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

func (c *Client) RemoveContainer(ctx context.Context, containerName string) error {
	c.logger.Debug(c.logTag, "Removing container %s", containerName)
	if err := c.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{
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

	// Docker multiplexes stdout and stderr, so we need to demultiplex them
	// Write both streams to the same buffer to preserve chronological order
	var combined bytes.Buffer
	if _, err := stdcopy.StdCopy(&combined, &combined, logs); err != nil {
		return "", fmt.Errorf("demultiplexing logs: %w", err)
	}

	return combined.String(), nil
}

func (c *Client) WaitForBoshReady(ctx context.Context, maxWait time.Duration) error {
	c.logger.Info(c.logTag, "Waiting for BOSH to be ready...")

	// Create HTTP client that skips TLS verification (BOSH uses self-signed certs)
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	boshURL := fmt.Sprintf("https://localhost:%s/info", DirectorPort)
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		// First check if container is still running
		running, err := c.IsContainerRunning(ctx)
		if err != nil {
			return err
		}
		if !running {
			logs, _ := c.GetContainerLogs(ctx, ContainerName, "100")
			return fmt.Errorf("container stopped unexpectedly. Last logs:\n%s", logs)
		}

		// Try to connect to BOSH /info endpoint
		resp, err := httpClient.Get(boshURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				c.logger.Info(c.logTag, "BOSH director is ready")
				return nil
			}
		}

		time.Sleep(2 * time.Second)
	}

	// Timeout - get logs for debugging
	logs, _ := c.GetContainerLogs(ctx, ContainerName, "100")
	return fmt.Errorf("timeout waiting for BOSH to start after %v. Last logs:\n%s", maxWait, logs)
}

func (c *Client) ImageExists(ctx context.Context) (bool, error) {
	_, _, err := c.cli.ImageInspectWithRaw(ctx, ImageName)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspecting image: %w", err)
	}
	return true, nil
}

type pullProgress struct {
	Status         string `json:"status"`
	ProgressDetail struct {
		Current int `json:"current"`
		Total   int `json:"total"`
	} `json:"progressDetail"`
	Progress string `json:"progress"`
	ID       string `json:"id"`
}

func (c *Client) PullImage(ctx context.Context) error {
	c.logger.Info(c.logTag, "Pulling image %s...", ImageName)

	out, err := c.cli.ImagePull(ctx, ImageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}
	defer out.Close()

	decoder := json.NewDecoder(out)
	var lastStatus string
	for {
		var progress pullProgress
		if err := decoder.Decode(&progress); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("reading pull progress: %w", err)
		}

		if progress.Status != lastStatus && progress.ID == "" {
			c.logger.Info(c.logTag, "%s", progress.Status)
			lastStatus = progress.Status
		}
	}

	c.logger.Info(c.logTag, "Image pulled successfully")
	return nil
}

// CheckForImageUpdate checks if a newer version of the image is available in the registry.
//
// This method queries the Docker Registry API v2 to compare the remote manifest digest
// with the local image digest WITHOUT downloading the image layers. This makes the check
// fast and bandwidth-efficient.
//
// Returns true if a newer version is available in the registry, false if the local image
// is up to date.
func (c *Client) CheckForImageUpdate(ctx context.Context) (bool, error) {
	c.logger.Debug(c.logTag, "Checking for image updates for %s", ImageName)

	// Get the current local image
	localImage, _, err := c.cli.ImageInspectWithRaw(ctx, ImageName)
	if err != nil {
		if client.IsErrNotFound(err) {
			// Image doesn't exist locally, so an update is needed
			return true, nil
		}
		return false, fmt.Errorf("inspecting local image: %w", err)
	}

	// Get the local image digest from RepoDigests
	// RepoDigests contains the registry digest(s) for the image
	var localDigest string
	if len(localImage.RepoDigests) > 0 {
		// Extract just the digest part (after @)
		parts := strings.Split(localImage.RepoDigests[0], "@")
		if len(parts) == 2 {
			localDigest = parts[1]
		}
	}

	// If we don't have a repo digest, we need to pull to check
	// This can happen if the image was built locally or loaded from a tarball
	if localDigest == "" {
		c.logger.Debug(c.logTag, "Local image has no repo digest, needs update check via pull")
		return true, nil
	}

	// Query the Docker Registry API to get the latest manifest digest
	// We use ImagePull with platform filtering to just get the manifest without downloading layers
	// The RegistryAuth is empty for public registries
	remoteDigest, err := c.getRemoteImageDigest(ctx, ImageName)
	if err != nil {
		// If we can't check the remote, we'll treat it as no update available
		// The actual pull will fail later if there's a real connectivity issue
		c.logger.Debug(c.logTag, "Failed to check remote image digest: %v", err)
		return false, fmt.Errorf("checking remote image: %w", err)
	}

	// Compare digests
	updateAvailable := localDigest != remoteDigest
	if updateAvailable {
		c.logger.Info(c.logTag, "New image version available (local: %s, remote: %s)", 
			localDigest[:12], remoteDigest[:12])
	} else {
		c.logger.Debug(c.logTag, "Image is up to date (digest: %s)", localDigest[:12])
	}

	return updateAvailable, nil
}

// getRemoteImageDigest queries the Docker registry to get the digest of the remote image
// without pulling the actual image layers.
func (c *Client) getRemoteImageDigest(ctx context.Context, imageName string) (string, error) {
	// Parse the image name to extract registry, repository, and tag
	// Format: [registry/]repository[:tag]
	registry := "ghcr.io"
	repository := strings.TrimPrefix(imageName, registry+"/")
	tag := "latest"
	
	if strings.Contains(repository, ":") {
		parts := strings.Split(repository, ":")
		repository = parts[0]
		tag = parts[1]
	}

	// Build the registry API URL
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)
	
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	// Set the Accept header to request the manifest
	// We use application/vnd.docker.distribution.manifest.v2+json for the Docker v2 manifest format
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")

	// Create HTTP client with TLS config
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		},
		Timeout: 30 * time.Second,
	}

	// Make the request
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting manifest: %w", err)
	}
	defer resp.Body.Close()

	// Check for authentication requirement
	if resp.StatusCode == http.StatusUnauthorized {
		// For public registries, we might need to get a token first
		// This is a simplified version - for now we'll fall back to assuming update needed
		return "", fmt.Errorf("registry requires authentication")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Get the digest from the response header
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no digest in response header")
	}

	return digest, nil
}

func (c *Client) ExecCommand(ctx context.Context, containerName string, cmd []string) (string, error) {
	c.logger.Debug(c.logTag, "Executing command in container %s: %v", containerName, cmd)

	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := c.cli.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return "", fmt.Errorf("creating exec: %w", err)
	}

	resp, err := c.cli.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("attaching to exec: %w", err)
	}
	defer resp.Close()

	// Docker multiplexes stdout and stderr, we need to demultiplex
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		return "", fmt.Errorf("reading exec output: %w", err)
	}

	// Return stdout, log stderr if present
	if stderr.Len() > 0 {
		c.logger.Debug(c.logTag, "Exec stderr: %s", stderr.String())
	}

	return stdout.String(), nil
}

func (c *Client) FollowContainerLogs(ctx context.Context, containerName string, follow bool, tail string, stdout io.Writer, stderr io.Writer) error {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
	}

	logs, err := c.cli.ContainerLogs(ctx, containerName, options)
	if err != nil {
		return fmt.Errorf("getting container logs: %w", err)
	}
	defer logs.Close()

	if _, err := stdcopy.StdCopy(stdout, stderr, logs); err != nil {
		return fmt.Errorf("streaming logs: %w", err)
	}

	return nil
}
