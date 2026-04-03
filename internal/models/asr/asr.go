package asr

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

// ASR defines the interface for Automatic Speech Recognition model operations.
type ASR interface {
	// Transcribe sends audio bytes to the ASR model and returns the transcribed text.
	Transcribe(ctx context.Context, audioBytes []byte, fileName string) (string, error)

	GetModelName() string
	GetModelID() string
}

// Config holds the configuration needed to create an ASR instance.
type Config struct {
	Source    types.ModelSource
	BaseURL   string
	ModelName string
	APIKey    string
	ModelID   string
	Language  string // optional: specify language for transcription
}

// NewASR creates an ASR instance based on the provided configuration.
// All ASR vendors use the OpenAI-compatible /v1/audio/transcriptions API.
func NewASR(config *Config) (ASR, error) {
	return NewOpenAIASR(config)
}

// StreamASR defines the interface for real-time streaming ASR.
// Audio chunks are sent via SendAudio, transcription results are received via the Results channel.
type StreamASR interface {
	// Connect establishes the streaming connection to the ASR service.
	Connect(ctx context.Context) error
	// SendAudio sends a chunk of raw PCM16 mono 16kHz audio data.
	SendAudio(ctx context.Context, audioChunk []byte) error
	// Results returns a read-only channel that receives transcription events.
	Results() <-chan TranscriptionEvent
	// Close gracefully closes the streaming connection and releases resources.
	Close() error
}

// TranscriptionEvent represents a streaming transcription result.
type TranscriptionEvent struct {
	Text    string `json:"text"`
	IsFinal bool   `json:"is_final"` // true = confirmed segment, false = interim/partial
	Error   string `json:"error,omitempty"`
}

// StreamProvider identifies the streaming ASR backend.
const (
	StreamProviderOpenAIRealtime  = "openai_realtime"
	StreamProviderFasterWhisper   = "faster_whisper"
)

// StreamConfig holds configuration for streaming ASR providers.
type StreamConfig struct {
	Provider  string // StreamProviderOpenAIRealtime or StreamProviderFasterWhisper
	BaseURL   string // WebSocket URL of the ASR service
	ModelName string
	APIKey    string
	Language  string
}

// NewStreamASR creates a StreamASR instance based on the provider in the config.
func NewStreamASR(config *StreamConfig) (StreamASR, error) {
	switch config.Provider {
	case StreamProviderOpenAIRealtime:
		return NewOpenAIRealtimeASR(config), nil
	case StreamProviderFasterWhisper:
		return NewFasterWhisperStreamASR(config), nil
	default:
		return nil, fmt.Errorf("unsupported stream ASR provider: %s", config.Provider)
	}
}
