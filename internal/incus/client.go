package incus

import (
	"context"
	"fmt"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	incus "github.com/lxc/incus/client"
	"github.com/lxc/incus/shared/api"
)

const (
	ContainerName    = "instant-bosh"
	NetworkName      = "instant-bosh-incus"
	NetworkSubnet    = "10.246.0.0/16"
	NetworkGateway   = "10.246.0.1"
	ContainerIP      = "10.246.0.10"
	DirectorPort     = "25555"
	SSHPort          = "2222"
	DefaultProject   = "default"
	DefaultProfile   = "default"
	DefaultStoragePool = "default"
)

type ClientFactory interface {
	NewClient(logger boshlog.Logger, remote string, project string, customImage string) (*Client, error)
}

type DefaultClientFactory struct{}

func (f *DefaultClientFactory) NewClient(logger boshlog.Logger, remote string, project string, customImage string) (*Client, error) {
	return NewClient(logger, remote, project, customImage)
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
}

func NewClient(logger boshlog.Logger, remote string, project string, customImage string) (*Client, error) {
	var server incus.InstanceServer
	var err error
	
	if remote == "" || remote == "local" {
		server, err = incus.ConnectIncusUnix("", nil)
		if err != nil {
			return nil, fmt.Errorf("connecting to local Incus: %w", err)
		}
	} else {
		return nil, fmt.Errorf("remote Incus connections not yet implemented")
	}
	
	if project == "" {
		project = DefaultProject
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
		remote:      remote,
		project:     project,
		imageName:   imageName,
		storagePool: DefaultStoragePool,
		networkName: NetworkName,
	}
	
	return client, nil
}

func (c *Client) Close() error {
	c.cli.Disconnect()
	return nil
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
	
	convertedImage, err := c.EnsureImage(ctx, c.imageName)
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
			Fingerprint: convertedImage,
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

func (c *Client) GetContainerLogs(ctx context.Context, containerName string, tail string) (string, error) {
	return "", fmt.Errorf("GetContainerLogs not yet implemented for Incus")
}

func (c *Client) EnsureImage(ctx context.Context, imageRef string) (string, error) {
	c.logger.Debug(c.logTag, "Ensuring image %s", imageRef)
	
	convertedAlias := "instant-bosh-system"
	
	aliases, err := c.cli.GetImageAliases()
	if err != nil {
		return "", fmt.Errorf("getting image aliases: %w", err)
	}
	
	for _, alias := range aliases {
		if alias.Name == convertedAlias {
			c.logger.Debug(c.logTag, "Using existing converted image: %s", convertedAlias)
			return alias.Target, nil
		}
	}
	
	c.logger.Info(c.logTag, "Image not found locally, pulling and converting from %s", imageRef)
	
	return "", fmt.Errorf("OCI image conversion not yet implemented")
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

func (w *incusAPIWrapper) GetInstanceFile(instanceName string, filePath string) (io.ReadCloser, *api.InstanceFileResponse, error) {
	return w.server.GetInstanceFile(instanceName, filePath)
}

func (w *incusAPIWrapper) DeleteInstanceFile(instanceName string, filePath string) error {
	return w.server.DeleteInstanceFile(instanceName, filePath)
}

func (w *incusAPIWrapper) CreateInstanceFromImage(source incus.ImageServer, image api.Image, req api.InstancesPost) (incus.RemoteOperation, error) {
	return w.server.CreateInstanceFromImage(source, image, req)
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

func (w *incusAPIWrapper) DeleteNetwork(name string) (incus.Operation, error) {
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

func (w *incusAPIWrapper) GetInstanceLogs(instanceName string) ([]string, error) {
	return w.server.GetInstanceLogs(instanceName)
}

func (w *incusAPIWrapper) GetInstanceLogfile(instanceName string, filename string) (io.ReadCloser, error) {
	return w.server.GetInstanceLogfile(instanceName, filename)
}

func (w *incusAPIWrapper) Disconnect() {
	w.server.Disconnect()
}
