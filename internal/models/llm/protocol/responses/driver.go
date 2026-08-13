// Package responses implements the OpenAI Responses protocol.
//
// Responses is not a renamed Chat Completions. Input is a list of typed items
// rather than messages, reasoning is an output item of its own instead of a
// field beside the content, tool calls are top-level items rather than a
// property of an assistant message, and the stream is a sequence of named
// events rather than deltas on a choice.
package responses

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

// Driver implements protocol.Driver for the Responses API.
type Driver struct{}

func init() { protocol.Register(Driver{}) }

// ID reports the protocol.
func (Driver) ID() spi.ProtocolID { return spi.ProtocolOpenAIResponses }

// EndpointPath reports the standard path.
func (Driver) EndpointPath() string { return "/responses" }

// BuildDraft renders the call into a Responses body.
func (d Driver) BuildDraft(call protocol.Call) (*spi.Draft, error) {
	draft := spi.NewDraft(d.ID(), call.Model, call.Stream)
	draft.Set("model", call.Model)
	if call.Stream {
		draft.Set("stream", true)
	}

	instructions, input := convertInput(call.Messages)
	if instructions != "" {
		draft.Set("instructions", instructions)
	}
	draft.Set("input", input)

	// Server-side conversation state is opt-in, and storing prompts on the
	// vendor's side is a decision for the deployment rather than a default.
	draft.Set("store", false)

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
	if maxTokens := opts.EffectiveMaxTokens(); maxTokens > 0 {
		draft.Set("max_output_tokens", maxTokens)
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
	return draft, nil
}

// convertInput splits the system prompt into `instructions` and renders the
// rest as input items.
func convertInput(messages []spi.Message) (string, []any) {
	var instructions []string
	items := make([]any, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if text := plainText(msg); text != "" {
				instructions = append(instructions, text)
			}

		case "tool":
			// A tool result is its own item referencing the call it answers.
			items = append(items, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  msg.Content,
			})

		case "assistant":
			if content := contentParts(msg, "output_text"); len(content) > 0 {
				items = append(items, map[string]any{
					"type":    "message",
					"role":    "assistant",
					"content": content,
				})
			}
			for _, call := range msg.ToolCalls {
				items = append(items, map[string]any{
					"type":      "function_call",
					"call_id":   call.ID,
					"name":      call.Function.Name,
					"arguments": call.Function.Arguments,
				})
			}

		default:
			if content := contentParts(msg, "input_text"); len(content) > 0 {
				items = append(items, map[string]any{
					"type":    "message",
					"role":    "user",
					"content": content,
				})
			}
		}
	}
	return strings.Join(instructions, "\n\n"), items
}

// contentParts renders a message body as Responses content parts. The text
// part is named differently on input and output items, which is why the caller
// passes the type in.
func contentParts(msg spi.Message, textType string) []any {
	var parts []any
	for _, part := range msg.MultiContent {
		switch part.Type {
		case "text":
			if part.Text != "" {
				parts = append(parts, map[string]any{"type": textType, "text": part.Text})
			}
		case "image_url":
			if part.ImageURL != nil && part.ImageURL.URL != "" {
				parts = append(parts, map[string]any{
					"type":      "input_image",
					"image_url": part.ImageURL.URL,
				})
			}
		}
	}
	for _, img := range msg.Images {
		if img != "" {
			parts = append(parts, map[string]any{"type": "input_image", "image_url": img})
		}
	}
	if msg.Content != "" {
		parts = append(parts, map[string]any{"type": textType, "text": msg.Content})
	}
	return parts
}

func plainText(msg spi.Message) string {
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

// convertTools renders tools in the flat shape Responses uses, where the
// function fields sit on the tool itself rather than in a nested object.
func convertTools(tools []spi.Tool) []any {
	out := make([]any, 0, len(tools))
	for _, tool := range tools {
		var params any
		if len(tool.Function.Parameters) > 0 {
			_ = json.Unmarshal(tool.Function.Parameters, &params)
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"parameters":  params,
		})
	}
	return out
}

