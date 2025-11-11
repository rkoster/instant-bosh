package logwriter

import (
	"bytes"
	"io"
	"strings"

	"github.com/rkoster/instant-bosh/internal/logparser"
)

// Config holds configuration options for the LogWriter
type Config struct {
	// Colorize enables colorized output
	Colorize bool
	// MessageOnly when true, only writes the message portion of log lines
	MessageOnly bool
	// Components is a list of component names to filter by (empty means all)
	Components []string
}

// Writer wraps an io.Writer and parses/formats log lines before writing
type Writer struct {
	writer          io.Writer
	config          Config
	buffer          bytes.Buffer
	componentFilter map[string]bool
	hasFilter       bool
}

// New creates a new LogWriter with the given configuration
func New(writer io.Writer, config Config) *Writer {
	lw := &Writer{
		writer: writer,
		config: config,
	}

	if len(config.Components) > 0 {
		lw.hasFilter = true
		lw.componentFilter = make(map[string]bool)
		for _, comp := range config.Components {
			lw.componentFilter[comp] = true
		}
	}

	return lw
}

func (lw *Writer) Write(p []byte) (n int, err error) {
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
		logLine := logparser.ParseLogLine(line)

		// Apply component filter if set
		if lw.hasFilter && logLine.Component != "" {
			if !lw.componentFilter[logLine.Component] {
				// Skip this line - component not in filter
				continue
			}
		}

		// Format and write the line based on config
		var output string
		if lw.config.MessageOnly {
			// Write only the message portion
			if logLine.Message != "" {
				output = logLine.Message + "\n"
			} else {
				// Skip lines with no message
				continue
			}
		} else {
			// Write the full formatted line
			output = logLine.FormatLogLine(lw.config.Colorize) + "\n"
		}

		if _, writeErr := lw.writer.Write([]byte(output)); writeErr != nil {
			return len(p), writeErr
		}
	}

	return len(p), nil
}
