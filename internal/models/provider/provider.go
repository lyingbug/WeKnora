// Package provider defines the unified interface and registry for multi-vendor model API adapters.
package provider

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/types"
)

// ProviderName 模型服务商名称
type ProviderName string

const (
	// OpenAI
	ProviderOpenAI ProviderName = "openai"
	// 阿里云 DashScope
	ProviderAliyun ProviderName = "aliyun"
	// 智谱AI (GLM 系列)
	ProviderZhipu ProviderName = "zhipu"
	// OpenRouter
	ProviderOpenRouter ProviderName = "openrouter"
	// 硅基流动
	ProviderSiliconFlow ProviderName = "siliconflow"
	// Jina AI (Embedding and Rerank)
	ProviderJina ProviderName = "jina"
	// Generic 兼容OpenAI (自定义部署)
	ProviderGeneric ProviderName = "generic"
	// DeepSeek
	ProviderDeepSeek ProviderName = "deepseek"
	// Google Gemini
	ProviderGemini ProviderName = "gemini"
	// 火山引擎 Ark
	ProviderVolcengine ProviderName = "volcengine"
	// 腾讯混元
	ProviderHunyuan ProviderName = "hunyuan"
	// MiniMax
	ProviderMiniMax ProviderName = "minimax"
	// 小米 Mimo
	ProviderMimo ProviderName = "mimo"
	// GPUStack (私有化部署)
	ProviderGPUStack ProviderName = "gpustack"
	// 月之暗面 Moonshot (Kimi)
	ProviderMoonshot ProviderName = "moonshot"
	// 魔搭 ModelScope
	ProviderModelScope ProviderName = "modelscope"
	// 百度千帆
	ProviderQianfan ProviderName = "qianfan"
	// 七牛云
	ProviderQiniu ProviderName = "qiniu"
	// 美团 LongCat AI
	ProviderLongCat ProviderName = "longcat"
	// 腾讯云 LKEAP (知识引擎原子能力)
	ProviderLKEAP ProviderName = "lkeap"
	// NVIDIA
	ProviderNvidia ProviderName = "nvidia"
	// Novita AI
	ProviderNovita ProviderName = "novita"
	// Ollama (本地模型)
	ProviderOllama ProviderName = "ollama"
)

// Common validation errors
var (
	ErrModelNameRequired = fmt.Errorf("model name is required")
	ErrAPIKeyRequired    = fmt.Errorf("API key is required")
	ErrBaseURLRequired   = fmt.Errorf("base URL is required")
)

// validateRequired checks common required fields on a provider config.
func validateRequired(config *Config, needBaseURL, needAPIKey, needModelName bool) error {
	if needBaseURL && config.BaseURL == "" {
		return ErrBaseURLRequired
	}
	if needAPIKey && config.APIKey == "" {
		return ErrAPIKeyRequired
	}
	if needModelName && config.ModelName == "" {
		return ErrModelNameRequired
	}
	return nil
}

// AllProviders 返回所有注册的提供者名称
func AllProviders() []ProviderName {
	return []ProviderName{
		ProviderGeneric,
		ProviderAliyun,
		ProviderZhipu,
		ProviderVolcengine,
		ProviderHunyuan,
		ProviderSiliconFlow,
		ProviderDeepSeek,
		ProviderMiniMax,
		ProviderMoonshot,
		ProviderModelScope,
		ProviderQianfan,
		ProviderQiniu,
		ProviderOpenAI,
		ProviderGemini,
		ProviderOpenRouter,
		ProviderJina,
		ProviderMimo,
		ProviderLongCat,
		ProviderLKEAP,
		ProviderGPUStack,
		ProviderNvidia,
		ProviderNovita,
		ProviderOllama,
	}
}

// DetectProvider identifies the provider by matching the baseURL against registered URLPatterns.
// Falls back to Generic if no URLPatterns match.
func DetectProvider(baseURL string) ProviderName {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, name := range AllProviders() {
		if p, ok := registry[name]; ok {
			for _, pattern := range p.Info().URLPatterns {
				if strings.Contains(baseURL, pattern) {
					return name
				}
			}
		}
	}
	return ProviderGeneric
}

// ModelEntry describes a single model offered by a provider.
type ModelEntry struct {
	ID          string          `json:"id"`
	DisplayName string          `json:"display_name"`
	ModelType   types.ModelType `json:"model_type"`
	Description string          `json:"description"`
	Tags        []string        `json:"tags,omitempty"`
}

// ProviderInfo 包含提供者的元数据
type ProviderInfo struct {
	Name         ProviderName               // 提供者标识
	DisplayName  string                     // 可读名称
	Description  string                     // 提供者描述
	DocURL       string                     // 官方 API 文档地址
	DefaultURLs  map[types.ModelType]string // 按模型类型区分的默认 BaseURL
	ModelTypes   []types.ModelType          // 支持的模型类型
	RequiresAuth bool                       // 是否需要 API key
	ExtraFields  []ExtraFieldConfig         // 额外配置字段
	Models       []ModelEntry               // Structured model catalog
	URLPatterns  []string                   // URL detection patterns (data-driven)
}

