package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ASRStreamHandler handles WebSocket connections for real-time streaming ASR.
type ASRStreamHandler struct {
	kbService    interfaces.KnowledgeBaseService
	modelService interfaces.ModelService
}

// NewASRStreamHandler creates a new ASR stream handler instance.
func NewASRStreamHandler(
	kbService interfaces.KnowledgeBaseService,
	modelService interfaces.ModelService,
) *ASRStreamHandler {
	return &ASRStreamHandler{
		kbService:    kbService,
		modelService: modelService,
	}
}

// wsUpgrader is the WebSocket upgrader with permissive origin check (auth is handled by middleware).
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  16 * 1024,
	WriteBufferSize: 16 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // CORS is handled by middleware
	},
}

// clientMessage represents a text message from the browser WebSocket client.
type clientMessage struct {
	Type     string `json:"type"`               // "stop", "config"
	Language string `json:"language,omitempty"`  // optional language override
}

// serverMessage represents a text message sent to the browser WebSocket client.
type serverMessage struct {
	Type    string `json:"type"`              // "ready", "transcript", "error", "done"
	Text    string `json:"text,omitempty"`
	IsFinal bool   `json:"is_final,omitempty"`
	Message string `json:"message,omitempty"` // for error type
}

// HandleASRStream godoc
// @Summary      Streaming ASR via WebSocket
// @Description  Upgrades to WebSocket, receives audio chunks, forwards to ASR service, returns real-time transcription
// @Tags         ASR
// @Param        id  path  string  true  "Knowledge Base ID"
// @Success      101 "Switching Protocols"
// @Security     Bearer
// @Router       /knowledge-bases/{id}/asr/stream [get]
func (h *ASRStreamHandler) HandleASRStream(c *gin.Context) {
	ctx := c.Request.Context()

	// 1. Validate knowledge base access and ASR config
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID is required"})
		return
	}

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get knowledge base"})
		return
	}
	if kb == nil || kb.TenantID != tenantID {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}

	if !kb.ASRConfig.IsStreamASREnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "streaming ASR is not enabled for this knowledge base"})
		return
	}

	// 2. Build StreamConfig from KB's ASR configuration and the referenced model
	streamCfg, err := h.buildStreamConfig(ctx, kb)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. Upgrade to WebSocket
	wsConn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Errorf(ctx, "[ASR-Stream] WebSocket upgrade failed: %v", err)
		return // Upgrade already sent error response
	}
	defer wsConn.Close()

	logger.Infof(ctx, "[ASR-Stream] WebSocket connected for KB=%s, provider=%s", kbID, streamCfg.Provider)

	// 4. Create and connect StreamASR
	streamASR, err := asr.NewStreamASR(streamCfg)
	if err != nil {
		logger.Errorf(ctx, "[ASR-Stream] Failed to create StreamASR: %v", err)
		writeWSMessage(wsConn, serverMessage{Type: "error", Message: "failed to create ASR: " + err.Error()})
		return
	}

	if err := streamASR.Connect(ctx); err != nil {
		logger.Errorf(ctx, "[ASR-Stream] Failed to connect to ASR service: %v", err)
		writeWSMessage(wsConn, serverMessage{Type: "error", Message: "failed to connect to ASR service: " + err.Error()})
		return
	}
	defer streamASR.Close()

	// 5. Send "ready" to client
	writeWSMessage(wsConn, serverMessage{Type: "ready"})

	// 6. Bridge: browser audio ↔ ASR service
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Goroutine A: Read audio from browser WebSocket → forward to ASR
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}

			msgType, data, err := wsConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					logger.Warnf(ctx, "[ASR-Stream] WebSocket read error: %v", err)
				}
				return
			}

			switch msgType {
			case websocket.BinaryMessage:
				// Audio chunk (PCM16 from browser AudioWorklet)
				if err := streamASR.SendAudio(ctx, data); err != nil {
					logger.Warnf(ctx, "[ASR-Stream] SendAudio error: %v", err)
					return
				}

			case websocket.TextMessage:
				// Control message
				var msg clientMessage
				if err := json.Unmarshal(data, &msg); err != nil {
					logger.Warnf(ctx, "[ASR-Stream] Invalid text message: %v", err)
					continue
				}
				switch msg.Type {
				case "stop":
					logger.Infof(ctx, "[ASR-Stream] Client requested stop")
					streamASR.Close()
					return
				}
			}
		}
	}()

	// Goroutine B: Read ASR results → write to browser WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		for evt := range streamASR.Results() {
			if evt.Error != "" {
				writeWSMessage(wsConn, serverMessage{Type: "error", Message: evt.Error})
				continue
			}
			writeWSMessage(wsConn, serverMessage{
				Type:    "transcript",
				Text:    evt.Text,
				IsFinal: evt.IsFinal,
			})
		}
		// ASR results channel closed → send "done"
		writeWSMessage(wsConn, serverMessage{Type: "done"})
	}()

	// Wait for both goroutines to finish
	wg.Wait()
	close(done)

	logger.Infof(ctx, "[ASR-Stream] Session ended for KB=%s", kbID)
}

// buildStreamConfig constructs StreamConfig from the knowledge base's ASR settings and the referenced model.
func (h *ASRStreamHandler) buildStreamConfig(ctx context.Context, kb *types.KnowledgeBase) (*asr.StreamConfig, error) {
	cfg := &asr.StreamConfig{
		Provider: kb.ASRConfig.StreamProvider,
		Language: kb.ASRConfig.Language,
	}

	switch kb.ASRConfig.StreamProvider {
	case asr.StreamProviderOpenAIRealtime:
		// For OpenAI Realtime, we need the API key from the ASR model
		if kb.ASRConfig.ModelID == "" {
			return nil, errors.NewBadRequestError("ASR model ID is required for OpenAI Realtime streaming")
		}
		asrModel, err := h.modelService.GetASRModel(ctx, kb.ASRConfig.ModelID)
		if err != nil {
			return nil, err
		}
		// Extract API key and base URL from the ASR model
		if oai, ok := asrModel.(*asr.OpenAIASR); ok {
			cfg.APIKey = oai.GetAPIKey()
			cfg.BaseURL = oai.GetBaseURL()
		}
		cfg.ModelName = asrModel.GetModelName()

	case asr.StreamProviderFasterWhisper:
		// For faster-whisper, the stream URL is in the ASR config
		if kb.ASRConfig.StreamURL == "" {
			return nil, errors.NewBadRequestError("stream_url is required for faster-whisper streaming")
		}
		cfg.BaseURL = kb.ASRConfig.StreamURL
		// Optionally get model name from the ASR model config
		if kb.ASRConfig.ModelID != "" {
			asrModel, err := h.modelService.GetASRModel(ctx, kb.ASRConfig.ModelID)
			if err == nil {
				cfg.ModelName = asrModel.GetModelName()
			}
		}

	default:
		return nil, errors.NewBadRequestError("unsupported stream provider: " + kb.ASRConfig.StreamProvider)
	}

	return cfg, nil
}

// writeWSMessage marshals and writes a JSON message to the WebSocket connection.
func writeWSMessage(conn *websocket.Conn, msg serverMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	conn.WriteMessage(websocket.TextMessage, data)
}
