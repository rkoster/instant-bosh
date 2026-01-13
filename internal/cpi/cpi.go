package cpi

import (
	"context"
	"io"
	"time"
)

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

	// Status operations
	IsRunning(ctx context.Context) (bool, error)
	Exists(ctx context.Context) (bool, error)

	// Command execution
	ExecCommand(ctx context.Context, command []string) (string, error)

	// Logs
	GetLogs(ctx context.Context, tail string) (string, error)
	FollowLogs(ctx context.Context, stdout, stderr io.Writer) error

	// Readiness
	WaitForReady(ctx context.Context, maxWait time.Duration) error

	// Configuration
	GetContainerName() string
	GetHostAddress() string
	GetCloudConfigBytes() []byte

	// Resource management
	EnsurePrerequisites(ctx context.Context) error
	Close() error
}

// StartOptions contains configuration for starting a BOSH director
type StartOptions struct {
	SkipUpdate         bool
	SkipStemcellUpload bool
	CustomImage        string
}
