// Package protocol is the WeKnora plugin ABI: JSON-RPC 2.0 over a
// newline-delimited byte stream (the same framing MCP stdio uses).
//
// Transports (stdio subprocess, in-process JS, HTTP fallback) only bind
// this ABI. Plugin authors implement methods, not servers.
package protocol

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
)

const (
	jsonrpcVersion = "2.0"
	maxLineBytes   = 4 << 20
)

// Request is a JSON-RPC 2.0 request or notification (notification has no ID).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e == nil {
		return "jsonrpc: empty error"
	}
	return fmt.Sprintf("jsonrpc %d: %s", e.Code, e.Message)
}

// Conn is a serialized JSON-RPC client over a bidirectional byte stream.
type Conn struct {
	r    *bufio.Reader
	w    *bufio.Writer
	mu   sync.Mutex
	next atomic.Int64
}

// NewConn wraps a reader/writer pair. Writes are flushed after each message.
func NewConn(r io.Reader, w io.Writer) *Conn {
	return &Conn{
		r: bufio.NewReaderSize(r, 64*1024),
		w: bufio.NewWriterSize(w, 64*1024),
	}
}

// Call sends a request and waits for the matching response. Non-JSON lines
// (beginner console.log on stdout) are skipped so logs do not break the stream.
func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	if c == nil {
		return fmt.Errorf("jsonrpc: nil conn")
	}
	id := c.next.Add(1)
	raw, err := marshalParams(params)
	if err != nil {
		return err
	}
	req := Request{JSONRPC: jsonrpcVersion, ID: id, Method: method, Params: raw}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.writeLocked(req); err != nil {
		return err
	}

	type outcome struct {
		resp Response
		err  error
	}
	ch := make(chan outcome, 1)
	go func() {
		resp, err := c.readMatchLocked(id)
		ch <- outcome{resp: resp, err: err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case out := <-ch:
		if out.err != nil {
			return out.err
		}
		if out.resp.Error != nil {
			return out.resp.Error
		}
		if result == nil || len(out.resp.Result) == 0 || string(out.resp.Result) == "null" {
			return nil
		}
		if err := json.Unmarshal(out.resp.Result, result); err != nil {
			return fmt.Errorf("jsonrpc: decode result: %w", err)
		}
		return nil
	}
}

// Notify writes a notification (no response expected).
func (c *Conn) Notify(method string, params any) error {
	if c == nil {
		return fmt.Errorf("jsonrpc: nil conn")
	}
	raw, err := marshalParams(params)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked(Request{JSONRPC: jsonrpcVersion, Method: method, Params: raw})
}

func (c *Conn) writeLocked(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := c.w.Write(line); err != nil {
		return err
	}
	if err := c.w.WriteByte('\n'); err != nil {
		return err
	}
	return c.w.Flush()
}

func (c *Conn) readMatchLocked(id int64) (Response, error) {
	for {
		line, err := readLine(c.r)
		if err != nil {
			return Response{}, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}
		if resp.ID == nil {
			continue
		}
		got, ok := asInt64(resp.ID)
		if !ok || got != id {
			continue
		}
		return resp, nil
	}
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(buf)+len(chunk) > maxLineBytes {
			return nil, fmt.Errorf("jsonrpc: line exceeds %d bytes", maxLineBytes)
		}
		buf = append(buf, chunk...)
		if err == nil {
			return bytes.TrimSuffix(buf, []byte("\n")), nil
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF && len(buf) > 0 {
			return buf, nil
		}
		return nil, err
	}
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		return raw, nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		return i, err == nil
	default:
		return 0, false
	}
}
