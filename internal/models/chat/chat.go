package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	"github.com/Tencent/WeKnora/internal/models/utils/ollama"
	"github.com/Tencent/WeKnora/internal/types"
)

// The request vocabulary lives in the model-plugin seam (internal/models/llm/spi)
// so protocol drivers can render it without importing this package. These
// aliases keep every existing caller working while leaving exactly one
// definition of a message in the codebase.

// Tool represents a function/tool definition
type Tool = spi.Tool

// FunctionDef represents a function definition
type FunctionDef = spi.FunctionDef

// ChatOptions 聊天选项
type ChatOptions = spi.Options

// MessageContentPart represents a part of multi-content message
type MessageContentPart = spi.MessageContentPart

// ImageURL represents the image URL structure
type ImageURL = spi.ImageURL

// Message 表示聊天消息
type Message = spi.Message

// ToolCall represents a tool call in a message
type ToolCall = spi.ToolCall

// FunctionCall represents a function call
type FunctionCall = spi.FunctionCall

// Chat 定义了聊天接口
type Chat interface {
	// Chat 进行非流式聊天
	Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*types.ChatResponse, error)

	// ChatStream 进行流式聊天
	ChatStream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan types.StreamResponse, error)

	// GetModelName 获取模型名称
	GetModelName() string

	// GetModelID 获取模型ID
	GetModelID() string
}

type ChatConfig struct {
	Source    types.ModelSource
	BaseURL   string
	ModelName string
	APIKey    string
	ModelID   string
	Provider  string
	// MaxConcurrency caps concurrent background calls to this model; 0 falls
	// back to the process-wide default (see limiter.GateN).
	MaxConcurrency int
	ExtraConfig    map[string]string
	// CustomHeaders 允许在调用远程 OpenAI 兼容 API 时附加自定义 HTTP 请求头（类似 OpenAI Python SDK 的 extra_headers）。
	CustomHeaders map[string]string
	AppID         string
	AppSecret     string // 加密值，由工厂函数调用方传入，在 NewWeKnoraCloudChat 中使用前已解密
}

// ConfigFromModel 根据 types.Model 构造 ChatConfig。
// 保证生产路径（service 层根据 DB 中的模型配置拉起实例）和测试路径
// （handler 层根据前端表单临时拉起实例）走完全相同的字段映射，避免重复样板。
// appID / appSecret 是已经解密/解析好的 WeKnoraCloud 凭证，调用方负责传入。
func ConfigFromModel(m *types.Model, appID, appSecret string) *ChatConfig {
	if m == nil {
		return nil
	}
	return &ChatConfig{
		ModelID:        m.ID,
		APIKey:         m.Parameters.APIKey,
		BaseURL:        m.Parameters.BaseURL,
		ModelName:      m.Name,
		Source:         m.Source,
		Provider:       m.Parameters.Provider,
		MaxConcurrency: m.Parameters.MaxConcurrency,
		ExtraConfig:    m.Parameters.ExtraConfig,
		CustomHeaders:  m.Parameters.CustomHeaders,
		AppID:          appID,
		AppSecret:      appSecret,
	}
}

// NewChat 创建聊天实例
func NewChat(config *ChatConfig, ollamaService *ollama.OllamaService) (Chat, error) {
	var c Chat
	var err error
	switch strings.ToLower(string(config.Source)) {
	case string(types.ModelSourceLocal):
		c, err = NewOllamaChat(config, ollamaService)
	case string(types.ModelSourceRemote):
		c, err = NewRemoteChat(config)
	default:
		return nil, fmt.Errorf("unsupported chat model source: %s", config.Source)
	}
	c, err = wrapChatDebug(c, err)
	c, err = wrapChatLangfuse(c, err)
	// Outermost: hold the per-model concurrency slot only around the real
	// provider round-trip, so the wait is excluded from debug/langfuse timing.
	return wrapChatConcurrency(c, config.MaxConcurrency, err)
}

// NewRemoteChat 根据解析出的模型插件创建远程聊天实例。
//
// Protocol selection is the plugin's, not a provider name's: a descriptor
// declares the wire it speaks, so Anthropic Messages and OpenAI Responses go
// through the plugin client that implements them, while the OpenAI-compatible
// majority keeps the established RemoteAPIChat transport. Either way the
// vendor's parameter dispositions come from the same descriptor.
func NewRemoteChat(config *ChatConfig) (Chat, error) {
	desc, ok := resolveDescriptor(config)
	if ok && desc.Protocol != spi.ProtocolOpenAIChat {
		client, resolved, err := NewPluginChat(config)
		if err != nil {
			return nil, err
		}
		if resolved {
			return client, nil
		}
	}
	return NewRemoteAPIChat(config)
}
