package provider

// RegisterAll registers all built-in providers.
// Call this once during application startup.
func RegisterAll() {
	Register(&GenericProvider{})
	Register(&AliyunProvider{})
	Register(&ZhipuProvider{})
	Register(&VolcengineProvider{})
	Register(&HunyuanProvider{})
	Register(&SiliconFlowProvider{})
	Register(&DeepSeekProvider{})
	Register(&MiniMaxProvider{})
	Register(&MoonshotProvider{})
	Register(&ModelScopeProvider{})
	Register(&QianfanProvider{})
	Register(&QiniuProvider{})
	Register(&OpenAIProvider{})
	Register(&GeminiProvider{})
	Register(&OpenRouterProvider{})
	Register(&JinaProvider{})
	Register(&MimoProvider{})
	Register(&LongCatProvider{})
	Register(&LKEAPProvider{})
	Register(&GPUStackProvider{})
	Register(&NvidiaProvider{})
	Register(&NovitaProvider{})
	Register(&OllamaProvider{})
}
