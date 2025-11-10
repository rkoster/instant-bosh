package logparser_test

import (
	"testing"
	"time"

	"github.com/rkoster/instant-bosh/internal/logparser"
	"github.com/stretchr/testify/assert"
)

func TestParseLogLine(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantComp  string
		wantLevel string
		wantMsg   string
		wantErr   bool
	}{
		{
			name:      "director access log",
			input:     `[director/access] 2025-11-10T14:35:24.468092247Z INFO - 127.0.0.1 - - [10/Nov/2025:14:35:24 +0000] "GET /info HTTP/1.1" 200 384 "-" "Ruby" 0.003 0.002 .`,
			wantComp:  "director/access",
			wantLevel: "INFO",
			wantMsg:   `127.0.0.1 - - [10/Nov/2025:14:35:24 +0000] "GET /info HTTP/1.1" 200 384 "-" "Ruby" 0.003 0.002 .`,
			wantErr:   false,
		},
		{
			name:      "director stdout log",
			input:     `[director/director.stdout] 2025-11-10T14:35:25.643928564Z INFO - D, [2025-11-10T14:35:25.440811 #2175] [] DEBUG -- Director: (0.000759s) (conn: 4880) SELECT * FROM "deployments" ORDER BY "name" ASC`,
			wantComp:  "director/director.stdout",
			wantLevel: "INFO",
			wantMsg:   `D, [2025-11-10T14:35:25.440811 #2175] [] DEBUG -- Director: (0.000759s) (conn: 4880) SELECT * FROM "deployments" ORDER BY "name" ASC`,
			wantErr:   false,
		},
		{
			name:      "director stderr log",
			input:     `[director/director.stderr] 2025-11-10T14:35:25.644221132Z ERROR - 127.0.0.1 - - [10/Nov/2025:14:35:25 +0000] "GET /info HTTP/1.0" 200 384 0.0011`,
			wantComp:  "director/director.stderr",
			wantLevel: "ERROR",
			wantMsg:   `127.0.0.1 - - [10/Nov/2025:14:35:25 +0000] "GET /info HTTP/1.0" 200 384 0.0011`,
			wantErr:   false,
		},
		{
			name:      "nats sync log",
			input:     `[nats/bosh-nats-sync] 2025-11-10T14:35:25.661709087Z INFO - I, [2025-11-10T14:35:25.419547 #770]  INFO : Executing NATS Users Synchronization`,
			wantComp:  "nats/bosh-nats-sync",
			wantLevel: "INFO",
			wantMsg:   `I, [2025-11-10T14:35:25.419547 #770]  INFO : Executing NATS Users Synchronization`,
			wantErr:   false,
		},
		{
			name:      "unparseable line",
			input:     `This is not a valid log line`,
			wantComp:  "",
			wantLevel: "",
			wantMsg:   "",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logLine, err := logparser.ParseLogLine(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, logLine)
			assert.Equal(t, tt.wantComp, logLine.Component)
			assert.Equal(t, tt.wantLevel, logLine.Level)
			assert.Equal(t, tt.wantMsg, logLine.Message)
			assert.Equal(t, tt.input, logLine.Raw)

			// For valid log lines, check timestamp is parsed
			if tt.wantComp != "" {
				assert.False(t, logLine.Timestamp.IsZero())
			}
		})
	}
}

func TestParseLogLine_Timestamp(t *testing.T) {
	input := `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message`
	logLine, err := logparser.ParseLogLine(input)

	assert.NoError(t, err)
	assert.NotNil(t, logLine)

	expectedTime, _ := time.Parse(time.RFC3339Nano, "2025-11-10T14:35:24.468092247Z")
	assert.Equal(t, expectedTime, logLine.Timestamp)
}

func TestFormatLogLine_NoColor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "formatted log line",
			input:    `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message`,
			expected: `[director/access] 14:35:24.468 INFO - test message`,
		},
		{
			name:     "error level",
			input:    `[director/stderr] 2025-11-10T14:35:24.468092247Z ERROR - error message`,
			expected: `[director/stderr] 14:35:24.468 ERROR - error message`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logLine, err := logparser.ParseLogLine(tt.input)
			assert.NoError(t, err)

			formatted := logLine.FormatLogLine(false)
			assert.Equal(t, tt.expected, formatted)
		})
	}
}

func TestFormatLogLine_WithColor(t *testing.T) {
	input := `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message`
	logLine, err := logparser.ParseLogLine(input)
	assert.NoError(t, err)

	formatted := logLine.FormatLogLine(true)

	// Check that ANSI color codes are present
	assert.Contains(t, formatted, "\033[36m") // Cyan for component
	assert.Contains(t, formatted, "\033[90m") // Gray for timestamp
	assert.Contains(t, formatted, "\033[32m") // Green for INFO
	assert.Contains(t, formatted, "\033[0m")  // Reset
	assert.Contains(t, formatted, "director/access")
	assert.Contains(t, formatted, "test message")
}

func TestFormatLogLine_UnparsedLine(t *testing.T) {
	input := `This is not a valid log line`
	logLine, err := logparser.ParseLogLine(input)
	assert.NoError(t, err)

	formatted := logLine.FormatLogLine(false)
	assert.Equal(t, input, formatted)
}

func TestExtractComponents(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name: "multiple components",
			input: `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message 1
[nats/bosh-nats-sync] 2025-11-10T14:35:25.661709087Z INFO - test message 2
[director/director.stdout] 2025-11-10T14:35:25.643928564Z INFO - test message 3
[director/access] 2025-11-10T14:35:26.468092247Z INFO - test message 4`,
			expected: []string{"director/access", "director/director.stdout", "nats/bosh-nats-sync"},
		},
		{
			name: "single component",
			input: `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message 1
[director/access] 2025-11-10T14:35:25.468092247Z INFO - test message 2`,
			expected: []string{"director/access"},
		},
		{
			name:     "empty input",
			input:    "",
			expected: []string{},
		},
		{
			name: "mixed valid and invalid lines",
			input: `[director/access] 2025-11-10T14:35:24.468092247Z INFO - test message 1
This is not a valid log line
[nats/bosh-nats-sync] 2025-11-10T14:35:25.661709087Z INFO - test message 2`,
			expected: []string{"director/access", "nats/bosh-nats-sync"},
		},
		{
			name:     "no valid log lines",
			input:    "This is not a valid log line\nNeither is this",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			components := logparser.ExtractComponents(tt.input)
			assert.Equal(t, tt.expected, components)
		})
	}
}
