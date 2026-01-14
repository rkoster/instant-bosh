package cpi

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os/exec"
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
	return i.client.GetContainerLogs(ctx, incus.ContainerName, tail)
}

func (i *IncusCPI) FollowLogs(ctx context.Context, stdout, stderr io.Writer) error {
	return i.FollowLogsWithOptions(ctx, true, "all", stdout, stderr)
}

func (i *IncusCPI) FollowLogsWithOptions(ctx context.Context, follow bool, tail string, stdout, stderr io.Writer) error {
	// For Incus, we'll exec into the container and tail log files
	fullName := fmt.Sprintf("%s:%s", i.client.GetRemote(), incus.ContainerName)

	var args []string
	if follow {
		// Use tail -f to follow the pre-start log
		args = []string{"exec", fullName, "--", "tail", "-f", "/var/log/bosh/pre-start.log"}
	} else {
		// Just show the log without following
		if tail != "all" && tail != "" {
			args = []string{"exec", fullName, "--", "tail", "-n", tail, "/var/log/bosh/pre-start.log"}
		} else {
			args = []string{"exec", fullName, "--", "cat", "/var/log/bosh/pre-start.log"}
		}
	}

	cmd := exec.CommandContext(ctx, "incus", args...)

	// Combine stdout and stderr since we want all output
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		// If pre-start log doesn't exist, try console log as fallback
		if !follow {
			args = []string{"console", fullName, "--show-log"}
			cmd = exec.CommandContext(ctx, "incus", args...)
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("getting container logs: %w", err)
			}
			return nil
		}
		return fmt.Errorf("following container logs: %w", err)
	}

	return nil
}

func (i *IncusCPI) WaitForReady(ctx context.Context, maxWait time.Duration) error {
	// Create HTTP client that skips TLS verification (BOSH uses self-signed certs)
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	boshURL := fmt.Sprintf("https://%s:%s/info", i.GetContainerIP(), i.GetDirectorPort())
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		// First check if container is still running
		running, err := i.IsRunning(ctx)
		if err != nil {
			return err
		}
		if !running {
			return fmt.Errorf("container stopped unexpectedly")
		}

		// Try to connect to BOSH /info endpoint
		resp, err := httpClient.Get(boshURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for BOSH to start after %v", maxWait)
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

func (i *IncusCPI) HasDirectNetworkAccess() bool {
	// Incus containers have direct network access via static routing
	// No jumpbox proxy needed
	return true
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
