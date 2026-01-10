package incus

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	incus "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
	"github.com/lxc/incus/v6/shared/cliconfig"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

const (
	ContainerName      = "instant-bosh"
	NetworkName        = "ibosh-net"
	NetworkSubnet      = "10.246.0.0/16"
	NetworkGateway     = "10.246.0.1"
	ContainerIP        = "10.246.0.10"
	DirectorPort       = "25555"
	SSHPort            = "2222"
	DefaultProject     = "default"
	DefaultProfile     = "default"
	DefaultStoragePool = "default"
)

//counterfeiter:generate . ClientFactory
type ClientFactory interface {
	NewClient(logger boshlog.Logger, remote string, project string, network string, storagePool string, customImage string) (*Client, error)
}

type DefaultClientFactory struct{}

func (f *DefaultClientFactory) NewClient(logger boshlog.Logger, remote string, project string, network string, storagePool string, customImage string) (*Client, error) {
	return NewClient(logger, remote, project, network, storagePool, customImage)
}

type Client struct {
	cli         IncusAPI
	logger      boshlog.Logger
	logTag      string
	remote      string
	project     string
	imageName   string
	storagePool string
	networkName string
	cliConfig   *cliconfig.Config
}

func NewClient(logger boshlog.Logger, remote string, project string, network string, storagePool string, customImage string) (*Client, error) {
	var server incus.InstanceServer
	var err error
	var actualRemote string
	var cliConfig *cliconfig.Config

	// Load the incus CLI configuration to use existing remotes with their certificates
	server, actualRemote, cliConfig, err = connectUsingCLIConfig(logger, remote)
	if err != nil {
		return nil, err
	}

	if project == "" {
		project = DefaultProject
	}

	if network == "" {
		network = NetworkName
	}

	if storagePool == "" {
		storagePool = DefaultStoragePool
	}

	imageName := "ghcr.io/rkoster/instant-bosh:latest"
	if customImage != "" {
		imageName = customImage
	}

	projectServer := server.UseProject(project)

	client := &Client{
		cli:         &incusAPIWrapper{server: projectServer},
		logger:      logger,
		logTag:      "incusClient",
		remote:      actualRemote,
		project:     project,
		imageName:   imageName,
		storagePool: storagePool,
		networkName: network,
		cliConfig:   cliConfig,
	}

	return client, nil
}

// connectUsingCLIConfig attempts to connect to an Incus server using the incus CLI configuration.
// This allows reusing remotes configured via 'incus remote add' with their certificates.
// If remote is empty, it uses the default remote from the CLI configuration.
// Returns the connected server, the actual remote name used, and the CLI config.
func connectUsingCLIConfig(logger boshlog.Logger, remote string) (incus.InstanceServer, string, *cliconfig.Config, error) {
	// Determine the config directory
	configDir := os.Getenv("INCUS_CONF")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, "", nil, fmt.Errorf("getting home directory: %w", err)
		}
		configDir = filepath.Join(homeDir, ".config", "incus")
	}

	configPath := filepath.Join(configDir, "config.yml")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// No config file, fall back to local unix socket if no remote specified
		if remote == "" || remote == "local" {
			logger.Debug("incusClient", "No incus config found, connecting to local unix socket")
			server, err := incus.ConnectIncusUnix("", nil)
			if err != nil {
				return nil, "", nil, fmt.Errorf("connecting to local Incus: %w", err)
			}
			return server, "local", nil, nil
		}
		return nil, "", nil, fmt.Errorf("incus configuration not found at %s. Please configure the remote first using 'incus remote add'", configPath)
	}

	// Load the CLI configuration
	config, err := cliconfig.LoadConfig(configPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("loading incus config from %s: %w", configPath, err)
	}

	// Determine which remote to use
	remoteName := remote
	if remoteName == "" {
		// Use the default remote from CLI config
		remoteName = config.DefaultRemote
		if remoteName == "" {
			remoteName = "local"
		}
		logger.Debug("incusClient", "Using default remote: %s", remoteName)
	}

	// Check if the remote is a URL instead of a name
	isURL := strings.HasPrefix(remoteName, "https://") || strings.HasPrefix(remoteName, "http://")

	if isURL {
		// Find remote by URL
		found := false
		for name, r := range config.Remotes {
			if r.Addr == remoteName {
				remoteName = name
				found = true
				logger.Debug("incusClient", "Found remote '%s' matching URL %s", name, remote)
				break
			}
		}
		if !found {
			return nil, "", nil, fmt.Errorf("no configured remote found for URL %s. Please add it first using 'incus remote add <name> %s'", remote, remote)
		}
	} else {
		// Check if remote name exists
		if _, exists := config.Remotes[remoteName]; !exists {
			return nil, "", nil, fmt.Errorf("remote '%s' not found in incus configuration. Please add it first using 'incus remote add'", remoteName)
		}
	}

	logger.Debug("incusClient", "Connecting to remote '%s' using incus CLI configuration", remoteName)

	// Use the CLI config to get the instance server (handles certificates automatically)
	server, err := config.GetInstanceServer(remoteName)
	if err != nil {
		return nil, "", nil, fmt.Errorf("connecting to remote '%s': %w", remoteName, err)
	}

	return server, remoteName, config, nil
}

