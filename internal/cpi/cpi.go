package cpi

import (
	"context"
	"io"
	"time"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
)

type ContainerInfo struct {
	Name    string
	Created time.Time
	Network string
}

// ImageInfo contains information about the container's source OCI image
type ImageInfo struct {
	// Ref is the full image reference (e.g., "ghcr.io/rkoster/instant-bosh:latest")
	Ref string
	// Digest is the image digest (e.g., "sha256:abc123...")
	// May be empty if not available locally (e.g., for Incus OCI images)
	Digest string
}

// CPI defines a unified interface for Cloud Provider Implementations (CPIs).
// Both Docker and Incus clients implement this interface, enabling mode-agnostic
// command implementations and easy addition of new CPIs in the future.
//
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . CPI
type CPI interface {
	// Lifecycle operations
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Destroy(ctx context.Context) error
	RemoveContainer(ctx context.Context) error // Removes container only, preserves volumes

	// Status operations
	IsRunning(ctx context.Context) (bool, error)
	Exists(ctx context.Context) (bool, error)
	ResourcesExist(ctx context.Context) (bool, error)

	// Command execution
	ExecCommand(ctx context.Context, containerName string, command []string) (string, error)

	// Logs
	GetLogs(ctx context.Context, tail string) (string, error)
	FollowLogs(ctx context.Context, stdout, stderr io.Writer) error
	FollowLogsWithOptions(ctx context.Context, follow bool, tail string, stdout, stderr io.Writer) error

	// Readiness
	WaitForReady(ctx context.Context, maxWait time.Duration) error

	// Configuration
	GetContainerName() string
	GetHostAddress() string
	GetCloudConfigBytes() []byte
	GetContainerIP() string
	GetDirectorPort() string
	GetSSHPort() string

	// Network access method
	// Returns true if direct network access is available to the container
	// Returns false if BOSH_ALL_PROXY (jumpbox) is needed
	HasDirectNetworkAccess() bool

	// Container management
	GetContainersOnNetwork(ctx context.Context) ([]ContainerInfo, error)

	// Resource management
	EnsurePrerequisites(ctx context.Context) error
	Close() error

	// Stemcell management
	// UploadStemcell uploads a stemcell to the BOSH director
	// For Docker CPI: creates a light stemcell from container image
	// For Incus CPI: downloads full stemcell from bosh.io and uploads via URL
	UploadStemcell(ctx context.Context, directorClient boshdir.Director, os, version string) error

	// Image management
	// GetCurrentImageInfo returns information about the OCI image the running container was created from.
	// Returns an error if the container doesn't exist.
	GetCurrentImageInfo(ctx context.Context) (ImageInfo, error)

	// GetTargetImageRef returns the OCI image reference that would be used for new containers.
	GetTargetImageRef() string

	// SetResolvedImage sets the resolved (digest-pinned) image reference for container creation.
	// This should be called before Start() to ensure the container is created with a digest-pinned
	// image reference, enabling accurate upgrade comparisons.
	//
	// pinnedRef: Digest-pinned reference (e.g., "ghcr.io/rkoster/instant-bosh@sha256:abc...")
	// digest:    The digest (e.g., "sha256:abc...")
	SetResolvedImage(pinnedRef, digest string)
}

type StartOptions struct {
	SkipUpdate         bool
	SkipStemcellUpload bool
	CustomImage        string
}
