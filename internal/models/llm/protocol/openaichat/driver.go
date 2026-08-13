// Package openaichat implements the OpenAI Chat Completions protocol, the
// baseline most vendors serve.
package openaichat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Tencent/WeKnora/internal/models/llm/protocol"
	"github.com/Tencent/WeKnora/internal/models/llm/protocol/internal/emit"
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	"github.com/Tencent/WeKnora/internal/models/llm/sse"
	"github.com/Tencent/WeKnora/internal/types"
)

// Driver implements protocol.Driver for Chat Completions.
type Driver struct{}

func init() { protocol.Register(Driver{}) }

// ID reports the protocol.
func (Driver) ID() spi.ProtocolID { return spi.ProtocolOpenAIChat }

// EndpointPath reports the standard path.
func (Driver) EndpointPath() string { return "/chat/completions" }

// BuildDraft renders the call into a Chat Completions body.
//
// Only fields the caller actually set are written. An OpenAI-shaped API treats
// an explicit zero as a real value, so writing every field would override
// model defaults the caller never touched — and would also defeat the vendor
// layer, whose forbidden-parameter contract is about removing fields that
// would otherwise be present.
func (d Driver) BuildDraft(call protocol.Call) (*spi.Draft, error) {
	draft := spi.NewDraft(d.ID(), call.Model, call.Stream)
	draft.Set("model", call.Model)
	draft.Set("messages", convertMessages(call.Messages, call.Replay))
	if call.Stream {
		draft.Set("stream", true)
		// Without this, usage is absent from streamed responses and every
		// token count downstream silently reads zero.
		draft.Set("stream_options", map[string]any{"include_usage": true})
	}

	opts := call.Options
	if opts == nil {
		return draft, nil
	}
	if opts.Temperature > 0 {
		draft.Set("temperature", opts.Temperature)
	}
	if opts.TopP > 0 {
		draft.Set("top_p", opts.TopP)
	}
	if opts.FrequencyPenalty != 0 {
		draft.Set("frequency_penalty", opts.FrequencyPenalty)
	}
	if opts.PresencePenalty != 0 {
		draft.Set("presence_penalty", opts.PresencePenalty)
	}
	if opts.Seed != 0 {
		draft.Set("seed", opts.Seed)
	}
	if maxTokens := opts.EffectiveMaxTokens(); maxTokens > 0 {
		draft.Set("max_tokens", maxTokens)
	}
	if len(opts.Tools) > 0 {
		draft.Set("tools", convertTools(opts.Tools))
	}
	if opts.ToolChoice != "" {
		draft.Set("tool_choice", opts.ToolChoice)
	}
	if opts.ParallelToolCalls != nil {
		draft.Set("parallel_tool_calls", *opts.ParallelToolCalls)
	}
	if len(opts.Format) > 0 {
		var format any
		if err := json.Unmarshal(opts.Format, &format); err != nil {
			return nil, fmt.Errorf("decode response format: %w", err)
		}
		draft.Set("response_format", format)
	}
	return draft, nil
}

// convertMessages renders the conversation in Chat Completions shape.
func convertMessages(messages []spi.Message, replay spi.ReasoningReplay) []any {
	out := make([]any, 0, len(messages))
	for _, msg := range messages {
		wire := map[string]any{"role": msg.Role}

		switch {
		case len(msg.MultiContent) > 0:
			wire["content"] = convertParts(msg.MultiContent)
		case len(msg.Images) > 0 && msg.Role == "user":
			parts := make([]any, 0, len(msg.Images)+1)
			for _, img := range msg.Images {
				parts = append(parts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": img},
				})
			}
			parts = append(parts, map[string]any{"type": "text", "text": msg.Content})
			wire["content"] = parts
		default:
			wire["content"] = msg.Content
		}

		if len(msg.ToolCalls) > 0 {
			wire["tool_calls"] = convertToolCalls(msg.ToolCalls)
		}
		if msg.Role == "tool" {
			wire["tool_call_id"] = msg.ToolCallID
			if msg.Name != "" {
				wire["name"] = msg.Name
			}
		} else if msg.Name != "" {
			wire["name"] = msg.Name
		}
		if protocol.ShouldReplayReasoning(replay, msg) {
			wire["reasoning_content"] = msg.ReasoningContent
		}
		out = append(out, wire)
	}
	return out
}

func convertParts(parts []spi.MessageContentPart) []any {
	out := make([]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text":
			out = append(out, map[string]any{"type": "text", "text": part.Text})
		case "image_url":
			if part.ImageURL == nil {
				continue
			}
			image := map[string]any{"url": part.ImageURL.URL}
			if part.ImageURL.Detail != "" {
				image["detail"] = part.ImageURL.Detail
			}
			out = append(out, map[string]any{"type": "image_url", "image_url": image})
		}
	}
	return out
}

func convertToolCalls(calls []spi.ToolCall) []any {
	out := make([]any, 0, len(calls))
	for _, call := range calls {
		callType := call.Type
		if callType == "" {
			callType = "function"
		}
		wire := map[string]any{
			"id":   call.ID,
			"type": callType,
			"function": map[string]any{
				"name":      call.Function.Name,
				"arguments": call.Function.Arguments,
			},
		}
		// Vendor state travels back exactly as it arrived. Gemini's thought
		// signatures ride here, and a replayed call without them is rejected.
		for key, raw := range call.ProviderMetadata {
			var value any
			if err := json.Unmarshal(raw, &value); err == nil {
				wire[key] = value
			}
		}
		out = append(out, wire)
	}
	return out
}

