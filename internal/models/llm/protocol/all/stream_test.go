package all

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/llm/protocol"
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	"github.com/Tencent/WeKnora/internal/types"
)

// The fixtures below follow each vendor's documented stream shape. They exist
// because the three protocols learn about reasoning in three different ways —
// a delta field, a typed event, and a content block — and the whole point of
// the seam is that a consumer downstream cannot tell which one produced a
// given chunk.

// collect drains a driver's decoded stream into a slice.
func collect(t *testing.T, id spi.ProtocolID, body string) []types.StreamResponse {
	t.Helper()
	driver, err := protocol.MustGet(id)
	if err != nil {
		t.Fatalf("driver %s: %v", id, err)
	}

	out := make(chan types.StreamResponse, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		driver.DecodeStream(context.Background(), strings.NewReader(body), out)
		close(out)
	}()

	var chunks []types.StreamResponse
	for chunk := range out {
		chunks = append(chunks, chunk)
	}
	<-done
	return chunks
}

// transcript renders the decoded stream as a compact "type:content" list, so a
// failure shows the whole sequence rather than one mismatched field.
func transcript(chunks []types.StreamResponse) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		entry := string(c.ResponseType) + ":" + c.Content
		if c.Done {
			entry += "|done"
		}
		out = append(out, entry)
	}
	return out
}

func assertTranscript(t *testing.T, got []types.StreamResponse, want []string) {
	t.Helper()
	gotLines := transcript(got)
	if len(gotLines) != len(want) {
		t.Fatalf("stream had %d chunks, want %d\ngot:  %q\nwant: %q",
			len(gotLines), len(want), gotLines, want)
	}
	for i := range want {
		if gotLines[i] != want[i] {
			t.Errorf("chunk %d = %q, want %q\ngot:  %q\nwant: %q",
				i, gotLines[i], want[i], gotLines, want)
		}
	}
}

func TestOpenAIChatStreamSeparatesReasoning(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"let me"}}]}`,
		``,
		`data: {"choices":[{"delta":{"reasoning_content":" think"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"Paris"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"."},"finish_reason":"stop"}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	chunks := collect(t, spi.ProtocolOpenAIChat, body)
	assertTranscript(t, chunks, []string{
		"thinking:let me",
		"thinking: think",
		"thinking:|done",
		"answer:Paris",
		"answer:.",
		"answer:|done",
	})

	final := chunks[len(chunks)-1]
	if final.FinishReason != "stop" {
		t.Errorf("finish reason = %q, want stop", final.FinishReason)
	}
	if final.Usage == nil || final.Usage.TotalTokens != 15 {
		t.Errorf("usage = %+v, want total 15", final.Usage)
	}
}

// A model with no reasoning field inlines its reasoning in <think> tags, and
// the tags can straddle chunk boundaries. The decoded stream must be
// indistinguishable from a vendor that uses a dedicated field.
func TestOpenAIChatStreamSplitsInlineThinkTags(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"<thi"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"nk>weigh options"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"</thi"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"nk>Paris"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	assertTranscript(t, collect(t, spi.ProtocolOpenAIChat, body), []string{
		"thinking:weigh options",
		"thinking:|done",
		"answer:Paris",
		"answer:|done",
	})
}

func TestAnthropicStreamSeparatesThinkingBlocks(t *testing.T) {
	body := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":12,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"27 * 453"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"EqQBCgIYAhIM"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"text"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"12231"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":12,"output_tokens":40}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	chunks := collect(t, spi.ProtocolAnthropicMessages, body)
	assertTranscript(t, chunks, []string{
		"thinking:27 * 453",
		"thinking:|done",
		"answer:12231",
		"answer:|done",
	})

	final := chunks[len(chunks)-1]
	if final.FinishReason != "end_turn" {
		t.Errorf("finish reason = %q, want end_turn", final.FinishReason)
	}
	// The signature delta must not surface as user-visible text; it exists to
	// be replayed, not read.
	for _, chunk := range chunks {
		if strings.Contains(chunk.Content, "EqQBCgIYAhIM") {
			t.Errorf("signature leaked into the stream: %q", chunk.Content)
		}
	}
	if final.Usage == nil || final.Usage.CompletionTokens != 40 {
		t.Errorf("usage = %+v, want 40 completion tokens", final.Usage)
	}
}

func TestResponsesStreamSeparatesReasoningSummary(t *testing.T) {
	body := strings.Join([]string{
		`event: response.reasoning_summary_text.delta`,
		`data: {"type":"response.reasoning_summary_text.delta","delta":"Answering a simple question"}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"The capital of France is Paris."}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":14,"output_tokens":9,"total_tokens":23}}}`,
		``,
	}, "\n")

	chunks := collect(t, spi.ProtocolOpenAIResponses, body)
	assertTranscript(t, chunks, []string{
		"thinking:Answering a simple question",
		"thinking:|done",
		"answer:The capital of France is Paris.",
		"answer:|done",
	})

	final := chunks[len(chunks)-1]
	if final.FinishReason != "stop" {
		t.Errorf("finish reason = %q, want stop", final.FinishReason)
	}
	if final.Usage == nil || final.Usage.TotalTokens != 23 {
		t.Errorf("usage = %+v, want total 23", final.Usage)
	}
}

// A response that exhausts its output ceiling before writing visible text must
// say so, otherwise an empty answer looks like a normal completion.
func TestResponsesStreamReportsIncomplete(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.incomplete","response":{"status":"incomplete",` +
			`"incomplete_details":{"reason":"max_output_tokens"},` +
			`"usage":{"input_tokens":14,"output_tokens":300,"total_tokens":314}}}`,
		``,
	}, "\n")

	chunks := collect(t, spi.ProtocolOpenAIResponses, body)
	final := chunks[len(chunks)-1]
	if final.FinishReason != "max_output_tokens" {
		t.Errorf("finish reason = %q, want max_output_tokens", final.FinishReason)
	}
}
