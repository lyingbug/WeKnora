package protocol

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestConnCallRoundTrip(t *testing.T) {
	cr, cw := io.Pipe()
	sr, sw := io.Pipe()
	t.Cleanup(func() {
		_ = cr.Close()
		_ = cw.Close()
		_ = sr.Close()
		_ = sw.Close()
	})
	go serveEcho(cr, sw)

	conn := NewConn(sr, cw)
	var out SearchResponse
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := conn.Call(ctx, MethodWebSearchSearch, SearchRequest{Query: "q"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 || out.Results[0].Title != "t" || out.Results[0].Snippet != "q" {
		t.Fatalf("out = %+v", out)
	}
}

func TestConnSkipsNoiseOnStdout(t *testing.T) {
	cr, cw := io.Pipe()
	sr, sw := io.Pipe()
	t.Cleanup(func() {
		_ = cr.Close()
		_ = cw.Close()
		_ = sr.Close()
		_ = sw.Close()
	})
	go func() {
		req, ok := readRequest(cr)
		if !ok {
			return
		}
		_, _ = sw.Write([]byte("debug: starting\n"))
		_, _ = sw.Write([]byte("not-json\n"))
		writeResult(sw, req.ID, SearchResponse{Results: []*types.WebSearchResult{}})
	}()

	conn := NewConn(sr, cw)
	var out SearchResponse
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Call(ctx, MethodWebSearchSearch, SearchRequest{}, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 0 {
		t.Fatalf("out = %+v", out)
	}
}

func TestConnCallTimeout(t *testing.T) {
	cr, cw := io.Pipe()
	sr, sw := io.Pipe()
	t.Cleanup(func() {
		_ = cr.Close()
		_ = cw.Close()
		_ = sr.Close()
		_ = sw.Close()
	})
	stop := make(chan struct{})
	go func() {
		_, _ = readRequest(cr)
		<-stop
	}()
	t.Cleanup(func() { close(stop) })

	conn := NewConn(sr, cw)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := conn.Call(ctx, MethodWebSearchSearch, SearchRequest{}, &SearchResponse{})
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestNotifyWritesNoID(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(strings.NewReader(""), &buf)
	if err := conn.Notify(MethodShutdown, nil); err != nil {
		t.Fatal(err)
	}
	var req Request
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &req); err != nil {
		t.Fatal(err)
	}
	if req.Method != MethodShutdown || req.ID != nil {
		t.Fatalf("req = %+v", req)
	}
}

func serveEcho(r io.Reader, w io.Writer) {
	req, ok := readRequest(r)
	if !ok {
		return
	}
	var in SearchRequest
	_ = json.Unmarshal(req.Params, &in)
	out := SearchResponse{}
	if q := strings.TrimSpace(in.Query); q != "" {
		out.Results = []*types.WebSearchResult{{
			Title: "t", URL: "https://x", Snippet: q,
		}}
	}
	writeResult(w, req.ID, out)
}

func readRequest(r io.Reader) (Request, bool) {
	br := bufio.NewReader(r)
	line, err := readLine(br)
	if err != nil {
		return Request{}, false
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Request{}, false
	}
	return req, true
}

func writeResult(w io.Writer, id any, result any) {
	raw, _ := json.Marshal(result)
	resp := Response{JSONRPC: jsonrpcVersion, ID: id, Result: raw}
	line, _ := json.Marshal(resp)
	_, _ = w.Write(append(line, '\n'))
}
