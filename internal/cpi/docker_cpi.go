package cpi

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/rkoster/instant-bosh/internal/docker"
)

var (
	dockerCloudConfigYAML = []byte(`azs:
- name: z1
- name: z2
- name: z3

vm_types:
- name: default
- name: minimal
- name: small
- name: small-highmem
- name: medium
- name: compilation

disk_types:
- name: default
  disk_size: 10240
- name: 10GB
  disk_size: 10240
- name: 100GB
  disk_size: 102400

networks:
- name: default
  type: manual
  subnets:
  - azs: [z1, z2, z3]
    range: 10.245.0.0/16
    dns: [8.8.8.8]
    reserved: [10.245.0.1-10.245.0.20]
    gateway: 10.245.0.1
    static: [10.245.0.21-10.245.0.100]
    cloud_properties:
      name: instant-bosh

vm_extensions:
- name: 50GB_ephemeral_disk
- name: 100GB_ephemeral_disk
- name: diego-ssh-proxy-network-properties
- name: cf-router-network-properties
- name: cf-tcp-router-network-properties

compilation:
  workers: 4
  az: z1
  reuse_compilation_vms: true
  vm_type: compilation
  network: default
`)
)

type DockerCPI struct {
	client *docker.Client
}

func NewDockerCPI(client *docker.Client) *DockerCPI {
	return &DockerCPI{client: client}
}

func (d *DockerCPI) GetDockerClient() *docker.Client {
	return d.client
}

func (d *DockerCPI) Start(ctx context.Context) error {
	return d.client.StartContainer(ctx)
}

func (d *DockerCPI) Stop(ctx context.Context) error {
	return d.client.StopContainer(ctx)
}

func (d *DockerCPI) Destroy(ctx context.Context) error {
	containers, err := d.client.GetContainersOnNetwork(ctx)
	if err == nil {
		for _, containerName := range containers {
			if containerName != docker.ContainerName {
				_ = d.client.RemoveContainer(ctx, containerName)
			}
		}
	}

	if err := d.client.RemoveContainer(ctx, docker.ContainerName); err != nil {
		return err
	}

	if err := d.client.RemoveVolume(ctx, docker.VolumeStore); err != nil {
		return err
	}

	if err := d.client.RemoveVolume(ctx, docker.VolumeData); err != nil {
		return err
	}

	if err := d.client.RemoveNetwork(ctx); err != nil {
		return err
	}

	return nil
}

func (d *DockerCPI) RemoveContainer(ctx context.Context) error {
	// Remove container only, preserve volumes for restart
	return d.client.RemoveContainer(ctx, docker.ContainerName)
}

func (d *DockerCPI) IsRunning(ctx context.Context) (bool, error) {
	return d.client.IsContainerRunning(ctx)
}

func (d *DockerCPI) Exists(ctx context.Context) (bool, error) {
	return d.client.ContainerExists(ctx)
}

func (d *DockerCPI) ResourcesExist(ctx context.Context) (bool, error) {
	storeExists, err := d.client.VolumeExists(ctx, docker.VolumeStore)
	if err != nil {
		return false, err
	}
	if storeExists {
		return true, nil
	}

	dataExists, err := d.client.VolumeExists(ctx, docker.VolumeData)
	if err != nil {
		return false, err
	}
	if dataExists {
		return true, nil
	}

	networkExists, err := d.client.NetworkExists(ctx, docker.NetworkName)
	if err != nil {
		return false, err
	}
	return networkExists, nil
}

func (d *DockerCPI) ExecCommand(ctx context.Context, containerName string, command []string) (string, error) {
	return d.client.ExecCommand(ctx, containerName, command)
}

func (d *DockerCPI) GetLogs(ctx context.Context, tail string) (string, error) {
	return d.client.GetContainerLogs(ctx, docker.ContainerName, tail)
}

func (d *DockerCPI) FollowLogs(ctx context.Context, stdout, stderr io.Writer) error {
	return d.client.FollowContainerLogs(ctx, docker.ContainerName, true, "all", stdout, stderr)
}

func (d *DockerCPI) FollowLogsWithOptions(ctx context.Context, follow bool, tail string, stdout, stderr io.Writer) error {
	return d.client.FollowContainerLogs(ctx, docker.ContainerName, follow, tail, stdout, stderr)
}

func (d *DockerCPI) WaitForReady(ctx context.Context, maxWait time.Duration) error {
	return d.client.WaitForBoshReady(ctx, maxWait)
}

func (d *DockerCPI) GetContainerName() string {
	return docker.ContainerName
}

func (d *DockerCPI) GetHostAddress() string {
	return d.client.GetHostAddress()
}

func (d *DockerCPI) GetCloudConfigBytes() []byte {
	return dockerCloudConfigYAML
}

func (d *DockerCPI) GetContainerIP() string {
	return docker.ContainerIP
}

func (d *DockerCPI) GetDirectorPort() string {
	return docker.DirectorPort
}

func (d *DockerCPI) GetSSHPort() string {
	return docker.SSHPort
}

func (d *DockerCPI) HasDirectNetworkAccess() bool {
	// Docker requires SOCKS5 proxy through jumpbox
	// since we access via localhost port forwarding
	return false
}

func (d *DockerCPI) GetContainersOnNetwork(ctx context.Context) ([]ContainerInfo, error) {
	dockerContainers, err := d.client.GetContainersOnNetworkDetailed(ctx)
	if err != nil {
		return nil, err
	}

	cpiContainers := make([]ContainerInfo, len(dockerContainers))
	for i, dc := range dockerContainers {
		cpiContainers[i] = ContainerInfo{
			Name:    dc.Name,
			Created: dc.Created,
			Network: dc.Network,
		}
	}
	return cpiContainers, nil
}

func (d *DockerCPI) EnsurePrerequisites(ctx context.Context) error {
	storeExists, err := d.client.VolumeExists(ctx, docker.VolumeStore)
	if err != nil {
		return fmt.Errorf("checking volume %s: %w", docker.VolumeStore, err)
	}
	dataExists, err := d.client.VolumeExists(ctx, docker.VolumeData)
	if err != nil {
		return fmt.Errorf("checking volume %s: %w", docker.VolumeData, err)
	}

	if !storeExists {
		if err := d.client.CreateVolume(ctx, docker.VolumeStore); err != nil {
			return fmt.Errorf("creating volume %s: %w", docker.VolumeStore, err)
		}
	}
	if !dataExists {
		if err := d.client.CreateVolume(ctx, docker.VolumeData); err != nil {
			return fmt.Errorf("creating volume %s: %w", docker.VolumeData, err)
		}
	}

	networkExists, err := d.client.NetworkExists(ctx, docker.NetworkName)
	if err != nil {
		return fmt.Errorf("checking network: %w", err)
	}

	if !networkExists {
		if err := d.client.CreateNetwork(ctx); err != nil {
			return fmt.Errorf("creating network: %w", err)
		}
	}

	return nil
}

func (d *DockerCPI) Close() error {
	return d.client.Close()
}
