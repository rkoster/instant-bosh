package commands

import (
	"context"
	"os"
	"strings"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/logparser"
	"github.com/rkoster/instant-bosh/internal/logwriter"
	"golang.org/x/term"
)

func LogsAction(ui boshui.UI, logger boshlog.Logger, listComponents bool, components []string, follow bool, tail string) error {
	ctx := context.Background()
	dockerClient, err := docker.NewClient(logger)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}

	if !running {
		ui.PrintLinef("instant-bosh is not running")
		return nil
	}

	// If listing components, fetch logs and extract components
	if listComponents {
		// Get all logs to find components from the beginning
		logContent, err := dockerClient.GetContainerLogs(ctx, docker.ContainerName, "all")
		if err != nil {
			return err
		}

		// Take only the first 2000 lines to determine components
		lines := strings.Split(logContent, "\n")
		if len(lines) > 2000 {
			lines = lines[:2000]
		}
		firstLines := strings.Join(lines, "\n")

		components := logparser.ExtractComponents(firstLines)
		ui.PrintLinef("Available components:")
		for _, component := range components {
			ui.PrintLinef("  %s", component)
		}
		return nil
	}

	// Use a log writer that parses and formats logs
	// Check if stdout is a terminal for colorization
	colorize := isTerminal(os.Stdout.Fd())

	config := logwriter.Config{
		Colorize:   colorize,
		Components: components,
	}

	stdoutWriter := logwriter.New(os.Stdout, config)
	stderrWriter := logwriter.New(os.Stderr, config)

	return dockerClient.FollowContainerLogs(ctx, docker.ContainerName, follow, tail, stdoutWriter, stderrWriter)
}

// isTerminal checks if the file descriptor is a terminal
func isTerminal(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}

// StreamMainComponentLogs streams logs from the main component, showing only messages
// This is used during startup to show progress without cluttering the output
func StreamMainComponentLogs(ctx context.Context, dockerClient *docker.Client, ui boshui.UI) error {
	// Use the UI to write messages directly - use same writer for both stdout and stderr
	// since we want all logs from the main component
	config := logwriter.Config{
		MessageOnly: true,
		Components:  []string{"main"},
	}
	writer := logwriter.New(&uiWriter{ui: ui}, config)

	// Follow logs from the container starting from the beginning
	// tail="all" gets all existing logs and then follows for new ones
	// Use the same writer for both stdout and stderr streams
	return dockerClient.FollowContainerLogs(ctx, docker.ContainerName, true, "all", writer, writer)
}

// uiWriter wraps boshui.UI to implement io.Writer
type uiWriter struct {
	ui boshui.UI
}

func (w *uiWriter) Write(p []byte) (n int, err error) {
	// Remove trailing newline if present, since PrintLinef adds one
	msg := strings.TrimSuffix(string(p), "\n")
	if msg != "" {
		w.ui.PrintLinef("%s", msg)
	}
	return len(p), nil
}
