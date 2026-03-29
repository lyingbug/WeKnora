package embedding

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

// Embedder defines the interface for text vectorization
type Embedder interface {
	// Embed converts text to vector
	Embed(ctx context.Context, text string) ([]float32, error)

	// BatchEmbed converts multiple texts to vectors in batch
	BatchEmbed(ctx context.Context, texts []string) ([][]float32, error)

	// GetModelName returns the model name
	GetModelName() string

	// GetDimensions returns the vector dimensions
	GetDimensions() int

	// GetModelID returns the model ID
	GetModelID() string

	EmbedderPooler
}

type EmbedderPooler interface {
	BatchEmbedWithPool(ctx context.Context, model Embedder, texts []string) ([][]float32, error)
}

// EmbedderType represents the embedder type
type EmbedderType string

// embedSingle calls batchFn with a single text, retrying up to 3 times if the result is empty.
func embedSingle(ctx context.Context, text string, batchFn func(context.Context, []string) ([][]float32, error)) ([]float32, error) {
	for range 3 {
		embeddings, err := batchFn(ctx, []string{text})
		if err != nil {
			return nil, err
		}
		if len(embeddings) > 0 {
			return embeddings[0], nil
		}
	}
	return nil, fmt.Errorf("no embedding returned")
}

// Config represents the embedder configuration
type Config struct {
	Source               types.ModelSource `json:"source"`
	BaseURL              string            `json:"base_url"`
	ModelName            string            `json:"model_name"`
	APIKey               string            `json:"api_key"`
	TruncatePromptTokens int               `json:"truncate_prompt_tokens"`
	Dimensions           int               `json:"dimensions"`
	ModelID              string            `json:"model_id"`
	Provider             string            `json:"provider"`
}
