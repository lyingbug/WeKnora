// Package anthropic implements the Anthropic Messages protocol.
//
// It differs from the OpenAI-shaped protocols in ways that are structural
// rather than cosmetic: the system prompt is a separate top-level field, every
// message body is a list of typed content blocks, tool results travel as user
// blocks instead of a tool role, and reasoning is a first-class block carrying
// a signature that must return unmodified on the next turn.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Tencent/WeKnora/internal/models/llm/protocol"
	"github.com/Tencent/WeKnora/internal/models/llm/protocol/internal/emit"
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	"github.com/Tencent/WeKnora/internal/models/llm/sse"
	"github.com/Tencent/WeKnora/internal/types"
)

// defaultMaxTokens is the fallback ceiling. The Messages API requires
// max_tokens on every request, unlike the OpenAI-shaped protocols where it is
// optional, so the driver must supply one when the vendor descriptor has not.
const defaultMaxTokens = 1024

// Driver implements protocol.Driver for the Messages API.
type Driver struct{}

func init() { protocol.Register(Driver{}) }

// ID reports the protocol.
func (Driver) ID() spi.ProtocolID { return spi.ProtocolAnthropicMessages }

// EndpointPath reports the standard path.
func (Driver) EndpointPath() string { return "/v1/messages" }

// BuildDraft renders the call into a Messages body.
func (d Driver) BuildDraft(call protocol.Call) (*spi.Draft, error) {
	draft := spi.NewDraft(d.ID(), call.Model, call.Stream)
	draft.Set("model", call.Model)
	if call.Stream {
		draft.Set("stream", true)
	}

	system, messages := convertMessages(call.Messages, call.Replay)
	if system != "" {
		draft.Set("system", system)
	}
	draft.Set("messages", messages)

	maxTokens := defaultMaxTokens
	opts := call.Options
	if opts != nil {
		if v := opts.EffectiveMaxTokens(); v > 0 {
			maxTokens = v
		}
		if opts.Temperature > 0 {
			draft.Set("temperature", opts.Temperature)
		}
		if opts.TopP > 0 {
			draft.Set("top_p", opts.TopP)
		}
		if len(opts.Tools) > 0 {
			draft.Set("tools", convertTools(opts.Tools))
		}
		if choice := convertToolChoice(opts.ToolChoice); choice != nil {
			draft.Set("tool_choice", choice)
		}
	}
	draft.Set("max_tokens", maxTokens)
	return draft, nil
}

// convertMessages splits the system prompt out and renders the rest as
// content-block messages.
//
// Consecutive same-role messages are merged because the API rejects a
// conversation that alternates incorrectly, and tool results in particular
// arrive as several separate messages that must become one user turn.
func convertMessages(messages []spi.Message, replay spi.ReasoningReplay) (string, []any) {
	var systemParts []string
	type turn struct {
		role   string
		blocks []any
	}
	var turns []turn

	appendBlocks := func(role string, blocks []any) {
		if len(blocks) == 0 {
			return
		}
		if n := len(turns); n > 0 && turns[n-1].role == role {
			turns[n-1].blocks = append(turns[n-1].blocks, blocks...)
			return
		}
		turns = append(turns, turn{role: role, blocks: blocks})
	}

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if text := messageText(msg); text != "" {
				systemParts = append(systemParts, text)
			}

		case "tool":
			// A tool result is a user-role block referencing the call it
			// answers, not a role of its own.
			appendBlocks("user", []any{map[string]any{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     msg.Content,
			}})

		case "assistant":
			var blocks []any
			// Thinking blocks must come first and must return exactly as they
			// arrived, signature included, or the API rejects the turn.
			if protocol.ShouldReplayReasoning(replay, msg) {
				block := map[string]any{"type": "thinking", "thinking": msg.ReasoningContent}
				if msg.ReasoningSignature != "" {
					block["signature"] = msg.ReasoningSignature
				}
				blocks = append(blocks, block)
			}
			blocks = append(blocks, contentBlocks(msg)...)
			for _, call := range msg.ToolCalls {
				var input any
				if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
					input = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    call.ID,
					"name":  call.Function.Name,
					"input": input,
				})
			}
			appendBlocks("assistant", blocks)

		default:
			appendBlocks("user", contentBlocks(msg))
		}
	}

	out := make([]any, 0, len(turns))
	for _, t := range turns {
		out = append(out, map[string]any{"role": t.role, "content": t.blocks})
	}
	return strings.Join(systemParts, "\n\n"), out
}

