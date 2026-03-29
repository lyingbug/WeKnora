package httputil

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// SSEEvent represents a Server-Sent Events event.
type SSEEvent struct {
	Data []byte
	Done bool
}

// SSEReader reads Server-Sent Events from an io.Reader.
type SSEReader struct {
	scanner *bufio.Scanner
}

// NewSSEReader creates a new SSE reader with a 1MB buffer for long thinking chains.
func NewSSEReader(reader io.Reader) *SSEReader {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	return &SSEReader{scanner: scanner}
}

// ReadEvent reads the next SSE event from the stream.
func (r *SSEReader) ReadEvent() (*SSEEvent, error) {
	for r.scanner.Scan() {
		line := r.scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Check for stream termination
		if line == "data: [DONE]" {
			return &SSEEvent{Done: true}, nil
		}

		// Parse data lines
		if strings.HasPrefix(line, "data: ") {
			jsonStr := line[6:]
			return &SSEEvent{Data: []byte(jsonStr)}, nil
		}

		// Skip other lines (event:, id:, etc.)
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}

	return nil, errors.New("EOF")
}
