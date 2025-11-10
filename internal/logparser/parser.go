package logparser

import (
	"regexp"
	"strings"
	"time"
)

// LogLine represents a parsed log line
type LogLine struct {
	Component string
	Timestamp time.Time
	Level     string
	Message   string
	Raw       string
}

var (
	// Pattern: [component] timestamp level - message
	logLineRegex = regexp.MustCompile(`^\[([^\]]+)\]\s+(\S+)\s+(\S+)\s+-\s+(.*)$`)
)

// ParseLogLine parses a Docker log line from instant-bosh
// Expected format: [component] timestamp level - message
// Example: [director/access] 2025-11-10T14:35:24.468092247Z INFO - 127.0.0.1 - - [10/Nov/2025:14:35:24 +0000] "GET /info HTTP/1.1" 200 384 "-" "Ruby" 0.003 0.002 .
func ParseLogLine(line string) (*LogLine, error) {
	matches := logLineRegex.FindStringSubmatch(line)
	if matches == nil {
		// Return unparsed line if it doesn't match the expected format
		return &LogLine{
			Raw: line,
		}, nil
	}

	component := matches[1]
	timestampStr := matches[2]
	level := matches[3]
	message := matches[4]

	// Parse timestamp
	timestamp, err := time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		// If timestamp parsing fails, return with raw timestamp
		return &LogLine{
			Component: component,
			Level:     level,
			Message:   message,
			Raw:       line,
		}, nil
	}

	return &LogLine{
		Component: component,
		Timestamp: timestamp,
		Level:     level,
		Message:   message,
		Raw:       line,
	}, nil
}

// FormatLogLine formats a parsed log line with optional color coding
func (l *LogLine) FormatLogLine(colorize bool) string {
	if l.Component == "" && l.Level == "" {
		// Unparsed line, return as-is
		return l.Raw
	}

	var sb strings.Builder

	if colorize {
		// Component in cyan
		sb.WriteString("\033[36m[")
		sb.WriteString(l.Component)
		sb.WriteString("]\033[0m ")

		// Timestamp in gray
		if !l.Timestamp.IsZero() {
			sb.WriteString("\033[90m")
			sb.WriteString(l.Timestamp.Format("15:04:05.000"))
			sb.WriteString("\033[0m ")
		}

		// Level with color
		switch l.Level {
		case "ERROR":
			sb.WriteString("\033[31mERROR\033[0m")
		case "WARN", "WARNING":
			sb.WriteString("\033[33mWARN\033[0m")
		case "INFO":
			sb.WriteString("\033[32mINFO\033[0m")
		case "DEBUG":
			sb.WriteString("\033[34mDEBUG\033[0m")
		default:
			sb.WriteString(l.Level)
		}

		sb.WriteString(" - ")
		sb.WriteString(l.Message)
	} else {
		// No color
		sb.WriteString("[")
		sb.WriteString(l.Component)
		sb.WriteString("] ")

		if !l.Timestamp.IsZero() {
			sb.WriteString(l.Timestamp.Format("15:04:05.000"))
			sb.WriteString(" ")
		}

		sb.WriteString(l.Level)
		sb.WriteString(" - ")
		sb.WriteString(l.Message)
	}

	return sb.String()
}

// ExtractComponents extracts unique component names from log content
func ExtractComponents(logContent string) []string {
	componentSet := make(map[string]bool)
	lines := strings.Split(logContent, "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		logLine, err := ParseLogLine(line)
		if err == nil && logLine.Component != "" {
			componentSet[logLine.Component] = true
		}
	}

	// Convert map to sorted slice
	components := make([]string, 0, len(componentSet))
	for component := range componentSet {
		components = append(components, component)
	}

	// Sort for consistent output
	// Using a simple bubble sort to avoid importing sort package
	for i := 0; i < len(components); i++ {
		for j := i + 1; j < len(components); j++ {
			if components[i] > components[j] {
				components[i], components[j] = components[j], components[i]
			}
		}
	}

	return components
}