// contentBlocks renders a message body as text and image blocks.
func contentBlocks(msg spi.Message) []any {
	var blocks []any
	for _, part := range msg.MultiContent {
		switch part.Type {
		case "text":
			if part.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": part.Text})
			}
		case "image_url":
			if part.ImageURL != nil {
				if block := imageBlock(part.ImageURL.URL); block != nil {
					blocks = append(blocks, block)
				}
			}
		}
	}
	for _, img := range msg.Images {
		if block := imageBlock(img); block != nil {
			blocks = append(blocks, block)
		}
	}
	if msg.Content != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
	}
	return blocks
}

// imageBlock renders an image reference. Anthropic takes base64 payloads and
// URLs through different source shapes, so a data URI has to be taken apart
// rather than passed along.
func imageBlock(reference string) map[string]any {
	if reference == "" {
		return nil
	}
	if strings.HasPrefix(reference, "data:") {
		mediaType, data, ok := parseDataURI(reference)
		if !ok {
			return nil
		}
		return map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": mediaType,
				"data":       data,
			},
		}
	}
	if u, err := url.Parse(reference); err != nil || u.Scheme == "" {
		return nil
	}
	return map[string]any{
		"type":   "image",
		"source": map[string]any{"type": "url", "url": reference},
	}
}

func parseDataURI(uri string) (mediaType, data string, ok bool) {
	rest := strings.TrimPrefix(uri, "data:")
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", "", false
	}
	meta, payload := rest[:comma], rest[comma+1:]
	if !strings.HasSuffix(meta, ";base64") {
		return "", "", false
	}
	mediaType = strings.TrimSuffix(meta, ";base64")
	if mediaType == "" {
		mediaType = "image/png"
	}
	return mediaType, payload, true
}

func messageText(msg spi.Message) string {
	if msg.Content != "" {
		return msg.Content
	}
	var parts []string
	for _, part := range msg.MultiContent {
		if part.Type == "text" && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func convertTools(tools []spi.Tool) []any {
	out := make([]any, 0, len(tools))
	for _, tool := range tools {
		var schema any
		if len(tool.Function.Parameters) > 0 {
			_ = json.Unmarshal(tool.Function.Parameters, &schema)
		}
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"name":         tool.Function.Name,
			"description":  tool.Function.Description,
			"input_schema": schema,
		})
	}
	return out
}

// convertToolChoice maps the neutral choice onto the object form this API uses.
func convertToolChoice(choice string) map[string]any {
	switch choice {
	case "":
		return nil
	case "auto":
		return map[string]any{"type": "auto"}
	case "required":
		return map[string]any{"type": "any"}
	case "none":
		return map[string]any{"type": "none"}
	default:
		// Anything else names a specific tool.
		return map[string]any{"type": "tool", "name": choice}
	}
}

