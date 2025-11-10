package commands

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"github.com/rkoster/instant-bosh/internal/logparser"
)

// LogWriter wraps an io.Writer and parses log lines before writing
type LogWriter struct {
	writer          io.Writer
	colorize        bool
	buffer          bytes.Buffer
	componentFilter map[string]bool
	hasFilter       bool
}

func NewLogWriter(writer io.Writer, colorize bool, components []string) *LogWriter {
	lw := &LogWriter{
		writer:   writer,
		colorize: colorize,
	}

	if len(components) > 0 {
		lw.hasFilter = true
		lw.componentFilter = make(map[string]bool)
		for _, comp := range components {
			lw.componentFilter[comp] = true
		}
	}

	return lw
}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	// Add to buffer
	lw.buffer.Write(p)

	// Process complete lines
	for {
		line, err := lw.buffer.ReadString('\n')
		if err != nil {
			// No complete line yet, put it back
			if line != "" {
				lw.buffer.WriteString(line)
			}
			break
		}

		// Remove trailing newline for parsing
		line = strings.TrimSuffix(line, "\n")

		// Parse the log line
		logLine, parseErr := logparser.ParseLogLine(line)
		if parseErr != nil {
			// If parsing fails, write the original line
			lw.writer.Write([]byte(line + "\n"))
			continue
		}

		// Apply component filter if set
		if lw.hasFilter && logLine.Component != "" {
			if !lw.componentFilter[logLine.Component] {
				// Skip this line - component not in filter
				continue
			}
		}

		// Format and write the parsed line
		formatted := logLine.FormatLogLine(lw.colorize) + "\n"
		lw.writer.Write([]byte(formatted))
	}

	return len(p), nil
}

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

	stdoutWriter := NewLogWriter(os.Stdout, colorize, components)
	stderrWriter := NewLogWriter(os.Stderr, colorize, components)

	return dockerClient.FollowContainerLogs(ctx, docker.ContainerName, follow, tail, stdoutWriter, stderrWriter)
}

// isTerminal checks if the file descriptor is a terminal
func isTerminal(fd uintptr) bool {
	// Simple check - in a real implementation you might want to use
	// a library like golang.org/x/term for cross-platform support
	return os.Getenv("TERM") != ""
}
