package cpi

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/incus"
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

func (i *IncusCPI) ExecCommand(ctx context.Context, command []string) (string, error) {
	return i.client.ExecCommand(ctx, incus.ContainerName, command)
}

func (i *IncusCPI) GetLogs(ctx context.Context, tail string) (string, error) {
	return "", fmt.Errorf("logs not yet implemented for Incus CPI")
}

func (i *IncusCPI) FollowLogs(ctx context.Context, stdout, stderr io.Writer) error {
	return fmt.Errorf("follow logs not yet implemented for Incus CPI")
}

func (i *IncusCPI) WaitForReady(ctx context.Context, maxWait time.Duration) error {
	time.Sleep(60 * time.Second)
	return nil
}

func (i *IncusCPI) GetContainerName() string {
	return incus.ContainerName
}

func (i *IncusCPI) GetHostAddress() string {
	return i.client.GetHostAddress()
}

func (i *IncusCPI) GetCloudConfigBytes() []byte {
	return commands.GetIncusCloudConfigBytes()
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
