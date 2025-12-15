package commands_test

import (
	"bytes"
	"encoding/binary"
)

// formatDockerLog formats a plain text string as a Docker multiplexed stream.
// Docker uses a multiplexed format for container logs with 8-byte headers:
// [stream_type, 0, 0, 0, size_bytes...]
// stream_type: 1=stdout, 2=stderr
// size: 4-byte big-endian uint32
func formatDockerLog(content string, streamType byte) *bytes.Buffer {
	buf := new(bytes.Buffer)
	
	// Write multiplexed stream format for each line
	header := make([]byte, 8)
	header[0] = streamType // 1 for stdout, 2 for stderr
	// bytes 1-3 are padding (zeros)
	binary.BigEndian.PutUint32(header[4:], uint32(len(content)))
	
	buf.Write(header)
	buf.WriteString(content)
	
	return buf
}

// formatDockerStdout formats content as Docker stdout stream
func formatDockerStdout(content string) *bytes.Buffer {
	return formatDockerLog(content, 1)
}

// formatDockerStderr formats content as Docker stderr stream
func formatDockerStderr(content string) *bytes.Buffer {
	return formatDockerLog(content, 2)
}
