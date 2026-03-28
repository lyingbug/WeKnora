package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"golang.org/x/sync/errgroup"
)

// toolDisplayNames maps internal tool names to user-friendly display labels.
var toolDisplayNames = map[string]string{
	agenttools.ToolThinking:            "深度思考",
	agenttools.ToolTodoWrite:           "制定计划",
	agenttools.ToolGrepChunks:          "关键词搜索",
	agenttools.ToolKnowledgeSearch:     "知识搜索",
	agenttools.ToolListKnowledgeChunks: "查看文档分块",
	agenttools.ToolQueryKnowledgeGraph: "查询知识图谱",
	agenttools.ToolGetDocumentInfo:     "获取文档信息",
	agenttools.ToolDatabaseQuery:       "查询数据",
	agenttools.ToolDataAnalysis:        "数据分析",
	agenttools.ToolDataSchema:          "查看数据结构",
	agenttools.ToolWebSearch:           "搜索网页",
	agenttools.ToolWebFetch:            "获取网页",
	agenttools.ToolFinalAnswer:         "最终回答",
	agenttools.ToolExecuteSkillScript:  "执行技能脚本",
	agenttools.ToolReadSkill:           "读取技能",
}

// toolHintSensitiveArgs lists tools whose arguments should NOT be shown in hints
// (e.g., database_query exposes raw SQL which leaks implementation details).
var toolHintSensitiveArgs = map[string]bool{
	agenttools.ToolDatabaseQuery: true,
}

// formatToolHint returns a concise human-readable hint for a tool call, e.g. `搜索网页("query text")`.
// Uses display names instead of internal tool names, and hides sensitive arguments.
func formatToolHint(name string, args map[string]any) string {
	displayName := name
	if dn, ok := toolDisplayNames[name]; ok {
		displayName = dn
	}

	if len(args) == 0 || toolHintSensitiveArgs[name] {
		return displayName
	}
	for _, v := range args {
		if s, ok := v.(string); ok {
			if len(s) > 40 {
				s = s[:40] + "…"
			}
			return fmt.Sprintf(`%s("%s")`, displayName, s)
		}
	}
	return displayName
}

// executeToolCalls runs every tool call in the LLM response, appending results to step.ToolCalls.
// It also emits tool-call and tool-result events, and optionally runs reflection after each call.
// When config.ParallelToolCalls is true, independent tool calls (excluding final_answer) execute
// concurrently using errgroup; final_answer is always executed last and serially.
func (e *AgentEngine) executeToolCalls(
	ctx context.Context, response *types.ChatResponse,
	step *types.AgentStep, iteration int, sessionID string,
) {
	if len(response.ToolCalls) == 0 {
		return
	}

	round := iteration + 1
	logger.Infof(ctx, "[Agent][Round-%d] Executing %d tool call(s)", round, len(response.ToolCalls))

	// Separate final_answer from other tool calls
	var normalCalls []int
	var finalAnswerCalls []int
	for i, tc := range response.ToolCalls {
		if tc.Function.Name == agenttools.ToolFinalAnswer {
			finalAnswerCalls = append(finalAnswerCalls, i)
		} else {
			normalCalls = append(normalCalls, i)
		}
	}

	useParallel := e.config.ParallelToolCalls && len(normalCalls) > 1

	// Pre-allocate results slice to preserve original order
	results := make([]types.ToolCall, len(response.ToolCalls))

	if useParallel {
		logger.Infof(ctx, "[Agent][Round-%d] Parallel execution enabled for %d tool call(s)", round, len(normalCalls))
		var mu sync.Mutex
		g, gCtx := errgroup.WithContext(ctx)

		for _, idx := range normalCalls {
			idx := idx // capture loop variable
			g.Go(func() error {
				tc := e.executeSingleToolCall(gCtx, response.ToolCalls[idx], idx, len(response.ToolCalls), round, iteration, sessionID, &mu)
				results[idx] = tc
				return nil
			})
		}

		// Wait for all parallel calls to complete (errors are captured per-tool, not propagated)
		_ = g.Wait()
	} else {
		// Sequential execution for normal calls
		for _, idx := range normalCalls {
			tc := e.executeSingleToolCall(ctx, response.ToolCalls[idx], idx, len(response.ToolCalls), round, iteration, sessionID, nil)
			results[idx] = tc
		}
	}

	// Execute final_answer calls serially (must be last)
	for _, idx := range finalAnswerCalls {
		tc := e.executeSingleToolCall(ctx, response.ToolCalls[idx], idx, len(response.ToolCalls), round, iteration, sessionID, nil)
		results[idx] = tc
	}

	// Append results in original order
	for _, tc := range results {
		if tc.ID != "" { // skip zero-value entries (shouldn't happen, but defensive)
			step.ToolCalls = append(step.ToolCalls, tc)
		}
	}
}

