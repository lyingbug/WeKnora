package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// TaskHandler dispatches the three memory background tasks.
//
// One handler for all three task types rather than three registrations: they
// share the same dependency and the same error semantics, and a single entry
// point makes it impossible to register the asynq side of one and forget the
// Lite side of another — a class of bug whose only symptom is "the feature
// silently does nothing on Lite".
type TaskHandler struct {
	writer interfaces.MemoryWriterService
}

// NewTaskHandler creates the memory task handler.
func NewTaskHandler(writer interfaces.MemoryWriterService) *TaskHandler {
	return &TaskHandler{writer: writer}
}

// Handle routes one task to its stage.
func (h *TaskHandler) Handle(ctx context.Context, task *asynq.Task) error {
	switch task.Type() {
	case types.TypeMemoryExtract:
		return h.handleExtract(ctx, task)
	case types.TypeMemoryConsolidate:
		return h.handleConsolidate(ctx, task)
	case types.TypeMemoryDecay:
		return h.handleDecay(ctx, task)
	default:
		return fmt.Errorf("memory: unexpected task type %q", task.Type())
	}
}

func (h *TaskHandler) handleExtract(ctx context.Context, task *asynq.Task) error {
	var payload types.MemoryExtractPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		// A payload this process cannot read will never become readable, so
		// returning SkipRetry stops asynq from burning its retry budget on it.
		return fmt.Errorf("memory: bad extract payload: %v: %w", err, asynq.SkipRetry)
	}
	if payload.SpaceID == "" || payload.SessionID == "" {
		return nil
	}
	ctx = logger.WithRequestID(ctx, uuid.New().String())
	ctx = logger.WithField(ctx, "memory_space", payload.SpaceID)
	logger.Infof(ctx, "memory: extracting for space %s session %s", payload.SpaceID, payload.SessionID)
	return h.writer.Extract(ctx, payload)
}

func (h *TaskHandler) handleConsolidate(ctx context.Context, task *asynq.Task) error {
	var payload types.MemoryConsolidatePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("memory: bad consolidate payload: %v: %w", err, asynq.SkipRetry)
	}
	if payload.SpaceID == "" {
		return nil
	}
	ctx = logger.WithRequestID(ctx, uuid.New().String())
	ctx = logger.WithField(ctx, "memory_space", payload.SpaceID)
	return h.writer.Consolidate(ctx, payload)
}

func (h *TaskHandler) handleDecay(ctx context.Context, task *asynq.Task) error {
	var payload types.MemoryDecayPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("memory: bad decay payload: %v: %w", err, asynq.SkipRetry)
	}
	if payload.SpaceID == "" {
		return h.writer.DecayAll(ctx)
	}
	return h.writer.Decay(ctx, payload.TenantID, payload.SpaceID)
}
