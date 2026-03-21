package chatpipline

import (
	"context"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

const imageAnalysisPrompt = `You are an image analysis assistant. Analyze the provided image(s) thoroughly.

Your task:
1. Describe the visual content: objects, scene, layout, relationships, and key details
2. If the image contains any text, perform complete OCR and include ALL visible text
3. If both visual content and text exist, include both in your response

Output a plain text description. Be thorough and complete, especially for any text/OCR content.
Do NOT output JSON. Just output the description directly.`

// PluginImageAnalysis performs standalone VLM-based image analysis.
// It sends images to a vision-capable model and stores the description
// in chatManage.ImageDescription.
type PluginImageAnalysis struct {
	modelService   interfaces.ModelService
	messageService interfaces.MessageService
	config         *config.Config
}

// NewPluginImageAnalysis creates and registers a new PluginImageAnalysis instance.
func NewPluginImageAnalysis(eventManager *EventManager,
	modelService interfaces.ModelService, messageService interfaces.MessageService,
	config *config.Config,
) *PluginImageAnalysis {
	res := &PluginImageAnalysis{
		modelService:   modelService,
		messageService: messageService,
		config:         config,
	}
	eventManager.Register(res)
	return res
}

// ActivationEvents returns the event types this plugin handles.
func (p *PluginImageAnalysis) ActivationEvents() []types.EventType {
	return []types.EventType{types.IMAGE_ANALYSIS}
}

// OnEvent processes the IMAGE_ANALYSIS event.
// It sends images to a VLM for analysis and stores the result.
func (p *PluginImageAnalysis) OnEvent(ctx context.Context,
	eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	if len(chatManage.Images) == 0 {
		pipelineInfo(ctx, "ImageAnalysis", "skip", map[string]interface{}{
			"session_id": chatManage.SessionID,
			"reason":     "no_images",
		})
		return next()
	}

	pipelineInfo(ctx, "ImageAnalysis", "start", map[string]interface{}{
		"session_id":  chatManage.SessionID,
		"image_count": len(chatManage.Images),
	})

	// Select a vision-capable model
	vlmModel := p.selectVLM(ctx, chatManage)
	if vlmModel == nil {
		pipelineWarn(ctx, "ImageAnalysis", "no_vlm", map[string]interface{}{
			"session_id": chatManage.SessionID,
		})
		return next()
	}

	// Build user message with image context
	userContent := "Please analyze the following image(s)."
	if q := strings.TrimSpace(chatManage.Query); q != "" {
		userContent = "User's question: " + q + "\n\nPlease analyze the attached image(s) in the context of this question."
	}

	// Emit progress event
	var toolCallID string
	if chatManage.EventBus != nil {
		toolCallID = uuid.New().String()
		chatManage.EventBus.Emit(ctx, types.Event{
			Type:      types.EventType(event.EventAgentToolCall),
			SessionID: chatManage.SessionID,
			Data: event.AgentToolCallData{
				ToolCallID: toolCallID,
				ToolName:   "image_analysis",
			},
		})
	}

	thinking := false
	vlmStart := time.Now()
	response, err := vlmModel.Chat(ctx, []chat.Message{
		{Role: "system", Content: imageAnalysisPrompt},
		{Role: "user", Content: userContent, Images: chatManage.Images},
	}, &chat.ChatOptions{
		Temperature:         0.3,
		MaxCompletionTokens: 500,
		Thinking:            &thinking,
	})
	if err != nil {
		if toolCallID != "" && chatManage.EventBus != nil {
			chatManage.EventBus.Emit(ctx, types.Event{
				Type:      types.EventType(event.EventAgentToolResult),
				SessionID: chatManage.SessionID,
				Data: event.AgentToolResultData{
					ToolCallID: toolCallID,
					ToolName:   "image_analysis",
					Output:     "图片分析失败",
					Success:    false,
					Duration:   time.Since(vlmStart).Milliseconds(),
				},
			})
		}
		pipelineError(ctx, "ImageAnalysis", "vlm_call", map[string]interface{}{
			"session_id": chatManage.SessionID,
			"error":      err.Error(),
		})
		return next()
	}

	// Emit completion event
	if toolCallID != "" && chatManage.EventBus != nil {
		chatManage.EventBus.Emit(ctx, types.Event{
			Type:      types.EventType(event.EventAgentToolResult),
			SessionID: chatManage.SessionID,
			Data: event.AgentToolResultData{
				ToolCallID: toolCallID,
				ToolName:   "image_analysis",
				Output:     "已分析图片内容",
				Success:    true,
				Duration:   time.Since(vlmStart).Milliseconds(),
			},
		})
	}

	chatManage.ImageDescription = strings.TrimSpace(response.Content)

	// Persist image description back to the user message
	if chatManage.ImageDescription != "" && chatManage.UserMessageID != "" {
		p.updateUserMessageImageCaption(ctx, chatManage)
	}

	pipelineInfo(ctx, "ImageAnalysis", "done", map[string]interface{}{
		"session_id":    chatManage.SessionID,
		"desc_len":      len(chatManage.ImageDescription),
		"vlm_duration":  time.Since(vlmStart).Milliseconds(),
	})

	return next()
}

// selectVLM picks a vision-capable model for image analysis.
func (p *PluginImageAnalysis) selectVLM(ctx context.Context, chatManage *types.ChatManage) chat.Chat {
	// Prefer the chat model if it supports vision
	if chatManage.ChatModelSupportsVision {
		m, err := p.modelService.GetChatModel(ctx, chatManage.ChatModelID)
		if err == nil {
			return m
		}
		pipelineWarn(ctx, "ImageAnalysis", "vision_model_fallback", map[string]interface{}{
			"session_id": chatManage.SessionID,
			"error":      err.Error(),
		})
	}

	// Fall back to dedicated VLM
	if chatManage.VLMModelID != "" {
		m, err := p.modelService.GetChatModel(ctx, chatManage.VLMModelID)
		if err == nil {
			return m
		}
		pipelineWarn(ctx, "ImageAnalysis", "vlm_fallback", map[string]interface{}{
			"session_id":   chatManage.SessionID,
			"vlm_model_id": chatManage.VLMModelID,
			"error":        err.Error(),
		})
	}

	return nil
}

// updateUserMessageImageCaption writes the generated ImageDescription back to
// the stored user message so that subsequent turns can see it in history.
func (p *PluginImageAnalysis) updateUserMessageImageCaption(ctx context.Context, chatManage *types.ChatManage) {
	msg, err := p.messageService.GetMessage(ctx, chatManage.SessionID, chatManage.UserMessageID)
	if err != nil {
		pipelineWarn(ctx, "ImageAnalysis", "get_user_message", map[string]interface{}{
			"session_id":      chatManage.SessionID,
			"user_message_id": chatManage.UserMessageID,
			"error":           err.Error(),
		})
		return
	}

	if len(msg.Images) == 0 {
		return
	}

	msg.Images[0].Caption = chatManage.ImageDescription

	if err := p.messageService.UpdateMessageImages(ctx, chatManage.SessionID, chatManage.UserMessageID, msg.Images); err != nil {
		pipelineWarn(ctx, "ImageAnalysis", "update_image_caption", map[string]interface{}{
			"session_id":      chatManage.SessionID,
			"user_message_id": chatManage.UserMessageID,
			"error":           err.Error(),
		})
	}
}