// message mirrors a complete Messages response.
type message struct {
	Content []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Thinking  string          `json:"thinking"`
		Signature string          `json:"signature"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      *usage `json:"usage"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// ParseResponse decodes a complete Messages response.
func (d Driver) ParseResponse(body []byte) (*types.ChatResponse, error) {
	var resp message
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return nil, fmt.Errorf("provider error: %s", resp.Error.Message)
	}

	out := &types.ChatResponse{FinishReason: resp.StopReason, Usage: resp.Usage.normalize()}
	var text, thinking []string
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			text = append(text, block.Text)
		case "thinking":
			thinking = append(thinking, block.Thinking)
		case "redacted_thinking":
			// The payload is encrypted and carries no readable text; the turn
			// still reasoned, so the fact is worth surfacing.
			thinking = append(thinking, "")
		case "tool_use":
			out.ToolCalls = append(out.ToolCalls, types.LLMToolCall{
				ID:   block.ID,
				Type: "function",
				Function: types.FunctionCall{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}
	out.Content = strings.Join(text, "")
	out.ReasoningContent = strings.Join(thinking, "")
	return out, nil
}

// streamEvent mirrors the events this driver consumes.
type streamEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message *struct {
		Usage *usage `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage *usage `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// DecodeStream decodes a Messages SSE stream.
func (d Driver) DecodeStream(ctx context.Context, body io.Reader, out chan<- types.StreamResponse) {
	reader := sse.NewReader(body)
	thinking := &emit.Thinking{}

	var (
		finishReason string
		acc          *types.TokenUsage
		blockType    string
	)

	for {
		select {
		case <-ctx.Done():
			emit.Error(out, ctx.Err().Error())
			return
		default:
		}

		event, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			emit.Error(out, fmt.Sprintf("read stream: %v", err))
			return
		}
		if event.Done || len(event.Data) == 0 {
			if event.Done {
				break
			}
			continue
		}

		var ev streamEvent
		if err := json.Unmarshal(event.Data, &ev); err != nil {
			emit.Error(out, fmt.Sprintf("decode stream event: %v", err))
			return
		}
		if ev.Error != nil && ev.Error.Message != "" {
			emit.Error(out, ev.Error.Message)
			return
		}

		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				acc = mergeUsage(acc, ev.Message.Usage)
			}

		case "content_block_start":
			if ev.ContentBlock != nil {
				blockType = ev.ContentBlock.Type
			}
			// Text follows reasoning, so the panel closes here rather than
			// waiting for the first token.
			if blockType == "text" {
				thinking.Finish(out)
			}

		case "content_block_delta":
			if ev.Delta == nil {
				continue
			}
			switch ev.Delta.Type {
			case "thinking_delta":
				thinking.Emit(out, ev.Delta.Thinking)
			case "text_delta":
				thinking.Finish(out)
				if ev.Delta.Text != "" {
					out <- types.StreamResponse{
						ResponseType: types.ResponseTypeAnswer,
						Content:      ev.Delta.Text,
					}
				}
			case "signature_delta", "input_json_delta":
				// The signature closes a thinking block and the partial JSON
				// accumulates a tool call; neither is user-visible text.
			}

		case "content_block_stop":
			blockType = ""

		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				finishReason = ev.Delta.StopReason
			}
			acc = mergeUsage(acc, ev.Usage)

		case "message_stop":
			// The stream ends after this event.
		}
	}

	thinking.Finish(out)
	out <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Done:         true,
		Usage:        acc,
		FinishReason: finishReason,
	}
}

// usage mirrors Anthropic's token accounting, where cache counters sit beside
// the input count rather than inside it.
type usage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
}

// normalize folds Anthropic's counters into the shared usage model. Cache
// reads and writes are additional input tokens here, not a subset of
// input_tokens, so the prompt total is their sum.
func (u *usage) normalize() types.TokenUsage {
	if u == nil {
		return types.TokenUsage{}
	}
	read := valueOr(u.CacheReadInputTokens)
	write := valueOr(u.CacheCreationInputTokens)
	prompt := u.InputTokens + read + write

	out := types.TokenUsage{
		PromptTokens:     prompt,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      prompt + u.OutputTokens,
	}
	reported := u.CacheReadInputTokens != nil || u.CacheCreationInputTokens != nil
	miss := prompt - read
	if miss < 0 {
		miss = 0
	}
	out.SetPromptCacheUsage(read, write, miss, reported)
	return out
}

// mergeUsage combines the partial counts a stream reports at its start and end.
func mergeUsage(current *types.TokenUsage, next *usage) *types.TokenUsage {
	if next == nil {
		return current
	}
	merged := next.normalize()
	if current == nil {
		return &merged
	}
	// message_start reports input counts and message_delta reports the final
	// output count, so each field takes the larger of the two observations.
	if merged.PromptTokens > current.PromptTokens {
		current.PromptTokens = merged.PromptTokens
		current.CacheReadTokens = merged.CacheReadTokens
		current.CacheWriteTokens = merged.CacheWriteTokens
		current.CacheMissTokens = merged.CacheMissTokens
		current.CacheReported = current.CacheReported || merged.CacheReported
	}
	if merged.CompletionTokens > current.CompletionTokens {
		current.CompletionTokens = merged.CompletionTokens
	}
	current.TotalTokens = current.PromptTokens + current.CompletionTokens
	return current
}

func valueOr(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// Endpoint reports the full URL for a base URL, tolerating the spellings users
// paste: a bare host, a versioned base, or the endpoint itself.
func (d Driver) Endpoint(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	switch {
	case strings.HasSuffix(base, "/messages"):
		return base
	case strings.HasSuffix(base, "/v1"), strings.HasSuffix(base, "/v1beta"):
		return base + "/messages"
	default:
		return base + d.EndpointPath()
	}
}
