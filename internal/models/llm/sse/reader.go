// Package sse reads Server-Sent Events streams from model APIs.
//
// It keeps the event name alongside the payload. The OpenAI Chat Completions
// protocol needs only the data lines, but the Responses and Anthropic
// protocols are event-typed, and a reader that discards `event:` forces every
// consumer to re-derive the type from the payload — which works only as long
// as every vendor also repeats it there.
package sse

import (
	"bufio"
	"io"
	"strings"
)

// maxLineBytes bounds one event line. Reasoning-capable models emit very long
// single-line payloads, and the default scanner limit truncates them into
// invalid JSON.
const maxLineBytes = 1024 * 1024

// Event is one parsed Server-Sent Event.
type Event struct {
	// Name is the `event:` field, empty when the stream does not send one.
	Name string
	// Data is the concatenated `data:` payload.
	Data []byte
	// Done reports the `[DONE]` sentinel that OpenAI-shaped streams end with.
	Done bool
}

// Reader parses an SSE stream.
type Reader struct {
	scanner *bufio.Scanner
}

// NewReader returns a reader over an SSE body.
func NewReader(r io.Reader) *Reader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, maxLineBytes), maxLineBytes)
	return &Reader{scanner: scanner}
}

// Next returns the next event, or io.EOF when the stream ends.
//
// Per the SSE specification an event is terminated by a blank line and may
// carry several data lines, which are joined with newlines. Vendors mostly
// send one line per event, but honoring the specification costs nothing and
// avoids truncating the ones that do not.
func (r *Reader) Next() (*Event, error) {
	event := &Event{}
	var data []string

	for r.scanner.Scan() {
		line := r.scanner.Text()

		if line == "" {
			// Blank line dispatches the event, unless nothing accumulated.
			if len(data) == 0 && event.Name == "" {
				continue
			}
			return finish(event, data), nil
		}
		if strings.HasPrefix(line, ":") {
			// A comment line, commonly used as a keep-alive.
			continue
		}

		field, value := splitField(line)
		switch field {
		case "event":
			event.Name = value
		case "data":
			if value == "[DONE]" {
				event.Done = true
				return event, nil
			}
			data = append(data, value)
		default:
			// `id:` and `retry:` carry no meaning for these APIs.
		}
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	// A stream that ends without a trailing blank line still owes its last
	// event, which is common when a connection closes right after the payload.
	if len(data) > 0 || event.Name != "" {
		return finish(event, data), nil
	}
	return nil, io.EOF
}

func finish(event *Event, data []string) *Event {
	event.Data = []byte(strings.Join(data, "\n"))
	return event
}

// splitField splits an SSE line into its field name and value, tolerating a
// missing space after the colon.
func splitField(line string) (field, value string) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return line, ""
	}
	field = line[:idx]
	value = line[idx+1:]
	return field, strings.TrimPrefix(value, " ")
}
