package asr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/gorilla/websocket"
)

// FasterWhisperStreamASR implements StreamASR using a faster-whisper-streaming WebSocket service.
type FasterWhisperStreamASR struct {
	config    *StreamConfig
	conn      *websocket.Conn
	resultsCh chan TranscriptionEvent
	done      chan struct{}
	closeOnce sync.Once
}

// NewFasterWhisperStreamASR creates a new FasterWhisperStreamASR instance.
func NewFasterWhisperStreamASR(config *StreamConfig) *FasterWhisperStreamASR {
	return &FasterWhisperStreamASR{
		config:    config,
		resultsCh: make(chan TranscriptionEvent, 100),
		done:      make(chan struct{}),
	}
}

// configMessage is the initial configuration sent after WebSocket connection.
type configMessage struct {
	Type     string `json:"type"`
	Model    string `json:"model"`
	Language string `json:"language"`
}

// serverMessage represents a message received from the faster-whisper service.
type serverMessage struct {
	Type    string `json:"type,omitempty"`
	Text    string `json:"text,omitempty"`
	IsFinal bool   `json:"is_final,omitempty"`
	Message string `json:"message,omitempty"`
}

// Connect establishes the WebSocket connection to the faster-whisper service.
func (f *FasterWhisperStreamASR) Connect(ctx context.Context) error {
	header := http.Header{}
	if f.config.APIKey != "" {
		header.Set("Authorization", "Bearer "+f.config.APIKey)
	}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, f.config.BaseURL, header)
	if err != nil {
		return fmt.Errorf("failed to connect to faster-whisper service at %s: %w", f.config.BaseURL, err)
	}
	f.conn = conn

	logger.Infof(ctx, "[StreamASR/FasterWhisper] Connected to %s", f.config.BaseURL)

	// Send initial config message.
	cfgMsg := configMessage{
		Type:     "config",
		Model:    f.config.ModelName,
		Language: f.config.Language,
	}
	if err := f.conn.WriteJSON(cfgMsg); err != nil {
		f.conn.Close()
		return fmt.Errorf("failed to send config message: %w", err)
	}

	logger.Infof(ctx, "[StreamASR/FasterWhisper] Sent config: model=%s, language=%s", f.config.ModelName, f.config.Language)

	go f.readPump()

	return nil
}

// SendAudio sends a chunk of raw PCM16 audio data as a binary WebSocket message.
func (f *FasterWhisperStreamASR) SendAudio(ctx context.Context, audioChunk []byte) error {
	if f.conn == nil {
		return fmt.Errorf("websocket connection not established; call Connect first")
	}

	if err := f.conn.WriteMessage(websocket.BinaryMessage, audioChunk); err != nil {
		return fmt.Errorf("failed to send audio chunk: %w", err)
	}
	return nil
}

// Results returns a read-only channel that receives transcription events.
func (f *FasterWhisperStreamASR) Results() <-chan TranscriptionEvent {
	return f.resultsCh
}

// Close gracefully shuts down the WebSocket connection and releases resources.
func (f *FasterWhisperStreamASR) Close() error {
	var closeErr error
	f.closeOnce.Do(func() {
		if f.conn != nil {
			// Signal the server that we are done sending audio.
			stopMsg := map[string]string{"type": "stop"}
			if err := f.conn.WriteJSON(stopMsg); err != nil {
				logger.Warnf(context.Background(), "[StreamASR/FasterWhisper] Failed to send stop message: %v", err)
			}

			// Send a proper WebSocket close frame.
			closeMessage := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
			if err := f.conn.WriteMessage(websocket.CloseMessage, closeMessage); err != nil {
				logger.Warnf(context.Background(), "[StreamASR/FasterWhisper] Failed to send close frame: %v", err)
			}

			closeErr = f.conn.Close()
		}
		close(f.done)
	})
	return closeErr
}

// readPump continuously reads messages from the WebSocket and dispatches TranscriptionEvents.
func (f *FasterWhisperStreamASR) readPump() {
	defer close(f.resultsCh)

	for {
		select {
		case <-f.done:
			return
		default:
		}

		_, message, err := f.conn.ReadMessage()
		if err != nil {
			// Don't send error if we're shutting down.
			select {
			case <-f.done:
				return
			default:
			}
			logger.Warnf(context.Background(), "[StreamASR/FasterWhisper] Read error: %v", err)
			f.safeSend(TranscriptionEvent{Error: fmt.Sprintf("websocket read error: %v", err)})
			return
		}

		var msg serverMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			logger.Warnf(context.Background(), "[StreamASR/FasterWhisper] Failed to parse message: %v", err)
			f.safeSend(TranscriptionEvent{Error: fmt.Sprintf("failed to parse server message: %v", err)})
			continue
		}

		switch msg.Type {
		case "error":
			logger.Warnf(context.Background(), "[StreamASR/FasterWhisper] Server error: %s", msg.Message)
			f.safeSend(TranscriptionEvent{Error: msg.Message})
		case "done":
			logger.Infof(context.Background(), "[StreamASR/FasterWhisper] Server signaled done")
			return
		default:
			// Transcription result (text message).
			if msg.Text != "" || msg.IsFinal {
				f.safeSend(TranscriptionEvent{
					Text:    msg.Text,
					IsFinal: msg.IsFinal,
				})
			}
		}
	}
}

// safeSend sends an event to resultsCh without blocking if the channel is full or done is closed.
func (f *FasterWhisperStreamASR) safeSend(event TranscriptionEvent) {
	select {
	case f.resultsCh <- event:
	case <-f.done:
	}
}
