package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	llmcontext "github.com/Tencent/WeKnora/internal/application/service/llmcontext"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// Errors returned by the SubAgentOrchestrator
var (
	ErrMaxDepthExceeded = errors.New("sub-agent max recursion depth exceeded")
	ErrAgentNotFound    = errors.New("sub-agent not found")
	ErrNotSubAgentable  = errors.New("target agent is not configured as a sub-agent")
)

// AbsoluteMaxSubAgentDepth is the hard limit on sub-agent nesting depth.
const AbsoluteMaxSubAgentDepth = 5

// context key types to avoid string key collisions
type ctxKey int

const (
	ctxKeySubAgentDepth ctxKey = iota
	ctxKeyParentTraceID
	ctxKeyTokenBudget
)

// GetSubAgentDepth extracts the current sub-agent recursion depth from context.
func GetSubAgentDepth(ctx context.Context) int {
	if v, ok := ctx.Value(ctxKeySubAgentDepth).(int); ok {
		return v
	}
	return 0
}

// SubAgentExecution holds the parameters for a single sub-agent invocation.
type SubAgentExecution struct {
	AgentID       string // Target CustomAgent ID
	Task          string // Sub-task description
	Context       string // Context summary from parent agent
	Depth         int    // Current recursion depth
	ParentTraceID string // Parent trace ID for call chain tracking
	MaxIterations int    // Override default max iterations (0 = use default)
}

// SubAgentOrchestrator manages the lifecycle of sub-agent executions.
type SubAgentOrchestrator struct {
	agentService       interfaces.AgentService
	customAgentService interfaces.CustomAgentService
	modelService       interfaces.ModelService
	parentEventBus     *event.EventBus
	parentSessionID    string
	config             *types.AgentConfig
	maxDepth           int
	tokenBudget        *TokenBudget
}

// NewSubAgentOrchestrator creates a new orchestrator.
func NewSubAgentOrchestrator(
	agentService interfaces.AgentService,
	customAgentService interfaces.CustomAgentService,
	modelService interfaces.ModelService,
	parentEventBus *event.EventBus,
	parentSessionID string,
	config *types.AgentConfig,
	tokenBudget *TokenBudget,
) *SubAgentOrchestrator {
	maxDepth := config.SubAgentMaxDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	if maxDepth > AbsoluteMaxSubAgentDepth {
		maxDepth = AbsoluteMaxSubAgentDepth
	}
	return &SubAgentOrchestrator{
		agentService:       agentService,
		customAgentService: customAgentService,
		modelService:       modelService,
		parentEventBus:     parentEventBus,
		parentSessionID:    parentSessionID,
		config:             config,
		maxDepth:           maxDepth,
		tokenBudget:        tokenBudget,
	}
}