func (c *Client) Close() error {
	c.cli.Disconnect()
	return nil
}

func (c *Client) NetworkName() string {
	return c.networkName
}

func (c *Client) NetworkExists(ctx context.Context, name string) (bool, error) {
	c.logger.Debug(c.logTag, "Checking if network %s exists", name)
	_, _, err := c.cli.GetNetwork(name)
	if err != nil {
		if api.StatusErrorCheck(err, 404) {
			return false, nil
		}
		return false, fmt.Errorf("inspecting network %s: %w", name, err)
	}
	return true, nil
}

func (c *Client) CreateNetwork(ctx context.Context) error {
	c.logger.Debug(c.logTag, "Creating network %s", c.networkName)

	network := api.NetworksPost{
		Name: c.networkName,
		NetworkPut: api.NetworkPut{
			Config: map[string]string{
				"ipv4.address": NetworkGateway + "/16",
				"ipv4.nat":     "true",
			},
		},
		Type: "bridge",
	}

	err := c.cli.CreateNetwork(network)
	if err != nil {
		return fmt.Errorf("creating network: %w", err)
	}
	return nil
}

func (c *Client) ContainerExists(ctx context.Context) (bool, error) {
	_, _, err := c.cli.GetInstance(ContainerName)
	if err != nil {
		if api.StatusErrorCheck(err, 404) {
			return false, nil
		}
		return false, fmt.Errorf("checking if container exists: %w", err)
	}
	return true, nil
}

func (c *Client) IsContainerRunning(ctx context.Context) (bool, error) {
	instance, _, err := c.cli.GetInstance(ContainerName)
	if err != nil {
		if api.StatusErrorCheck(err, 404) {
			return false, nil
		}
		return false, fmt.Errorf("getting instance: %w", err)
	}
	return instance.Status == "Running", nil
}

func (c *Client) StartContainer(ctx context.Context) error {
	c.logger.Debug(c.logTag, "Creating container %s", ContainerName)

	imageFingerprint, err := c.EnsureImage(ctx, c.imageName)
	if err != nil {
		return fmt.Errorf("ensuring image: %w", err)
	}

	devices := map[string]map[string]string{
		"eth0": {
			"type":    "nic",
			"network": c.networkName,
			"name":    "eth0",
		},
		"root": {
			"type": "disk",
			"path": "/",
			"pool": c.storagePool,
		},
	}

	config := map[string]string{
		"security.privileged": "true",
		"raw.lxc":             "lxc.mount.auto = proc:rw sys:rw cgroup:rw\nlxc.apparmor.profile = unconfined",
	}

	req := api.InstancesPost{
		Name: ContainerName,
		Type: api.InstanceTypeContainer,
		InstancePut: api.InstancePut{
			Config:  config,
			Devices: devices,
		},
		Source: api.InstanceSource{
			Type:        "image",
			Fingerprint: imageFingerprint,
		},
	}

	op, err := c.cli.CreateInstance(req)
	if err != nil {
		return fmt.Errorf("creating instance: %w", err)
	}

	err = op.Wait()
	if err != nil {
		return fmt.Errorf("waiting for instance creation: %w", err)
	}

	c.logger.Debug(c.logTag, "Starting container %s", ContainerName)

	state := api.InstanceStatePut{
		Action:  "start",
		Timeout: -1,
	}

	op, err = c.cli.UpdateInstanceState(ContainerName, state, "")
	if err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	err = op.Wait()
	if err != nil {
		return fmt.Errorf("waiting for container to start: %w", err)
	}

	return nil
}

func (c *Client) StopContainer(ctx context.Context) error {
	c.logger.Debug(c.logTag, "Stopping container %s", ContainerName)

	state := api.InstanceStatePut{
		Action:  "stop",
		Timeout: 30,
		Force:   false,
	}

	op, err := c.cli.UpdateInstanceState(ContainerName, state, "")
	if err != nil {
		return fmt.Errorf("stopping container: %w", err)
	}

	err = op.Wait()
	if err != nil {
		return fmt.Errorf("waiting for container to stop: %w", err)
	}

	return nil
}

func (c *Client) RemoveContainer(ctx context.Context, containerName string) error {
	c.logger.Debug(c.logTag, "Removing container %s", containerName)

	op, err := c.cli.DeleteInstance(containerName)
	if err != nil {
		return fmt.Errorf("removing container: %w", err)
	}

	err = op.Wait()
	if err != nil {
		return fmt.Errorf("waiting for container removal: %w", err)
	}

	return nil
}