func convertTools(tools []spi.Tool) []any {
	out := make([]any, 0, len(tools))
	for _, tool := range tools {
		var params any
		if len(tool.Function.Parameters) > 0 {
			_ = json.Unmarshal(tool.Function.Parameters, &params)
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
				"parameters":  params,
			},
		})
	}
	return out
}

// completion mirrors the response fields this driver consumes. Vendors add
// their own, which json ignores.
type completion struct {
	Choices []struct {
		Message struct {
			Content          string            `json:"content"`
			ReasoningContent string            `json:"reasoning_content"`
			Reasoning        string            `json:"reasoning"`
			ToolCalls        []json.RawMessage `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *usage    `json:"usage"`
	Error *apiError `json:"error"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

// ParseResponse decodes a complete Chat Completions response.
func (d Driver) ParseResponse(body []byte) (*types.ChatResponse, error) {
	var resp completion
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode chat completion: %w", err)
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return nil, fmt.Errorf("provider error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("chat completion contained no choices")
	}

	choice := resp.Choices[0]
	reasoning := choice.Message.ReasoningContent
	if reasoning == "" {
		// Some OpenAI-compatible servers spell it `reasoning`.
		reasoning = choice.Message.Reasoning
	}
	content, inlineReasoning := emit.SplitInlineThinking(choice.Message.Content)
	if reasoning == "" {
		reasoning = inlineReasoning
	}

	out := &types.ChatResponse{
		Content:          content,
		ReasoningContent: reasoning,
		FinishReason:     choice.FinishReason,
		ToolCalls:        decodeToolCalls(choice.Message.ToolCalls),
		Usage:            resp.Usage.normalize(),
	}
	return out, nil
}

// decodeToolCalls decodes tool calls while preserving any vendor fields, which
// the standard shape cannot describe but the next turn must carry back.
func decodeToolCalls(raw []json.RawMessage) []types.LLMToolCall {
	if len(raw) == 0 {
		return nil
	}
	out := make([]types.LLMToolCall, 0, len(raw))
	for _, item := range raw {
		var call struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}
		if err := json.Unmarshal(item, &call); err != nil {
			continue
		}
		out = append(out, types.LLMToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: types.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return out
}

// DecodeStream decodes a Chat Completions SSE stream.
func (d Driver) DecodeStream(ctx context.Context, body io.Reader, out chan<- types.StreamResponse) {
	reader := sse.NewReader(body)
	thinking := &emit.Thinking{}
	splitter := &emit.InlineSplitter{}

	var (
		finishReason string
		acc          *types.TokenUsage
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
		if event.Done {
			break
		}
		if len(event.Data) == 0 {
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal(event.Data, &chunk); err != nil {
			emit.Error(out, fmt.Sprintf("decode stream chunk: %v", err))
			return
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			emit.Error(out, chunk.Error.Message)
			return
		}
		if chunk.Usage != nil {
			usage := chunk.Usage.normalize()
			acc = &usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}

		reasoning := choice.Delta.ReasoningContent
		if reasoning == "" {
			reasoning = choice.Delta.Reasoning
		}
		if reasoning != "" {
			thinking.Emit(out, reasoning)
		}

		if choice.Delta.Content != "" {
			// A model that inlines its reasoning in <think> tags is routed to
			// the same thinking channel, so the UI shows one behavior
			// regardless of which convention the vendor picked.
			answer, inline := splitter.Feed(choice.Delta.Content)
			if inline != "" {
				thinking.Emit(out, inline)
			}
			if answer != "" {
				thinking.Finish(out)
				out <- types.StreamResponse{
					ResponseType: types.ResponseTypeAnswer,
					Content:      answer,
				}
			}
		}
		if len(choice.Delta.ToolCalls) > 0 {
			thinking.Finish(out)
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

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string            `json:"content"`
			ReasoningContent string            `json:"reasoning_content"`
			Reasoning        string            `json:"reasoning"`
			ToolCalls        []json.RawMessage `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *usage    `json:"usage"`
	Error *apiError `json:"error"`
}

// usage mirrors the token accounting, including the two cache spellings in
// circulation: OpenAI nests cached tokens under prompt_tokens_details, while
// DeepSeek reports hit and miss counters at the top level.
type usage struct {
	PromptTokens          int  `json:"prompt_tokens"`
	CompletionTokens      int  `json:"completion_tokens"`
	TotalTokens           int  `json:"total_tokens"`
	PromptCacheHitTokens  *int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens *int `json:"prompt_cache_miss_tokens"`
	PromptTokensDetails   *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u *usage) normalize() types.TokenUsage {
	if u == nil {
		return types.TokenUsage{}
	}
	out := types.TokenUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}

	read, reported := 0, false
	if u.PromptCacheHitTokens != nil {
		read, reported = *u.PromptCacheHitTokens, true
	} else if u.PromptTokensDetails != nil {
		read, reported = u.PromptTokensDetails.CachedTokens, true
	}
	miss := out.PromptTokens - read
	if u.PromptCacheMissTokens != nil {
		miss = *u.PromptCacheMissTokens
	}
	if miss < 0 {
		miss = 0
	}
	out.SetPromptCacheUsage(read, 0, miss, reported)
	return out
}

// trimEndpoint keeps a caller-provided base URL from producing a doubled path
// when it already names the endpoint.
func trimEndpoint(baseURL, path string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, path) {
		return base
	}
	return base + path
}

// Endpoint reports the full URL for a base URL.
func (d Driver) Endpoint(baseURL string) string {
	return trimEndpoint(baseURL, d.EndpointPath())
}