// Execute runs a sub-agent and returns its final state.
func (o *SubAgentOrchestrator) Execute(
	ctx context.Context,
	exec SubAgentExecution,
) (*types.AgentState, error) {

	// 1. Depth check
	if exec.Depth >= o.maxDepth {
		return nil, fmt.Errorf("%w: current depth=%d, max=%d",
			ErrMaxDepthExceeded, exec.Depth, o.maxDepth)
	}

	// 2. Token budget pre-check
	if o.tokenBudget != nil && !o.tokenBudget.Check(1024) {
		return nil, ErrBudgetExhausted
	}

	// 3. Load target CustomAgent
	customAgent, err := o.customAgentService.GetAgentByID(ctx, exec.AgentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrAgentNotFound, exec.AgentID, err)
	}

	// Verify the agent allows being called as a sub-agent
	if !customAgent.Config.CanBeSubAgent {
		return nil, fmt.Errorf("%w: %s", ErrNotSubAgentable, exec.AgentID)
	}

	customAgent.EnsureDefaults()

	// 4. Build sub-agent config
	subConfig := o.buildSubAgentConfig(customAgent, exec)

	// 5. Create ScopedEventBus
	traceID := uuid.New().String()[:8]
	scopedBus := event.NewScopedEventBus(
		o.parentEventBus,
		exec.Depth+1,
		exec.AgentID,
		customAgent.Name,
		traceID,
	)

	// Emit start event
	scopedBus.EmitStart(ctx, exec.Task)

	// 6. Create isolated ContextManager (in-memory, not persisted)
	subContextMgr := llmcontext.NewContextManagerWithMemory(
		llmcontext.NewSlidingWindowStrategy(10),
		32*1024, // 32K tokens for sub-agent
	)

	// 7. Inject depth into context
	subCtx := context.WithValue(ctx, ctxKeySubAgentDepth, exec.Depth+1)
	subCtx = context.WithValue(subCtx, ctxKeyParentTraceID, traceID)

	// 8. Resolve models
	chatModel, err := o.modelService.GetChatModel(ctx, customAgent.Config.ModelID)
	if err != nil {
		scopedBus.EmitError(ctx, fmt.Sprintf("failed to load model: %v", err))
		return nil, fmt.Errorf("failed to load chat model for sub-agent %s: %w", exec.AgentID, err)
	}

	rerankModel, _ := o.modelService.GetRerankModel(ctx, customAgent.Config.RerankModelID)

	// 9. Build query with optional context
	query := exec.Task
	if exec.Context != "" {
		query = fmt.Sprintf("Context from parent agent:\n%s\n\nTask:\n%s", exec.Context, exec.Task)
	}

	// 10. Create sub-agent engine
	subSessionID := fmt.Sprintf("%s/sub/%s/%s", o.parentSessionID, exec.AgentID, traceID)

	engine, err := o.agentService.CreateAgentEngine(
		subCtx,
		subConfig,
		chatModel,
		rerankModel,
		scopedBus.Inner(),
		subContextMgr,
		subSessionID,
	)
	if err != nil {
		scopedBus.EmitError(ctx, fmt.Sprintf("failed to create sub-agent engine: %v", err))
		return nil, fmt.Errorf("failed to create sub-agent engine: %w", err)
	}

	// 11. Execute
	startTime := time.Now()
	messageID := uuid.New().String()

	logger.Infof(ctx, "[SubAgent] Starting sub-agent %s (depth=%d, trace=%s) for task: %s",
		exec.AgentID, exec.Depth+1, traceID, exec.Task)

	state, err := engine.Execute(subCtx, subSessionID, messageID, query, nil)
	if err != nil {
		scopedBus.EmitError(ctx, fmt.Sprintf("sub-agent execution error: %v", err))
		return nil, fmt.Errorf("sub-agent %s execution error: %w", exec.AgentID, err)
	}

	duration := time.Since(startTime).Milliseconds()

	// 12. Deduct token budget
	if o.tokenBudget != nil {
		tokensUsed := EstimateTokensFromAgentState(state.FinalAnswer, state.RoundSteps)
		_ = o.tokenBudget.Deduct(tokensUsed)
	}

	// 13. Emit complete event
	scopedBus.EmitComplete(ctx, state.FinalAnswer, len(state.RoundSteps), duration)

	logger.Infof(ctx, "[SubAgent] Sub-agent %s completed (depth=%d, trace=%s, steps=%d, duration=%dms)",
		exec.AgentID, exec.Depth+1, traceID, len(state.RoundSteps), duration)

	return state, nil
}

// buildSubAgentConfig creates an AgentConfig for the sub-agent based on its CustomAgent settings.
func (o *SubAgentOrchestrator) buildSubAgentConfig(
	customAgent *types.CustomAgent,
	exec SubAgentExecution,
) *types.AgentConfig {
	config := &types.AgentConfig{
		MaxIterations:     customAgent.Config.MaxIterations,
		ReflectionEnabled: customAgent.Config.ReflectionEnabled,
		AllowedTools:      customAgent.Config.AllowedTools,
		Temperature:       customAgent.Config.Temperature,
		KnowledgeBases:    customAgent.Config.KnowledgeBases,
		WebSearchEnabled:  customAgent.Config.WebSearchEnabled,
		MultiTurnEnabled:  false, // Sub-agents don't need multi-turn
		Thinking:          customAgent.Config.Thinking,
		MCPSelectionMode:  customAgent.Config.MCPSelectionMode,
		MCPServices:       customAgent.Config.MCPServices,

		// Sub-agent's own sub-agent config:
		// Only enabled if depth allows AND the target agent has it configured
		SubAgentEnabled:     customAgent.Config.SubAgentEnabled && (exec.Depth+1 < o.maxDepth),
		SubAgentMaxDepth:    o.maxDepth,
		SubAgentMaxParallel: customAgent.Config.SubAgentMaxParallel,
		SubAgentTokenBudget: customAgent.Config.SubAgentTokenBudget,
		AllowedSubAgents:    customAgent.Config.AllowedSubAgents,
	}

	// Override max iterations if specified
	if exec.MaxIterations > 0 {
		config.MaxIterations = exec.MaxIterations
	}

	return config
}
