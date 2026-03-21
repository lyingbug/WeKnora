package chatpipline

import (
	"context"
	"sync"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PluginRewriteParallel orchestrates parallel execution of text rewrite and
// image analysis. When the user message contains images, the text-only rewrite
// (using the fast chat model) and VLM-based image analysis run concurrently,
// reducing overall latency compared to the sequential approach in PluginRewrite.
//
// When no images are present, it delegates directly to PluginRewrite with no
// behavioral change.
type PluginRewriteParallel struct {
	rewritePlugin      *PluginRewrite
	imageAnalysisPlugin *PluginImageAnalysis
}

// NewPluginRewriteParallel creates and registers a new parallel rewrite orchestrator.
func NewPluginRewriteParallel(
	eventManager *EventManager,
	modelService interfaces.ModelService,
	messageService interfaces.MessageService,
	config *config.Config,
) *PluginRewriteParallel {
	// Create internal plugins without registering them with the event manager.
	// They are invoked directly by this orchestrator.
	rewritePlugin := &PluginRewrite{
		modelService:   modelService,
		messageService: messageService,
		config:         config,
	}
	imageAnalysisPlugin := &PluginImageAnalysis{
		modelService:   modelService,
		messageService: messageService,
		config:         config,
	}

	res := &PluginRewriteParallel{
		rewritePlugin:       rewritePlugin,
		imageAnalysisPlugin: imageAnalysisPlugin,
	}
	eventManager.Register(res)
	return res
}

// ActivationEvents returns the event types this plugin handles.
func (p *PluginRewriteParallel) ActivationEvents() []types.EventType {
	return []types.EventType{types.REWRITE_PARALLEL}
}

// OnEvent handles the REWRITE_PARALLEL event.
// When images are present, it runs text rewrite and image analysis in parallel.
// When no images are present, it falls through to the standard rewrite path.
func (p *PluginRewriteParallel) OnEvent(ctx context.Context,
	eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	hasImages := len(chatManage.Images) > 0

	// No images: delegate directly to standard rewrite (no parallelism needed)
	if !hasImages {
		pipelineInfo(ctx, "RewriteParallel", "delegate_text_only", map[string]interface{}{
			"session_id": chatManage.SessionID,
		})
		return p.rewritePlugin.OnEvent(ctx, types.REWRITE_QUERY, chatManage, next)
	}

	pipelineInfo(ctx, "RewriteParallel", "start_parallel", map[string]interface{}{
		"session_id":  chatManage.SessionID,
		"image_count": len(chatManage.Images),
	})

	// Create a copy for text-only rewrite (strip images so it uses the chat model)
	rewriteChatManage := *chatManage
	rewriteChatManage.Images = nil // Force text-only path in selectModel

	var wg sync.WaitGroup
	var rewriteErr, imageErr *PluginError

	wg.Add(2)

	// Goroutine 1: Text rewrite + intent classification (chat model, fast)
	go func() {
		defer wg.Done()
		rewriteErr = p.rewritePlugin.OnEvent(ctx, types.REWRITE_QUERY, &rewriteChatManage, func() *PluginError {
			return nil
		})
		pipelineInfo(ctx, "RewriteParallel", "text_rewrite_done", map[string]interface{}{
			"session_id":     chatManage.SessionID,
			"rewrite_query":  rewriteChatManage.RewriteQuery,
			"skip_kb_search": rewriteChatManage.SkipKBSearch,
			"has_error":      rewriteErr != nil,
		})
	}()

	// Goroutine 2: Image analysis (VLM, slower)
	go func() {
		defer wg.Done()
		imageErr = p.imageAnalysisPlugin.OnEvent(ctx, types.IMAGE_ANALYSIS, chatManage, func() *PluginError {
			return nil
		})
		pipelineInfo(ctx, "RewriteParallel", "image_analysis_done", map[string]interface{}{
			"session_id":    chatManage.SessionID,
			"desc_len":      len(chatManage.ImageDescription),
			"has_error":     imageErr != nil,
		})
	}()

	wg.Wait()

	// Merge results from text rewrite back into the main chatManage
	chatManage.RewriteQuery = rewriteChatManage.RewriteQuery
	chatManage.SkipKBSearch = rewriteChatManage.SkipKBSearch
	chatManage.History = rewriteChatManage.History
	// ImageDescription is already set on chatManage by the image analysis goroutine

	if rewriteErr != nil {
		pipelineWarn(ctx, "RewriteParallel", "rewrite_error", map[string]interface{}{
			"session_id": chatManage.SessionID,
			"error":      rewriteErr.Err.Error(),
		})
	}
	if imageErr != nil {
		pipelineWarn(ctx, "RewriteParallel", "image_error", map[string]interface{}{
			"session_id": chatManage.SessionID,
			"error":      imageErr.Err.Error(),
		})
	}

	pipelineInfo(ctx, "RewriteParallel", "complete", map[string]interface{}{
		"session_id":        chatManage.SessionID,
		"rewrite_query":     chatManage.RewriteQuery,
		"skip_kb_search":    chatManage.SkipKBSearch,
		"image_desc_len":    len(chatManage.ImageDescription),
		"rewrite_error":     rewriteErr != nil,
		"image_error":       imageErr != nil,
	})

	return next()
}
