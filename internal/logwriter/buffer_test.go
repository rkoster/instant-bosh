package logwriter_test

import (
	"sync"
	"testing"

	"github.com/rkoster/instant-bosh/internal/logwriter"
	"github.com/stretchr/testify/assert"
)

func TestLogBuffer_BasicWrite(t *testing.T) {
	buf := logwriter.NewLogBuffer(10)

	input := "line 1\nline 2\nline 3\n"
	n, err := buf.Write([]byte(input))

	assert.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.Equal(t, 3, buf.Len())

	lines := buf.Lines()
	assert.Equal(t, []string{"line 1", "line 2", "line 3"}, lines)
}

func TestLogBuffer_PartialLines(t *testing.T) {
	buf := logwriter.NewLogBuffer(10)

	// Write partial line
	n1, err1 := buf.Write([]byte("partial "))
	assert.NoError(t, err1)
	assert.Equal(t, 8, n1)
	assert.Equal(t, 0, buf.Len()) // No complete line yet

	// Complete the line
	n2, err2 := buf.Write([]byte("line\n"))
	assert.NoError(t, err2)
	assert.Equal(t, 5, n2)
	assert.Equal(t, 1, buf.Len())

	lines := buf.Lines()
	assert.Equal(t, []string{"partial line"}, lines)
}

func TestLogBuffer_RingBuffer(t *testing.T) {
	buf := logwriter.NewLogBuffer(3) // Only keep 3 lines

	// Write 5 lines
	input := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	n, err := buf.Write([]byte(input))

	assert.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.Equal(t, 3, buf.Len()) // Should only have 3 lines

	lines := buf.Lines()
	// Should have the last 3 lines
	assert.Equal(t, []string{"line 3", "line 4", "line 5"}, lines)
}

func TestLogBuffer_FormattedLines(t *testing.T) {
	buf := logwriter.NewLogBuffer(10)

	input := "[main] 2025-11-10T14:35:24.468092247Z INFO - Starting BOSH Director\n"
	buf.Write([]byte(input))

	// Test without color
	formattedNoColor := buf.FormattedLines(false)
	assert.Len(t, formattedNoColor, 1)
	assert.Contains(t, formattedNoColor[0], "[main]")
	assert.Contains(t, formattedNoColor[0], "INFO")
	assert.Contains(t, formattedNoColor[0], "Starting BOSH Director")
	assert.NotContains(t, formattedNoColor[0], "\033[") // No ANSI codes

	// Test with color
	formattedColor := buf.FormattedLines(true)
	assert.Len(t, formattedColor, 1)
	assert.Contains(t, formattedColor[0], "\033[") // Should have ANSI codes
	assert.Contains(t, formattedColor[0], "Starting BOSH Director")
}

func TestLogBuffer_FormattedLines_UnparsedLine(t *testing.T) {
	buf := logwriter.NewLogBuffer(10)

	input := "This is not a valid log line\n"
	buf.Write([]byte(input))

	formatted := buf.FormattedLines(false)
	assert.Len(t, formatted, 1)
	assert.Equal(t, "This is not a valid log line", formatted[0])
}

func TestLogBuffer_Clear(t *testing.T) {
	buf := logwriter.NewLogBuffer(10)

	buf.Write([]byte("line 1\nline 2\n"))
	assert.Equal(t, 2, buf.Len())

	buf.Clear()
	assert.Equal(t, 0, buf.Len())

	lines := buf.Lines()
	assert.Empty(t, lines)
}

func TestLogBuffer_LinesReturnsCopy(t *testing.T) {
	buf := logwriter.NewLogBuffer(10)

	buf.Write([]byte("line 1\nline 2\n"))

	lines1 := buf.Lines()
	lines2 := buf.Lines()

	// Modifying lines1 should not affect lines2
	lines1[0] = "modified"
	assert.Equal(t, "line 1", lines2[0])
}

func TestLogBuffer_ConcurrentWrites(t *testing.T) {
	buf := logwriter.NewLogBuffer(100)

	var wg sync.WaitGroup
	numGoroutines := 10
	linesPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < linesPerGoroutine; j++ {
				buf.Write([]byte("line from goroutine\n"))
			}
		}(i)
	}

	wg.Wait()

	// Should have all lines (100 total, buffer size is 100)
	assert.Equal(t, 100, buf.Len())
}

func TestLogBuffer_EmptyLines(t *testing.T) {
	buf := logwriter.NewLogBuffer(10)

	input := "line 1\n\nline 2\n"
	buf.Write([]byte(input))

	lines := buf.Lines()
	// Empty lines should be preserved
	assert.Equal(t, []string{"line 1", "", "line 2"}, lines)
}

func TestLogBuffer_MultipleWritesSingleLine(t *testing.T) {
	buf := logwriter.NewLogBuffer(10)

	// Write a line in multiple small chunks
	buf.Write([]byte("["))
	buf.Write([]byte("main"))
	buf.Write([]byte("] "))
	buf.Write([]byte("2025-11-10T14:35:24.468092247Z INFO - "))
	buf.Write([]byte("test message"))
	buf.Write([]byte("\n"))

	assert.Equal(t, 1, buf.Len())

	lines := buf.Lines()
	assert.Equal(t, "[main] 2025-11-10T14:35:24.468092247Z INFO - test message", lines[0])
}
