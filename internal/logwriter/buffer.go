package logwriter

import (
	"bytes"
	"strings"
	"sync"

	"github.com/rkoster/instant-bosh/internal/logparser"
)

// LogBuffer is a thread-safe ring buffer that captures log lines.
// It stores raw log lines and can format them on demand using the logparser.
type LogBuffer struct {
	mu       sync.Mutex
	lines    []string
	maxLines int
	partial  bytes.Buffer // holds incomplete line data
}

// NewLogBuffer creates a new LogBuffer that stores up to maxLines lines.
func NewLogBuffer(maxLines int) *LogBuffer {
	return &LogBuffer{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
	}
}

// Write implements io.Writer. It buffers incoming data and extracts complete lines.
func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Add to partial buffer
	lb.partial.Write(p)

	// Extract complete lines
	for {
		line, err := lb.partial.ReadString('\n')
		if err != nil {
			// No complete line yet, put it back
			if line != "" {
				lb.partial.WriteString(line)
			}
			break
		}

		// Remove trailing newline
		line = strings.TrimSuffix(line, "\n")

		// Add to ring buffer
		lb.addLine(line)
	}

	return len(p), nil
}

// addLine adds a line to the ring buffer, evicting the oldest if at capacity.
// Must be called with lock held.
func (lb *LogBuffer) addLine(line string) {
	if len(lb.lines) >= lb.maxLines {
		// Shift all elements left by 1 (evict oldest)
		copy(lb.lines, lb.lines[1:])
		lb.lines = lb.lines[:lb.maxLines-1]
	}
	lb.lines = append(lb.lines, line)
}

// Lines returns a copy of all buffered lines (raw, unparsed).
func (lb *LogBuffer) Lines() []string {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	result := make([]string, len(lb.lines))
	copy(result, lb.lines)
	return result
}

// FormattedLines returns all buffered lines parsed and formatted using logparser.
// If colorize is true, ANSI color codes are included in the output.
func (lb *LogBuffer) FormattedLines(colorize bool) []string {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	result := make([]string, len(lb.lines))
	for i, line := range lb.lines {
		parsed := logparser.ParseLogLine(line)
		result[i] = parsed.FormatLogLine(colorize)
	}
	return result
}

// Len returns the current number of lines in the buffer.
func (lb *LogBuffer) Len() int {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return len(lb.lines)
}

// Clear empties the buffer.
func (lb *LogBuffer) Clear() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.lines = lb.lines[:0]
	lb.partial.Reset()
}