// response mirrors a complete Responses body.
type response struct {
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Summary []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"summary"`
		// Reasoning text is exposed directly by some deployments and only as a
		// summary by others.
		Text      string `json:"text"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"output"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Usage *usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ParseResponse decodes a complete Responses body.
func (d Driver) ParseResponse(body []byte) (*types.ChatResponse, error) {
	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return nil, fmt.Errorf("provider error: %s", resp.Error.Message)
	}

	out := &types.ChatResponse{Usage: resp.Usage.normalize()}
	var text, reasoning []string

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" {
					text = append(text, part.Text)
				}
			}
		case "reasoning":
			if item.Text != "" {
				reasoning = append(reasoning, item.Text)
			}
			for _, part := range item.Summary {
				if part.Text != "" {
					reasoning = append(reasoning, part.Text)
				}
			}
		case "function_call":
			out.ToolCalls = append(out.ToolCalls, types.LLMToolCall{
				ID:       item.CallID,
				Type:     "function",
				Function: types.FunctionCall{Name: item.Name, Arguments: item.Arguments},
			})
		}
	}

	out.Content = strings.Join(text, "")
	out.ReasoningContent = strings.Join(reasoning, "")
	// A response can stop on the output ceiling before producing visible text,
	// and reporting that as a normal completion hides why the answer is empty.
	switch {
	case resp.IncompleteDetails != nil && resp.IncompleteDetails.Reason != "":
		out.FinishReason = resp.IncompleteDetails.Reason
	case len(out.ToolCalls) > 0:
		out.FinishReason = "tool_calls"
	case resp.Status == "completed":
		out.FinishReason = "stop"
	default:
		out.FinishReason = resp.Status
	}
	return out, nil
}

// streamEvent mirrors the typed events this driver consumes. Responses names
// roughly forty event types; the ones absent here carry no information this
// seam's neutral stream can express.
type streamEvent struct {
	Type     string    `json:"type"`
	Delta    string    `json:"delta"`
	Text     string    `json:"text"`
	Response *response `json:"response"`
	Item     *struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// DecodeStream decodes a Responses SSE stream.
func (d Driver) DecodeStream(ctx context.Context, body io.Reader, out chan<- types.StreamResponse) {
	reader := sse.NewReader(body)
	thinking := &emit.Thinking{}

	var (
		acc          *types.TokenUsage
		finishReason string
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

		var ev streamEvent
		if err := json.Unmarshal(event.Data, &ev); err != nil {
			emit.Error(out, fmt.Sprintf("decode stream event: %v", err))
			return
		}
		// The event name and the payload type agree, but only the payload is
		// guaranteed present on every deployment.
		kind := ev.Type
		if kind == "" {
			kind = event.Name
		}

		switch kind {
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			thinking.Emit(out, ev.Delta)

		case "response.output_text.delta":
			thinking.Finish(out)
			if ev.Delta != "" {
				out <- types.StreamResponse{
					ResponseType: types.ResponseTypeAnswer,
					Content:      ev.Delta,
				}
			}

		case "response.output_item.done":
			if ev.Item != nil && ev.Item.Type == "function_call" {
				thinking.Finish(out)
				finishReason = "tool_calls"
			}

		case "response.completed", "response.incomplete", "response.failed":
			if ev.Response != nil {
				if u := ev.Response.Usage.normalize(); u.TotalTokens > 0 {
					acc = &u
				}
				if ev.Response.IncompleteDetails != nil && ev.Response.IncompleteDetails.Reason != "" {
					finishReason = ev.Response.IncompleteDetails.Reason
				} else if finishReason == "" && ev.Response.Status == "completed" {
					finishReason = "stop"
				}
				if ev.Response.Error != nil && ev.Response.Error.Message != "" {
					emit.Error(out, ev.Response.Error.Message)
					return
				}
			}

		case "error":
			if ev.Error != nil {
				emit.Error(out, ev.Error.Message)
				return
			}
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

// usage mirrors the Responses token accounting, which names its fields for
// input and output rather than prompt and completion.
type usage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func (u *usage) normalize() types.TokenUsage {
	if u == nil {
		return types.TokenUsage{}
	}
	out := types.TokenUsage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	read, reported := 0, false
	if u.InputTokensDetails != nil {
		read, reported = u.InputTokensDetails.CachedTokens, true
	}
	miss := out.PromptTokens - read
	if miss < 0 {
		miss = 0
	}
	out.SetPromptCacheUsage(read, 0, miss, reported)
	return out
}

// Endpoint reports the full URL for a base URL.
func (d Driver) Endpoint(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, d.EndpointPath()) {
		return base
	}
	return base + d.EndpointPath()
}
