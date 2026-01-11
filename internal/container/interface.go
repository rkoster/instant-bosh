package container

import "context"

// Client is a generic interface for container operations
// that works with both Docker and Incus containers.
// This interface allows the director package to work with
// either container runtime without tight coupling.
//
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . Client
type Client interface {
	// ExecCommand executes a command in the container and returns its output
	ExecCommand(ctx context.Context, containerName string, cmd []string) (string, error)

	// GetHostAddress returns the address of the host where the container is running.
	// For Docker, this returns "127.0.0.1" since ports are forwarded locally.
	// For Incus on a remote server, this returns the Incus server's IP address
	// where proxy devices forward ports to the container.
	GetHostAddress() string

	// Close releases any resources held by the client
	Close() error
}
