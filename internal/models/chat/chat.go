package chat

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

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

// ChatConfig holds configuration for creating a Chat instance.
type ChatConfig struct {
	Source    types.ModelSource
	BaseURL   string
	ModelName string
	APIKey    string
	ModelID   string
	Provider  string
	Extra     map[string]any
}