func (c *Client) ExecCommand(ctx context.Context, containerName string, cmd []string) (string, error) {
	c.logger.Debug(c.logTag, "Executing command in container %s: %v", containerName, cmd)

	req := api.InstanceExecPost{
		Command:     cmd,
		WaitForWS:   true,
		Interactive: false,
		Environment: map[string]string{},
	}

	args := incus.InstanceExecArgs{
		Stdin:  nil,
		Stdout: &strings.Builder{},
		Stderr: &strings.Builder{},
	}

	op, err := c.cli.ExecInstance(containerName, req, &args)
	if err != nil {
		return "", fmt.Errorf("executing command: %w", err)
	}

	err = op.Wait()
	if err != nil {
		stderrOutput := args.Stderr.(*strings.Builder).String()
		if stderrOutput != "" {
			return "", fmt.Errorf("command failed: %w\nstderr: %s", err, stderrOutput)
		}
		return "", fmt.Errorf("command failed: %w", err)
	}

	opAPI := op.Get()
	if opAPI.StatusCode != api.Success {
		return "", fmt.Errorf("command returned non-zero exit code: %d", opAPI.StatusCode)
	}

	output := args.Stdout.(*strings.Builder).String()
	return output, nil
}

func (c *Client) GetContainerLogs(ctx context.Context, containerName string, tail string) (string, error) {
	return "", fmt.Errorf("GetContainerLogs not yet implemented for Incus")
}

func (c *Client) EnsureImage(ctx context.Context, imageRef string) (string, error) {
	c.logger.Debug(c.logTag, "Ensuring OCI image %s is available", imageRef)

	// Parse image reference to determine registry and image path
	// Format: ghcr.io/rkoster/instant-bosh:latest or docker.io/library/ubuntu:22.04
	imageParts := strings.Split(imageRef, ":")
	if len(imageParts) != 2 {
		return "", fmt.Errorf("invalid image reference format: %s (expected repository:tag)", imageRef)
	}

	registryAndRepo := imageParts[0]
	tag := imageParts[1]

	// Split registry from repository path
	parts := strings.SplitN(registryAndRepo, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid image reference format: %s (expected registry/repository:tag)", imageRef)
	}
	registry := parts[0]
	repository := parts[1]

	// Map registry to OCI remote name (as configured in incus CLI)
	ociRemoteName := ""
	if c.cliConfig != nil {
		registryURL := "https://" + registry
		for name, remote := range c.cliConfig.Remotes {
			if remote.Protocol == "oci" && remote.Addr == registryURL {
				ociRemoteName = name
				c.logger.Debug(c.logTag, "Found OCI remote '%s' for registry %s", name, registry)
				break
			}
		}
	}

	if ociRemoteName != "" && c.cliConfig != nil {
		// Use the OCI remote via cliconfig to pull the image
		c.logger.Debug(c.logTag, "Using OCI remote '%s' to pull image", ociRemoteName)

		ociImageServer, err := c.cliConfig.GetImageServer(ociRemoteName)
		if err != nil {
			return "", fmt.Errorf("connecting to OCI remote '%s': %w", ociRemoteName, err)
		}

		// For OCI, the alias format is: repository:tag (without leading /)
		ociAlias := fmt.Sprintf("%s:%s", repository, tag)

		// Get the image alias from the OCI registry
		aliasEntry, _, err := ociImageServer.GetImageAlias(ociAlias)
		if err != nil {
			return "", fmt.Errorf("getting image alias '%s' from OCI remote: %w", ociAlias, err)
		}

		// Get the full image info
		image, _, err := ociImageServer.GetImage(aliasEntry.Target)
		if err != nil {
			return "", fmt.Errorf("getting image info from OCI remote: %w", err)
		}

		// Check if the image already exists on the Incus server
		_, _, err = c.cli.GetImage(image.Fingerprint)
		if err == nil {
			c.logger.Debug(c.logTag, "Image already exists on server, fingerprint=%s", image.Fingerprint)
			return image.Fingerprint, nil
		}

		// Copy the image from OCI registry to the Incus server
		c.logger.Info(c.logTag, "Copying OCI image from %s to Incus server", imageRef)
		copyOp, err := c.cli.CopyImage(ociImageServer, *image, &incus.ImageCopyArgs{
			CopyAliases: false,
			Public:      false,
		})
		if err != nil {
			return "", fmt.Errorf("copying image from OCI registry: %w", err)
		}

		err = copyOp.Wait()
		if err != nil {
			return "", fmt.Errorf("waiting for image copy: %w", err)
		}
		c.logger.Info(c.logTag, "Image copied successfully, fingerprint=%s", image.Fingerprint)

		return image.Fingerprint, nil
	}

	// No OCI remote configured - this is an error for remote Incus servers
	return "", fmt.Errorf("no OCI remote configured for registry %s. Please add it first using 'incus remote add <name> https://%s --protocol=oci'", registry, registry)
}

