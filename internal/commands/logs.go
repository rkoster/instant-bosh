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
	"golang.org/x/term"
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
			if _, writeErr := lw.writer.Write([]byte(line + "\n")); writeErr != nil {
				return len(p), writeErr
			}
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
		if _, writeErr := lw.writer.Write([]byte(formatted)); writeErr != nil {
			return len(p), writeErr
		}
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
	return term.IsTerminal(int(fd))
}

// MessageOnlyLogWriter writes only the message portion of parsed log lines
type MessageOnlyLogWriter struct {
	writer          io.Writer
	buffer          bytes.Buffer
	componentFilter map[string]bool
	hasFilter       bool
}

func NewMessageOnlyLogWriter(writer io.Writer, components []string) *MessageOnlyLogWriter {
	lw := &MessageOnlyLogWriter{
		writer: writer,
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

func (lw *MessageOnlyLogWriter) Write(p []byte) (n int, err error) {
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
			// If parsing fails, skip this line
			continue
		}

		// Apply component filter if set
		if lw.hasFilter && logLine.Component != "" {
			if !lw.componentFilter[logLine.Component] {
				// Skip this line - component not in filter
				continue
			}
		}

		// Write only the message
		if logLine.Message != "" {
			if _, writeErr := lw.writer.Write([]byte(logLine.Message + "\n")); writeErr != nil {
				return len(p), writeErr
			}
		}
	}

	return len(p), nil
}

// StreamMainComponentLogs streams logs from the main component, showing only messages
// This is used during startup to show progress without cluttering the output
func StreamMainComponentLogs(ctx context.Context, dockerClient *docker.Client, ui boshui.UI) error {
	// Use the UI to write messages directly - use same writer for both stdout and stderr
	// since we want all logs from the main component
	writer := NewMessageOnlyLogWriter(&uiWriter{ui: ui}, []string{"main"})

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
