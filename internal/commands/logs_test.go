package commands_test

import (
	"bytes"
	"testing"

	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/stretchr/testify/assert"
)

func TestLogWriter_NoFilter(t *testing.T) {
	var buf bytes.Buffer
	writer := commands.NewLogWriter(&buf, false, nil)

	input := `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message 1
[nats/bosh-nats-sync] 2025-11-10T14:35:25.661709087Z INFO - test message 2
`

	n, err := writer.Write([]byte(input))
	assert.NoError(t, err)
	assert.Equal(t, len(input), n)

	output := buf.String()
	// Should contain both lines formatted
	assert.Contains(t, output, "[director/access]")
	assert.Contains(t, output, "test message 1")
	assert.Contains(t, output, "[nats/bosh-nats-sync]")
	assert.Contains(t, output, "test message 2")
}

func TestLogWriter_WithFilter_SingleComponent(t *testing.T) {
	var buf bytes.Buffer
	writer := commands.NewLogWriter(&buf, false, []string{"director/access"})

	input := `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message 1
[nats/bosh-nats-sync] 2025-11-10T14:35:25.661709087Z INFO - test message 2
[director/access] 2025-11-10T14:35:26.468092247Z INFO - test message 3
`

	n, err := writer.Write([]byte(input))
	assert.NoError(t, err)
	assert.Equal(t, len(input), n)

	output := buf.String()
	// Should contain only director/access lines
	assert.Contains(t, output, "test message 1")
	assert.Contains(t, output, "test message 3")
	assert.NotContains(t, output, "test message 2")
	assert.NotContains(t, output, "nats/bosh-nats-sync")
}

func TestLogWriter_WithFilter_MultipleComponents(t *testing.T) {
	var buf bytes.Buffer
	writer := commands.NewLogWriter(&buf, false, []string{"director/access", "director/director.stdout"})

	input := `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message 1
[nats/bosh-nats-sync] 2025-11-10T14:35:25.661709087Z INFO - test message 2
[director/director.stdout] 2025-11-10T14:35:26.468092247Z INFO - test message 3
[postgres/postgres] 2025-11-10T14:35:27.468092247Z INFO - test message 4
`

	n, err := writer.Write([]byte(input))
	assert.NoError(t, err)
	assert.Equal(t, len(input), n)

	output := buf.String()
	// Should contain only director/access and director/director.stdout lines
	assert.Contains(t, output, "test message 1")
	assert.Contains(t, output, "test message 3")
	assert.NotContains(t, output, "test message 2")
	assert.NotContains(t, output, "test message 4")
}

func TestLogWriter_WithColor(t *testing.T) {
	var buf bytes.Buffer
	writer := commands.NewLogWriter(&buf, true, nil)

	input := `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message
`

	n, err := writer.Write([]byte(input))
	assert.NoError(t, err)
	assert.Equal(t, len(input), n)

	output := buf.String()
	// Should contain ANSI color codes
	assert.Contains(t, output, "\033[")
	assert.Contains(t, output, "test message")
}

func TestLogWriter_PartialLines(t *testing.T) {
	var buf bytes.Buffer
	writer := commands.NewLogWriter(&buf, false, nil)

	// Write partial line
	input1 := `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test `
	n1, err1 := writer.Write([]byte(input1))
	assert.NoError(t, err1)
	assert.Equal(t, len(input1), n1)

	// Buffer should be empty (no complete line)
	assert.Equal(t, "", buf.String())

	// Complete the line
	input2 := `message
`
	n2, err2 := writer.Write([]byte(input2))
	assert.NoError(t, err2)
	assert.Equal(t, len(input2), n2)

	// Now the complete line should be in the buffer
	output := buf.String()
	assert.Contains(t, output, "test message")
}

func TestLogWriter_UnparsedLines(t *testing.T) {
	var buf bytes.Buffer
	writer := commands.NewLogWriter(&buf, false, nil)

	input := `This is not a valid log line
[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message
Another invalid line
`

	n, err := writer.Write([]byte(input))
	assert.NoError(t, err)
	assert.Equal(t, len(input), n)

	output := buf.String()
	// Unparsed lines should be written as-is
	assert.Contains(t, output, "This is not a valid log line")
	assert.Contains(t, output, "Another invalid line")
	assert.Contains(t, output, "test message")
}
