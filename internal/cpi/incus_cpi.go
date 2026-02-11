package cpi

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
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
	// Remove container first
	if err := i.client.RemoveContainer(ctx, incus.ContainerName); err != nil {
		return err
	}

	// Remove volumes created for the instance.
	if err := i.client.RemoveVolumes(ctx); err != nil {
		return fmt.Errorf("removing volumes: %w", err)
	}

	return nil
}

func (i *IncusCPI) IsRunning(ctx context.Context) (bool, error) {
	return i.client.IsContainerRunning(ctx)
}

func (i *IncusCPI) Exists(ctx context.Context) (bool, error) {
	return i.client.ContainerExists(ctx)
}

func (i *IncusCPI) ResourcesExist(ctx context.Context) (bool, error) {
	// For Incus, the main resource is the network
	// The container is stateful and doesn't use separate volumes like Docker
	networkExists, err := i.client.NetworkExists(ctx, i.client.NetworkName())
	if err != nil {
		return false, err
	}
	return networkExists, nil
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
	// For Incus, we use the console log which captures the entrypoint binary's stdout/stderr
	// This gives us structured output with process tags like [process], [director/sync_dns.stdout], etc.
	fullName := fmt.Sprintf("%s:%s", i.client.GetRemote(), incus.ContainerName)

	if follow {
		// For follow mode, we poll the console log since Incus doesn't support streaming the console log
		// We track the byte offset and only show new content on each poll

		// Get the current log size to use as starting offset
		// If tail is specified and not "0", show that many lines first
		cmd := exec.CommandContext(ctx, "incus", "console", fullName, "--show-log")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("getting container console log: %w", err)
		}

		lastOffset := 0
		if tail == "0" {
			// Skip all historical logs, only show new ones from now on
			lastOffset = len(output)
		} else if tail != "all" && tail != "" {
			// Show last N lines, then continue following
			lines := strings.Split(string(output), "\n")
			tailNum := -1
			if _, err := fmt.Sscanf(tail, "%d", &tailNum); err == nil {
				if tailNum > 0 && tailNum < len(lines) {
					// Calculate offset to show last N lines
					recentLines := lines[len(lines)-tailNum:]
					recentOutput := strings.Join(recentLines, "\n")
					if _, err := stdout.Write([]byte(recentOutput)); err != nil {
						return fmt.Errorf("writing to stdout: %w", err)
					}
					lastOffset = len(output)
				} else {
					// Show all if tail is larger than available lines or invalid
					if _, err := stdout.Write(output); err != nil {
						return fmt.Errorf("writing to stdout: %w", err)
					}
					lastOffset = len(output)
				}
			} else {
				// Invalid tail format, show all
				if _, err := stdout.Write(output); err != nil {
					return fmt.Errorf("writing to stdout: %w", err)
				}
				lastOffset = len(output)
			}
		} else {
			// tail == "all", show all historical logs first
			if _, err := stdout.Write(output); err != nil {
				return fmt.Errorf("writing to stdout: %w", err)
			}
			lastOffset = len(output)
		}

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				// Get the full console log
				cmd := exec.CommandContext(ctx, "incus", "console", fullName, "--show-log")
				output, err := cmd.CombinedOutput()
				if err != nil {
					// Container might be stopped, return error
					return fmt.Errorf("getting container console log: %w", err)
				}

				// Only show new content since last poll
				if len(output) > lastOffset {
					newContent := output[lastOffset:]
					if _, err := stdout.Write(newContent); err != nil {
						return fmt.Errorf("writing to stdout: %w", err)
					}
					lastOffset = len(output)
				}
			}
		}
	}

	// For non-follow mode, just show the console log
	args := []string{"console", fullName, "--show-log"}

	// If tail is specified, we'll pipe through tail
	if tail != "all" && tail != "" {
		cmd := exec.CommandContext(ctx, "incus", args...)
		cmdOut, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("creating stdout pipe: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("starting console command: %w", err)
		}

		// Tail the output
		tailCmd := exec.CommandContext(ctx, "tail", "-n", tail)
		tailCmd.Stdin = cmdOut
		tailCmd.Stdout = stdout
		tailCmd.Stderr = stderr

		if err := tailCmd.Run(); err != nil {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return fmt.Errorf("tailing output: %w", err)
		}

		return cmd.Wait()
	}

	// Show all logs
	cmd := exec.CommandContext(ctx, "incus", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("getting container console log: %w", err)
	}

	return nil
}

func (i *IncusCPI) WaitForReady(ctx context.Context, maxWait time.Duration) error {
	// Create HTTP client that skips TLS verification
	// This is intentional and safe for instant-bosh: the director uses self-signed certificates
	// and we're connecting to a local container, not a production system
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	boshURL := fmt.Sprintf("https://%s:%s/info", i.GetContainerIP(), i.GetDirectorPort())
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

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
	if err := i.client.EnsureVolumes(ctx); err != nil {
		return fmt.Errorf("ensuring volumes: %w", err)
	}

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
