package provider

import "github.com/Tencent/WeKnora/internal/types"

// ListModels returns all models of the given type offered by the named provider.
func ListModels(name ProviderName, mt types.ModelType) []ModelEntry {
	p, ok := Get(name)
	if !ok {
		return nil
	}
	var result []ModelEntry
	for _, m := range p.Info().Models {
		if m.ModelType == mt {
			result = append(result, m)
		}
	}
	return result
}

// ListAllModels returns models of the given type across all registered providers.
func ListAllModels(mt types.ModelType) map[ProviderName][]ModelEntry {
	result := make(map[ProviderName][]ModelEntry)
	for _, name := range AllProviders() {
		models := ListModels(name, mt)
		if len(models) > 0 {
			result[name] = models
		}
	}
	return result
}
