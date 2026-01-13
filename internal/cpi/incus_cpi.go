package cpi

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/rkoster/instant-bosh/internal/incus"
)

var (
	incusCloudConfigYAML = []byte(`azs:
- name: z1
- name: z2
- name: z3

vm_types:
- name: default
  cloud_properties:
    instance_type: c2-m4
    ephemeral_disk: 10240

disk_types:
- name: default
  disk_size: 10240

networks:
- name: default
  type: manual
  subnets:
  - azs: [z1, z2, z3]
    range: 10.246.0.0/16
    dns: [8.8.8.8]
    gateway: 10.246.0.1
    reserved: [10.246.0.1-10.246.0.20]
    static: [10.246.0.21-10.246.0.100]
    cloud_properties:
      name: instant-bosh-incus

compilation:
  workers: 4
  az: z1
  reuse_compilation_vms: true
  vm_type: default
  network: default
`)
)

type IncusCPI struct {
	client *incus.Client
}

func NewIncusCPI(client *incus.Client) *IncusCPI {
	return &IncusCPI{client: client}
}

func (i *IncusCPI) Start(ctx context.Context) error {
	return i.client.StartContainer(ctx)
}

func (i *IncusCPI) Stop(ctx context.Context) error {
	return i.client.StopContainer(ctx)
}

func (i *IncusCPI) Destroy(ctx context.Context) error {
	return i.client.RemoveContainer(ctx, incus.ContainerName)
}

func (i *IncusCPI) IsRunning(ctx context.Context) (bool, error) {
	return i.client.IsContainerRunning(ctx)
}

func (i *IncusCPI) Exists(ctx context.Context) (bool, error) {
	return i.client.ContainerExists(ctx)
}

func (i *IncusCPI) ExecCommand(ctx context.Context, containerName string, command []string) (string, error) {
	return i.client.ExecCommand(ctx, containerName, command)
}

func (i *IncusCPI) GetLogs(ctx context.Context, tail string) (string, error) {
	return "", fmt.Errorf("logs not yet implemented for Incus CPI")
}

func (i *IncusCPI) FollowLogs(ctx context.Context, stdout, stderr io.Writer) error {
	return fmt.Errorf("follow logs not yet implemented for Incus CPI")
}

func (i *IncusCPI) FollowLogsWithOptions(ctx context.Context, follow bool, tail string, stdout, stderr io.Writer) error {
	return fmt.Errorf("follow logs with options not yet implemented for Incus CPI")
}

func (i *IncusCPI) WaitForReady(ctx context.Context, maxWait time.Duration) error {
	timer := time.NewTimer(maxWait)
	defer timer.Stop()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("timeout waiting for Incus container to be ready after %v", maxWait)
		case <-ticker.C:
			running, err := i.IsRunning(ctx)
			if err != nil {
				continue
			}
			if running {
				return nil
			}
		}
	}
}

func (i *IncusCPI) GetContainerName() string {
	return incus.ContainerName
}

func (i *IncusCPI) GetHostAddress() string {
	return i.client.GetHostAddress()
}

func (i *IncusCPI) GetCloudConfigBytes() []byte {
	return incusCloudConfigYAML
}

func (i *IncusCPI) GetContainerIP() string {
	return incus.ContainerIP
}

func (i *IncusCPI) GetDirectorPort() string {
	return incus.DirectorPort
}

func (i *IncusCPI) GetSSHPort() string {
	return incus.SSHPort
}

func (i *IncusCPI) GetContainersOnNetwork(ctx context.Context) ([]ContainerInfo, error) {
	return nil, fmt.Errorf("GetContainersOnNetwork not yet implemented for Incus CPI")
}

func (i *IncusCPI) EnsurePrerequisites(ctx context.Context) error {
	networkExists, err := i.client.NetworkExists(ctx, i.client.NetworkName())
	if err != nil {
		return fmt.Errorf("checking network: %w", err)
	}

	if !networkExists {
		if err := i.client.CreateNetwork(ctx); err != nil {
			return fmt.Errorf("creating network: %w", err)
		}
	}

	return nil
}

func (i *IncusCPI) Close() error {
	return i.client.Close()
}
