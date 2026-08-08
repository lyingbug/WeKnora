package memory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
)

// stubTenantRepo serves workspace memory configuration. It embeds the
// interface so the tests only have to implement what the memory service
// actually calls; anything else panics loudly rather than silently returning
// a zero value.
type stubTenantRepo struct {
	interfaces.TenantRepository

	mu      sync.RWMutex
	configs map[uint64]*types.MemoryConfig
}

func (s *stubTenantRepo) set(tenantID uint64, cfg *types.MemoryConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[tenantID] = cfg
}

func (s *stubTenantRepo) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &types.Tenant{ID: id, MemoryConfig: s.configs[id]}, nil
}

// stubMessageRepo serves per-session transcripts. It implements the same
// watermark semantics as the real repository so tests exercise the paging that
// makes coverage guaranteed rather than asserting against a simplification.
type stubMessageRepo struct {
	interfaces.MessageRepository

	mu sync.Mutex
	// messages is the single-session shortcut used by most tests.
	messages []*types.Message
	// bySession is used by tests that span several conversations.
	bySession map[string][]*types.Message
}

func (s *stubMessageRepo) set(sessionID string, messages []*types.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bySession == nil {
		s.bySession = map[string][]*types.Message{}
	}
	s.bySession[sessionID] = messages
}

func (s *stubMessageRepo) ListMessagesBySessionAfterTime(
	_ context.Context, sessionID string, afterTime time.Time, limit int,
) ([]*types.Message, error) {
	s.mu.Lock()
	source := s.bySession[sessionID]
	if source == nil {
		source = s.messages
	}
	snapshot := append([]*types.Message(nil), source...)
	s.mu.Unlock()

	sort.SliceStable(snapshot, func(i, j int) bool {
		return snapshot[i].CreatedAt.Before(snapshot[j].CreatedAt)
	})
	var out []*types.Message
	for _, message := range snapshot {
		if !afterTime.IsZero() && !message.CreatedAt.After(afterTime) {
			continue
		}
		out = append(out, message)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// stubModelService hands out a chat model that replays a canned response and
// records what it was asked.
type stubModelService struct {
	interfaces.ModelService

	mu               sync.Mutex
	response         string
	requestedModelID string
	lastPrompt       string
	// prompts records every transcript the model was asked about, so a test
	// can assert that no message went unread across several runs.
	prompts []string
	calls   int
	// failNext makes the next call fail, standing in for a provider outage.
	failNext bool
}

func (s *stubModelService) GetChatModel(_ context.Context, modelID string) (chat.Chat, error) {
	s.mu.Lock()
	s.requestedModelID = modelID
	s.mu.Unlock()
	return &stubChatModel{owner: s}, nil
}

// seenTranscripts concatenates every prompt the model received.
func (s *stubModelService) seenTranscripts() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.prompts, "\n---\n")
}

type stubChatModel struct {
	owner *stubModelService
}

func (m *stubChatModel) Chat(
	_ context.Context, messages []chat.Message, _ *chat.ChatOptions,
) (*types.ChatResponse, error) {
	var prompt strings.Builder
	for _, message := range messages {
		prompt.WriteString(message.Content)
		prompt.WriteString("\n")
	}
	m.owner.mu.Lock()
	defer m.owner.mu.Unlock()
	m.owner.calls++
	m.owner.lastPrompt = prompt.String()
	m.owner.prompts = append(m.owner.prompts, prompt.String())
	if m.owner.failNext {
		m.owner.failNext = false
		return nil, errors.New("stub model outage")
	}
	return &types.ChatResponse{Content: m.owner.response}, nil
}

func (m *stubChatModel) ChatStream(
	_ context.Context, _ []chat.Message, _ *chat.ChatOptions,
) (<-chan types.StreamResponse, error) {
	return nil, errors.New("not used")
}

func (m *stubChatModel) GetModelName() string { return "stub" }
func (m *stubChatModel) GetModelID() string   { return "stub" }

// stubEnqueueOptions captures the scheduling decisions a test cares about.
type stubEnqueueOptions struct {
	queue     string
	processIn time.Duration
}

// stubEnqueuer records enqueued tasks instead of touching Redis, and lets a
// test drain them in order the way a worker would.
type stubEnqueuer struct {
	mu      sync.Mutex
	tasks   []*asynq.Task
	options []stubEnqueueOptions
}

func (s *stubEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	recorded := stubEnqueueOptions{}
	for _, opt := range opts {
		switch opt.Type() {
		case asynq.QueueOpt:
			if queue, ok := opt.Value().(string); ok {
				recorded.queue = queue
			}
		case asynq.ProcessInOpt:
			if delay, ok := opt.Value().(time.Duration); ok {
				recorded.processIn = delay
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, task)
	s.options = append(s.options, recorded)
	return &asynq.TaskInfo{ID: "stub", Type: task.Type()}, nil
}

// pop returns the oldest queued task, or nil when the queue is empty.
func (s *stubEnqueuer) pop() *asynq.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tasks) == 0 {
		return nil
	}
	task := s.tasks[0]
	s.tasks = s.tasks[1:]
	return task
}