// executeSingleToolCall executes a single tool call and emits all associated events.
// If mu is non-nil, it is used to synchronize event emission for thread safety.
func (e *AgentEngine) executeSingleToolCall(
	ctx context.Context,
	rawTC types.LLMToolCall,
	index, total, round, iteration int,
	sessionID string,
	mu *sync.Mutex,
) types.ToolCall {
	tc := rawTC
	// Normalize tool call ID for cross-provider compatibility
	tc.ID = agenttools.NormalizeToolCallID(tc.ID, tc.Function.Name, index)
	toolTag := fmt.Sprintf("[Agent][Round-%d][Tool %s (%d/%d)]",
		round, tc.Function.Name, index+1, total)

	var args map[string]any
	argsStr := tc.Function.Arguments
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		// Attempt JSON repair before giving up
		repaired := agenttools.RepairJSON(argsStr)
		if repairErr := json.Unmarshal([]byte(repaired), &args); repairErr != nil {
			logger.Errorf(ctx, "%s Failed to parse arguments (repair failed): %v", toolTag, err)
			return types.ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: map[string]any{"_raw": argsStr},
				Result: &types.ToolResult{
					Success: false,
					Error: fmt.Sprintf(
						"Failed to parse tool arguments: %v", err,
					) + "\n\n[Analyze the error above and try a different approach.]",
				},
			}
		}
		logger.Warnf(ctx, "%s Repaired malformed JSON arguments", toolTag)
		tc.Function.Arguments = repaired
	}

	logger.Debugf(ctx, "%s Args: %s", toolTag, tc.Function.Arguments)

	toolCallStartTime := time.Now()

	// Emit tool hint for UI progress display
	toolHint := formatToolHint(tc.Function.Name, args)
	e.emitEvent(ctx, mu, event.Event{
		ID:        tc.ID + "-tool-hint",
		Type:      event.EventAgentToolCall,
		SessionID: sessionID,
		Data: event.AgentToolCallData{
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Arguments:  args,
			Iteration:  iteration,
			Hint:       toolHint,
		},
	})

	// Execute tool with timeout to prevent indefinite hangs
	common.PipelineInfo(ctx, "Agent", "tool_call_start", map[string]interface{}{
		"iteration":    iteration,
		"round":        round,
		"tool":         tc.Function.Name,
		"tool_call_id": tc.ID,
		"tool_index":   fmt.Sprintf("%d/%d", index+1, total),
	})
	toolCtx, toolCancel := context.WithTimeout(ctx, defaultToolExecTimeout)
	result, err := e.toolRegistry.ExecuteTool(
		toolCtx, tc.Function.Name,
		json.RawMessage(tc.Function.Arguments),
	)
	toolCancel()
	duration := time.Since(toolCallStartTime).Milliseconds()

	toolCall := types.ToolCall{
		ID:       tc.ID,
		Name:     tc.Function.Name,
		Args:     args,
		Result:   result,
		Duration: duration,
	}

	if err != nil {
		logger.Errorf(ctx, "%s Failed in %dms: %v", toolTag, duration, err)
		toolCall.Result = &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}
	} else {
		success := result != nil && result.Success
		outputLen := 0
		if result != nil {
			outputLen = len(result.Output)
		}
		logger.Infof(ctx, "%s Completed in %dms: success=%v, output=%d chars",
			toolTag, duration, success, outputLen)
	}

	// Pipeline event for monitoring
	toolSuccess := toolCall.Result != nil && toolCall.Result.Success
	pipelineFields := map[string]interface{}{
		"iteration":    iteration,
		"round":        round,
		"tool":         tc.Function.Name,
		"tool_call_id": tc.ID,
		"duration_ms":  duration,
		"success":      toolSuccess,
	}
	if toolCall.Result != nil && toolCall.Result.Error != "" {
		pipelineFields["error"] = toolCall.Result.Error
	}
	if err != nil {
		common.PipelineError(ctx, "Agent", "tool_call_result", pipelineFields)
	} else if toolSuccess {
		common.PipelineInfo(ctx, "Agent", "tool_call_result", pipelineFields)
	} else {
		common.PipelineWarn(ctx, "Agent", "tool_call_result", pipelineFields)
	}

	// Debug-level output preview
	if toolCall.Result != nil && toolCall.Result.Output != "" {
		preview := toolCall.Result.Output
		if len(preview) > 500 {
			preview = preview[:500] + "... (truncated)"
		}
		logger.Debugf(ctx, "%s Output preview:\n%s", toolTag, preview)
	}
	if toolCall.Result != nil && toolCall.Result.Error != "" {
		logger.Debugf(ctx, "%s Tool error: %s", toolTag, toolCall.Result.Error)
	}

	// Emit tool result event (include structured data from tool result)
	e.emitEvent(ctx, mu, event.Event{
		ID:        tc.ID + "-tool-result",
		Type:      event.EventAgentToolResult,
		SessionID: sessionID,
		Data: event.AgentToolResultData{
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Output:     result.Output,
			Error:      result.Error,
			Success:    result.Success,
			Duration:   duration,
			Iteration:  iteration,
			Data:       result.Data, // Pass structured data for frontend rendering
		},
	})

	// Emit tool execution event (for internal monitoring)
	e.emitEvent(ctx, mu, event.Event{
		ID:        tc.ID + "-tool-exec",
		Type:      event.EventAgentTool,
		SessionID: sessionID,
		Data: event.AgentActionData{
			Iteration:  iteration,
			ToolName:   tc.Function.Name,
			ToolInput:  args,
			ToolOutput: result.Output,
			Success:    result.Success,
			Error:      result.Error,
			Duration:   duration,
		},
	})

	return toolCall
}

// emitEvent emits an event, using the provided mutex for thread safety when non-nil.
func (e *AgentEngine) emitEvent(ctx context.Context, mu *sync.Mutex, evt event.Event) {
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	e.eventBus.Emit(ctx, evt)
}
