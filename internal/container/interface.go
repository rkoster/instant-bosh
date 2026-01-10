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
	
	// Close releases any resources held by the client
	Close() error
}
