package asr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/gorilla/websocket"
)

const (
	openaiRealtimeDefaultURL = "wss://api.openai.com/v1/realtime"
)

// OpenAIRealtimeASR implements StreamASR using OpenAI's Realtime API over WebSocket.
type OpenAIRealtimeASR struct {
	config    *StreamConfig
	conn      *websocket.Conn
	resultsCh chan TranscriptionEvent
	done      chan struct{}
	closeOnce sync.Once
}

// NewOpenAIRealtimeASR creates a new OpenAIRealtimeASR instance.
func NewOpenAIRealtimeASR(config *StreamConfig) *OpenAIRealtimeASR {
	return &OpenAIRealtimeASR{
		config:    config,
		resultsCh: make(chan TranscriptionEvent, 100),
		done:      make(chan struct{}),
	}
}

// Connect establishes a WebSocket connection to the OpenAI Realtime API and
// configures the session for audio transcription.
func (o *OpenAIRealtimeASR) Connect(ctx context.Context) error {
	baseURL := o.config.BaseURL
	if baseURL == "" {
		baseURL = openaiRealtimeDefaultURL
	}
	wsURL := baseURL + "?model=" + o.config.ModelName

	header := http.Header{}
	header.Set("Authorization", "Bearer "+o.config.APIKey)
	header.Set("OpenAI-Beta", "realtime=v1")

	logger.Infof(ctx, "[StreamASR] Connecting to OpenAI Realtime API: %s", wsURL)

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("failed to connect to OpenAI Realtime API: %w", err)
	}
	o.conn = conn

	// Build session.update message
	if err := o.sendSessionUpdate(ctx); err != nil {
		o.conn.Close()
		return fmt.Errorf("failed to send session.update: %w", err)
	}

	logger.Infof(ctx, "[StreamASR] Connected and session configured for transcription")

	go o.readPump()

	return nil
}

// sendSessionUpdate sends the session.update message to configure transcription-only mode.
func (o *OpenAIRealtimeASR) sendSessionUpdate(ctx context.Context) error {
	transcriptionConfig := map[string]interface{}{
		"model": "whisper-1",
	}
	if o.config.Language != "" {
		transcriptionConfig["language"] = o.config.Language
	}

	sessionUpdate := map[string]interface{}{
		"type": "session.update",
		"session": map[string]interface{}{
			"modalities":         []string{"text"},
			"input_audio_format": "pcm16",
			"input_audio_transcription": transcriptionConfig,
			"turn_detection": map[string]interface{}{
				"type":                "server_vad",
				"threshold":           0.5,
				"prefix_padding_ms":   300,
				"silence_duration_ms": 500,
			},
		},
	}

	data, err := json.Marshal(sessionUpdate)
	if err != nil {
		return fmt.Errorf("failed to marshal session.update: %w", err)
	}

	logger.Infof(ctx, "[StreamASR] Sending session.update: %s", string(data))

	return o.conn.WriteMessage(websocket.TextMessage, data)
}

// SendAudio sends a chunk of PCM16 audio data to the Realtime API.
func (o *OpenAIRealtimeASR) SendAudio(ctx context.Context, audioChunk []byte) error {
	if o.conn == nil {
		return fmt.Errorf("websocket connection not established; call Connect first")
	}

	encoded := base64.StdEncoding.EncodeToString(audioChunk)

	msg := map[string]string{
		"type":  "input_audio_buffer.append",
		"audio": encoded,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal audio message: %w", err)
	}

	return o.conn.WriteMessage(websocket.TextMessage, data)
}

// readPump continuously reads messages from the WebSocket and dispatches transcription events.
func (o *OpenAIRealtimeASR) readPump() {
	defer func() {
		// Signal that readPump has exited
		select {
		case <-o.done:
		default:
		}
	}()

	for {
		select {
		case <-o.done:
			return
		default:
		}

		_, message, err := o.conn.ReadMessage()
		if err != nil {
			select {
			case <-o.done:
				// Connection closed intentionally; don't send error.
				return
			default:
			}
			o.sendEvent(TranscriptionEvent{
				Error: fmt.Sprintf("websocket read error: %v", err),
			})
			return
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(message, &raw); err != nil {
			continue
		}

		var msgType string
		if t, ok := raw["type"]; ok {
			if err := json.Unmarshal(t, &msgType); err != nil {
				continue
			}
		}

		switch msgType {
		case "conversation.item.input_audio_transcription.completed":
			var transcript string
			if t, ok := raw["transcript"]; ok {
				json.Unmarshal(t, &transcript)
			}
			o.sendEvent(TranscriptionEvent{
				Text:    transcript,
				IsFinal: true,
			})

		case "response.audio_transcript.delta":
			var delta string
			if d, ok := raw["delta"]; ok {
				json.Unmarshal(d, &delta)
			}
			o.sendEvent(TranscriptionEvent{
				Text:    delta,
				IsFinal: false,
			})

		case "response.audio_transcript.done":
			var transcript string
			if t, ok := raw["transcript"]; ok {
				json.Unmarshal(t, &transcript)
			}
			o.sendEvent(TranscriptionEvent{
				Text:    transcript,
				IsFinal: true,
			})

		case "error":
			var errObj map[string]interface{}
			if e, ok := raw["error"]; ok {
				json.Unmarshal(e, &errObj)
			}
			errMsg := "unknown error"
			if errObj != nil {
				if msg, ok := errObj["message"].(string); ok {
					errMsg = msg
				}
			}
			o.sendEvent(TranscriptionEvent{
				Error: errMsg,
			})
		}
	}
}

// sendEvent sends a TranscriptionEvent to the results channel without blocking.
func (o *OpenAIRealtimeASR) sendEvent(event TranscriptionEvent) {
	select {
	case o.resultsCh <- event:
	case <-o.done:
	default:
		// Channel full; drop the event to avoid blocking the read pump.
	}
}

// Results returns a read-only channel that receives transcription events.
func (o *OpenAIRealtimeASR) Results() <-chan TranscriptionEvent {
	return o.resultsCh
}

// Close gracefully closes the WebSocket connection and releases resources.
func (o *OpenAIRealtimeASR) Close() error {
	var closeErr error
	o.closeOnce.Do(func() {
		close(o.done)

		if o.conn != nil {
			// Send a close message to the server.
			closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
			if err := o.conn.WriteMessage(websocket.CloseMessage, closeMsg); err != nil {
				closeErr = fmt.Errorf("failed to send close message: %w", err)
			}
			if err := o.conn.Close(); err != nil && closeErr == nil {
				closeErr = fmt.Errorf("failed to close websocket: %w", err)
			}
		}

		close(o.resultsCh)
	})
	return closeErr
}