// GetDefaultURL 获取指定模型类型的默认 URL
func (p ProviderInfo) GetDefaultURL(modelType types.ModelType) string {
	if url, ok := p.DefaultURLs[modelType]; ok {
		return url
	}
	// 回退到 Chat URL
	if url, ok := p.DefaultURLs[types.ModelTypeKnowledgeQA]; ok {
		return url
	}
	return ""
}

// ExtraFieldConfig 定义提供者的额外配置字段
type ExtraFieldConfig struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "string", "number", "boolean", "select"
	Required    bool   `json:"required"`
	Default     string `json:"default"`
	Placeholder string `json:"placeholder"`
	Options     []struct {
		Label string `json:"label"`
		Value string `json:"value"`
	} `json:"options,omitempty"`
}

// Config 表示模型提供者的配置
type Config struct {
	Provider  ProviderName   `json:"provider"`
	BaseURL   string         `json:"base_url"`
	APIKey    string         `json:"api_key"`
	ModelName string         `json:"model_name"`
	ModelID   string         `json:"model_id"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// Provider is the interface that every model provider must implement.
type Provider interface {
	// Info 返回服务商的元数据
	Info() ProviderInfo

	// ValidateConfig 验证服务商的配置
	ValidateConfig(config *Config) error

	// Adapter returns provider-specific adapter rules for non-OpenAI-compatible behavior.
	// Returns nil for providers that are fully OpenAI-compatible (the default via BaseProvider).
	Adapter() *ProviderAdapter
}

// BaseProvider provides a default nil Adapter() implementation.
// Embed this in providers that are fully OpenAI-compatible and need no adapter rules.
type BaseProvider struct{}

func (BaseProvider) Adapter() *ProviderAdapter { return nil }

// registry 存储所有注册的提供者
var (
	registryMu sync.RWMutex
	registry   = make(map[ProviderName]Provider)
)

// Register 添加一个提供者到全局注册表
func Register(p Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[p.Info().Name] = p
}

// Get 通过名称从注册表中获取提供者
func Get(name ProviderName) (Provider, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[name]
	return p, ok
}

// GetOrDefault 通过名称从注册表中获取提供者，如果未找到则返回默认提供者
func GetOrDefault(name ProviderName) Provider {
	p, ok := Get(name)
	if ok {
		return p
	}
	// 如果未找到则返回默认提供者
	p, _ = Get(ProviderGeneric)
	return p
}

// List 返回所有注册的提供者（按 AllProviders 定义的顺序）
func List() []ProviderInfo {
	registryMu.RLock()
	defer registryMu.RUnlock()

	result := make([]ProviderInfo, 0, len(registry))
	for _, name := range AllProviders() {
		if p, ok := registry[name]; ok {
			result = append(result, p.Info())
		}
	}
	return result
}

// ListByModelType 返回所有支持指定模型类型的提供者（按 AllProviders 定义的顺序）
func ListByModelType(modelType types.ModelType) []ProviderInfo {
	registryMu.RLock()
	defer registryMu.RUnlock()

	result := make([]ProviderInfo, 0)
	for _, name := range AllProviders() {
		if p, ok := registry[name]; ok {
			info := p.Info()
			for _, t := range info.ModelTypes {
				if t == modelType {
					result = append(result, info)
					break
				}
			}
		}
	}
	return result
}

// ResolveProvider returns the explicitly-specified provider, or auto-detects from the baseURL.
func ResolveProvider(explicit string, baseURL string) ProviderName {
	if explicit != "" {
		return ProviderName(explicit)
	}
	return DetectProvider(baseURL)
}

// ResolveProviderWithSource resolves the provider, taking the model source into account.
// If source is "local", the provider is always Ollama (backward compatibility).
func ResolveProviderWithSource(explicit string, baseURL string, source string) ProviderName {
	if source == string(types.ModelSourceLocal) {
		return ProviderOllama
	}
	return ResolveProvider(explicit, baseURL)
}

// IsOllama returns true if the provider is Ollama (local models).
func IsOllama(name ProviderName) bool {
	return name == ProviderOllama
}

// GetAdapter returns the ProviderAdapter for the named provider, or nil
// if the provider is not found or has no adapter rules.
func GetAdapter(name ProviderName) *ProviderAdapter {
	p, ok := Get(name)
	if !ok {
		return nil
	}
	return p.Adapter()
}

// NewConfigFromModel creates a Config from a types.Model.
func NewConfigFromModel(model *types.Model) (*Config, error) {
	if model == nil {
		return nil, fmt.Errorf("model is nil")
	}

	providerName := ResolveProvider(model.Parameters.Provider, model.Parameters.BaseURL)

	return &Config{
		Provider:  providerName,
		BaseURL:   model.Parameters.BaseURL,
		APIKey:    model.Parameters.APIKey,
		ModelName: model.Name,
		ModelID:   model.ID,
	}, nil
}
