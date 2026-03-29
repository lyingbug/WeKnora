package chat

import "encoding/json"

// Tool represents a function/tool definition
type Tool struct {
	Type     string      `json:"type"` // "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef represents a function definition
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ChatOptions 聊天选项
type ChatOptions struct {
	Temperature         float64         `json:"temperature"`                   // 温度参数
	TopP                float64         `json:"top_p"`                         // Top P 参数
	Seed                int             `json:"seed"`                          // 随机种子
	MaxTokens           int             `json:"max_tokens"`                    // 最大 token 数
	MaxCompletionTokens int             `json:"max_completion_tokens"`         // 最大完成 token 数
	FrequencyPenalty    float64         `json:"frequency_penalty"`             // 频率惩罚
	PresencePenalty     float64         `json:"presence_penalty"`              // 存在惩罚
	Thinking            *bool           `json:"thinking"`                      // 是否启用思考
	Tools               []Tool          `json:"tools,omitempty"`               // 可用工具列表
	ToolChoice          string          `json:"tool_choice,omitempty"`         // "auto", "required", "none", or specific tool
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"` // 是否允许并行工具调用（默认 nil 表示由模型决定）
	Format              json.RawMessage `json:"format,omitempty"`              // 响应格式定义
}

// MessageContentPart represents a part of multi-content message
type MessageContentPart struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // For type="text"
	ImageURL *ImageURL `json:"image_url,omitempty"` // For type="image_url"
}

// ImageURL represents the image URL structure
type ImageURL struct {
	URL    string `json:"url"`              // URL or base64 data URI
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// Message 表示聊天消息
type Message struct {
	Role         string               `json:"role"`                    // 角色：system, user, assistant, tool
	Content      string               `json:"content"`                 // 消息内容
	MultiContent []MessageContentPart `json:"multi_content,omitempty"` // 多内容消息（文本+图片）
	Name         string               `json:"name,omitempty"`          // Function/tool name (for tool role)
	ToolCallID   string               `json:"tool_call_id,omitempty"`  // Tool call ID (for tool role)
	ToolCalls    []ToolCall           `json:"tool_calls,omitempty"`    // Tool calls (for assistant role)
	Images       []string             `json:"images,omitempty"`        // Image URLs for multimodal (only for current user message)
}

// ToolCall represents a tool call in a message
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// GetThinking returns the Thinking option (implements provider.ChatOptsAccessor).
func (o *ChatOptions) GetThinking() *bool {
	if o == nil {
		return nil
	}
	return o.Thinking
}

// GetToolChoice returns the ToolChoice option (implements provider.ChatOptsAccessor).
func (o *ChatOptions) GetToolChoice() string {
	if o == nil {
		return ""
	}
	return o.ToolChoice
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}
