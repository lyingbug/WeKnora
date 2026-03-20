package event

import (
	"context"
)

// ScopedEventBus wraps a parent EventBus and adds sub-agent hierarchy metadata
// to all events emitted by child AgentEngines before forwarding them to the parent.
type ScopedEventBus struct {
	parent    *EventBus
	depth     int
	agentID   string
	agentName string
	traceID   string

	// inner is the EventBus given to the child AgentEngine.
	// Events emitted to inner are intercepted, decorated, and forwarded to parent.
	inner *EventBus
}

// NewScopedEventBus creates a ScopedEventBus that intercepts events from the inner
// bus and forwards them to parent with sub-agent metadata.
func NewScopedEventBus(parent *EventBus, depth int, agentID, agentName, traceID string) *ScopedEventBus {
	s := &ScopedEventBus{
		parent:    parent,
		depth:     depth,
		agentID:   agentID,
		agentName: agentName,
		traceID:   traceID,
		inner:     NewEventBus(),
	}
	s.setupInterceptors()
	return s
}

// Inner returns the EventBus that should be passed to the child AgentEngine.
func (s *ScopedEventBus) Inner() *EventBus {
	return s.inner
}

// setupInterceptors registers handlers on the inner bus that forward events to parent.
func (s *ScopedEventBus) setupInterceptors() {
	// Map from inner bus event types to parent bus sub-agent event types.
	// We use an explicit mapping because some inner event types (e.g. "agent.complete")
	// don't follow a simple "sub_agent." + original pattern.
	mapping := map[EventType]EventType{
		EventAgentThought:    EventSubAgentThought,
		EventAgentToolCall:   EventSubAgentToolCall,
		EventAgentToolResult: EventSubAgentToolResult,
		EventAgentReflection: EventSubAgentReflection,
		EventAgentFinalAnswer: EventSubAgentAnswer,
		EventAgentComplete:   EventSubAgentComplete,
		EventError:           EventSubAgentError,
	}

	for innerType, parentType := range mapping {
		innerType, parentType := innerType, parentType // capture
		s.inner.On(innerType, func(ctx context.Context, evt Event) error {
			return s.forwardEvent(ctx, parentType, evt)
		})
	}
}

// forwardEvent decorates the event with sub-agent metadata and emits it on the parent bus.
func (s *ScopedEventBus) forwardEvent(ctx context.Context, targetType EventType, evt Event) error {
	if evt.Metadata == nil {
		evt.Metadata = make(map[string]interface{})
	}
	evt.Metadata["sub_agent_depth"] = s.depth
	evt.Metadata["sub_agent_id"] = s.agentID
	evt.Metadata["sub_agent_name"] = s.agentName
	evt.Metadata["sub_agent_trace_id"] = s.traceID

	evt.Type = targetType

	return s.parent.Emit(ctx, evt)
}

// EmitStart emits a sub_agent.start event on the parent bus.
func (s *ScopedEventBus) EmitStart(ctx context.Context, task string) {
	_ = s.parent.Emit(ctx, Event{
		Type: EventSubAgentStart,
		Data: &SubAgentStartData{
			AgentID:   s.agentID,
			AgentName: s.agentName,
			Task:      task,
			Depth:     s.depth,
			TraceID:   s.traceID,
		},
	})
}

// EmitComplete emits a sub_agent.complete event on the parent bus.
func (s *ScopedEventBus) EmitComplete(ctx context.Context, finalAnswer string, stepsCount int, durationMs int64) {
	_ = s.parent.Emit(ctx, Event{
		Type: EventSubAgentComplete,
		Data: &SubAgentCompleteData{
			AgentID:     s.agentID,
			AgentName:   s.agentName,
			FinalAnswer: finalAnswer,
			Depth:       s.depth,
			DurationMs:  durationMs,
			StepsCount:  stepsCount,
			TraceID:     s.traceID,
		},
	})
}

// EmitError emits a sub_agent.error event on the parent bus.
func (s *ScopedEventBus) EmitError(ctx context.Context, errMsg string) {
	_ = s.parent.Emit(ctx, Event{
		Type: EventSubAgentError,
		Data: &ErrorData{
			Error: errMsg,
			Stage: "sub_agent",
			Extra: map[string]interface{}{
				"sub_agent_id":   s.agentID,
				"sub_agent_name": s.agentName,
				"depth":          s.depth,
				"trace_id":       s.traceID,
			},
		},
	})
}