type incusAPIWrapper struct {
	server incus.InstanceServer
}

func (w *incusAPIWrapper) GetServer() (*api.Server, string, error) {
	return w.server.GetServer()
}

func (w *incusAPIWrapper) GetInstance(name string) (*api.Instance, string, error) {
	return w.server.GetInstance(name)
}

func (w *incusAPIWrapper) GetInstances(instanceType api.InstanceType) ([]api.Instance, error) {
	return w.server.GetInstances(instanceType)
}

func (w *incusAPIWrapper) CreateInstance(instance api.InstancesPost) (incus.Operation, error) {
	return w.server.CreateInstance(instance)
}

func (w *incusAPIWrapper) UpdateInstanceState(name string, state api.InstanceStatePut, ETag string) (incus.Operation, error) {
	return w.server.UpdateInstanceState(name, state, ETag)
}

func (w *incusAPIWrapper) DeleteInstance(name string) (incus.Operation, error) {
	return w.server.DeleteInstance(name)
}

func (w *incusAPIWrapper) ExecInstance(name string, req api.InstanceExecPost, args *incus.InstanceExecArgs) (incus.Operation, error) {
	return w.server.ExecInstance(name, req, args)
}

func (w *incusAPIWrapper) GetInstanceFile(instanceName string, filePath string) (io.ReadCloser, *incus.InstanceFileResponse, error) {
	return w.server.GetInstanceFile(instanceName, filePath)
}

func (w *incusAPIWrapper) DeleteInstanceFile(instanceName string, filePath string) error {
	return w.server.DeleteInstanceFile(instanceName, filePath)
}

func (w *incusAPIWrapper) CreateInstanceFromImage(source incus.ImageServer, image api.Image, req api.InstancesPost) (incus.RemoteOperation, error) {
	return w.server.CreateInstanceFromImage(source, image, req)
}

func (w *incusAPIWrapper) CopyImage(source incus.ImageServer, image api.Image, args *incus.ImageCopyArgs) (incus.RemoteOperation, error) {
	return w.server.CopyImage(source, image, args)
}

func (w *incusAPIWrapper) GetImage(fingerprint string) (*api.Image, string, error) {
	return w.server.GetImage(fingerprint)
}

func (w *incusAPIWrapper) GetImageAliases() ([]api.ImageAliasesEntry, error) {
	return w.server.GetImageAliases()
}

func (w *incusAPIWrapper) CreateImage(image api.ImagesPost, args *incus.ImageCreateArgs) (incus.Operation, error) {
	return w.server.CreateImage(image, args)
}

func (w *incusAPIWrapper) CreateImageAlias(alias api.ImageAliasesPost) error {
	return w.server.CreateImageAlias(alias)
}

func (w *incusAPIWrapper) DeleteImage(fingerprint string) (incus.Operation, error) {
	return w.server.DeleteImage(fingerprint)
}

func (w *incusAPIWrapper) GetNetwork(name string) (*api.Network, string, error) {
	return w.server.GetNetwork(name)
}

func (w *incusAPIWrapper) GetNetworks() ([]api.Network, error) {
	return w.server.GetNetworks()
}

func (w *incusAPIWrapper) CreateNetwork(network api.NetworksPost) error {
	return w.server.CreateNetwork(network)
}

func (w *incusAPIWrapper) DeleteNetwork(name string) error {
	return w.server.DeleteNetwork(name)
}

func (w *incusAPIWrapper) GetStoragePool(name string) (*api.StoragePool, string, error) {
	return w.server.GetStoragePool(name)
}

func (w *incusAPIWrapper) GetStoragePools() ([]api.StoragePool, error) {
	return w.server.GetStoragePools()
}

func (w *incusAPIWrapper) GetProfile(name string) (*api.Profile, string, error) {
	return w.server.GetProfile(name)
}

func (w *incusAPIWrapper) UseProject(name string) incus.InstanceServer {
	return w.server.UseProject(name)
}

func (w *incusAPIWrapper) UseTarget(name string) incus.InstanceServer {
	return w.server.UseTarget(name)
}

func (w *incusAPIWrapper) GetInstanceLogfiles(instanceName string) ([]string, error) {
	return w.server.GetInstanceLogfiles(instanceName)
}

func (w *incusAPIWrapper) GetInstanceLogfile(instanceName string, filename string) (io.ReadCloser, error) {
	return w.server.GetInstanceLogfile(instanceName, filename)
}

func (w *incusAPIWrapper) Disconnect() {
	w.server.Disconnect()
}
