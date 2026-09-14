package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tencent/WeKnora/internal/im"
)

type messageRoundTripper func(*http.Request) (*http.Response, error)

func (f messageRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func messageTestResponse(code int) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"code":%d,"msg":"test response"}`, code))),
		Header:     make(http.Header),
	}
}

func useMessageTransport(t *testing.T, handler messageRoundTripper) *Adapter {
	t.Helper()
	original := httpClient
	httpClient = &http.Client{Transport: handler}
	t.Cleanup(func() { httpClient = original })
	return &Adapter{
		region: RegionFeishu, apiBaseURL: "https://example.com", tokenCache: "test-token",
		tokenExpAt: time.Now().Add(time.Hour),
	}
}

func readMessageText(t *testing.T, r *http.Request) string {
	t.Helper()
	var payload struct {
		Content string `json:"content"`
	}
	var content struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(payload.Content), &content); err != nil {
		t.Fatal(err)
	}
	return content.Text
}

func TestLongTextReplyRespectsRecipientRateLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var window []time.Time
		var delivered strings.Builder
		rejected := 0
		adapter := useMessageTransport(t, func(r *http.Request) (*http.Response, error) {
			now := time.Now()
			for len(window) > 0 && now.Sub(window[0]) >= time.Second {
				window = window[1:]
			}
			if len(window) >= 5 {
				rejected++
				return messageTestResponse(230020), nil
			}
			window = append(window, now)
			delivered.WriteString(readMessageText(t, r))
			return messageTestResponse(0), nil
		})
		answer := strings.Repeat("中文答案\n", 1200)
		err := adapter.SendReply(context.Background(),
			&im.IncomingMessage{MessageID: "original", UserID: "user"},
			&im.ReplyMessage{Content: answer})
		if err != nil || delivered.String() != answer || rejected != 0 {
			t.Fatalf("delivered=%d/%d bytes, rejected=%d, error=%v", delivered.Len(), len(answer), rejected, err)
		}
	})
}

func TestTextReplyRetriesOnlyRejectedChunk(t *testing.T) {
	for _, tc := range []struct {
		name, messageID string
		code            int
	}{
		{"reply recipient limit", "original", 230020},
		{"send recipient limit", "", 230020},
		{"reply API limit", "original", 99991400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var attempts []string
				var delivered strings.Builder
				var rejectedAt time.Time
				adapter := useMessageTransport(t, func(r *http.Request) (*http.Response, error) {
					wantPath := "/open-apis/im/v1/messages"
					if tc.messageID != "" {
						wantPath += "/original/reply"
					}
					if r.URL.Path != wantPath {
						t.Fatalf("unexpected endpoint: %s", r.URL.Path)
					}
					text := readMessageText(t, r)
					attempts = append(attempts, text)
					if len(attempts) == 2 {
						rejectedAt = time.Now()
						return messageTestResponse(tc.code), nil
					}
					if len(attempts) == 3 && time.Since(rejectedAt) < time.Second {
						t.Fatal("retry did not back off")
					}
					delivered.WriteString(text)
					return messageTestResponse(0), nil
				})
				answer := strings.Repeat("a", textReplyChunkBytes) + strings.Repeat("b", textReplyChunkBytes) + "tail"
				err := adapter.SendReply(context.Background(),
					&im.IncomingMessage{MessageID: tc.messageID, UserID: "user"},
					&im.ReplyMessage{Content: answer})
				if err != nil || delivered.String() != answer {
					t.Fatalf("incomplete reply: %v", err)
				}
				if len(attempts) != 4 || attempts[1] != attempts[2] || attempts[3] != "tail" {
					t.Fatalf("wrong chunk retry sequence: %d attempts", len(attempts))
				}
			})
		})
	}
}

func TestMessageRateLimitRetryBounds(t *testing.T) {
	for _, tc := range []struct {
		name               string
		code, wantAttempts int
		wantDelay          time.Duration
	}{
		{"persistent limit", 230020, 4, 7 * time.Second},
		{"permanent rejection", 230013, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				attempts := 0
				adapter := useMessageTransport(t, func(_ *http.Request) (*http.Response, error) {
					attempts++
					return messageTestResponse(tc.code), nil
				})
				start := time.Now()
				err := adapter.SendReply(context.Background(),
					&im.IncomingMessage{MessageID: "original", UserID: "user"},
					&im.ReplyMessage{Content: strings.Repeat("x", textReplyChunkBytes+1)})
				if err == nil || attempts != tc.wantAttempts || time.Since(start) != tc.wantDelay {
					t.Fatalf("attempts=%d elapsed=%s error=%v", attempts, time.Since(start), err)
				}
			})
		})
	}
}

func TestMessageSendWaitsRespectCancellation(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(fmt.Sprintf("rate_limited=%t", limited), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				attempts := 0
				adapter := useMessageTransport(t, func(_ *http.Request) (*http.Response, error) {
					attempts++
					if limited {
						return messageTestResponse(230020), nil
					}
					return messageTestResponse(0), nil
				})
				cancelDone := make(chan struct{})
				go func() {
					defer close(cancelDone)
					select {
					case <-time.After(100 * time.Millisecond):
						cancel()
					case <-ctx.Done():
					}
				}()
				defer func() { cancel(); <-cancelDone }()
				start := time.Now()
				var err error
				if limited {
					_, _, err = adapter.postFeishuMessage(ctx, "token", "https://example.com/reply", nil)
				} else {
					err = adapter.SendReply(ctx, &im.IncomingMessage{MessageID: "original", UserID: "user"},
						&im.ReplyMessage{Content: strings.Repeat("x", textReplyChunkBytes+1)})
				}
				if !errors.Is(err, context.Canceled) || attempts != 1 || time.Since(start) != 100*time.Millisecond {
					t.Fatalf("attempts=%d elapsed=%s error=%v", attempts, time.Since(start), err)
				}
			})
		})
	}
}
