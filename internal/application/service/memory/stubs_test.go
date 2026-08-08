package memory

import (
	"context"
	"errors"
	"strings"
	"sync"

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

// stubMessageRepo returns a fixed session transcript.
type stubMessageRepo struct {
	interfaces.MessageRepository

	messages []*types.Message
}

func (s *stubMessageRepo) GetRecentMessagesBySession(
	_ context.Context, _ string, _ int,
) ([]*types.Message, error) {
	return s.messages, nil
}

// stubModelService hands out a chat model that replays a canned response and
// records what it was asked.
type stubModelService struct {
	interfaces.ModelService

	response         string
	requestedModelID string
	lastPrompt       string
	calls            int
}

func (s *stubModelService) GetChatModel(_ context.Context, modelID string) (chat.Chat, error) {
	s.requestedModelID = modelID
	return &stubChatModel{owner: s}, nil
}

type stubChatModel struct {
	owner *stubModelService
}

func (m *stubChatModel) Chat(
	_ context.Context, messages []chat.Message, _ *chat.ChatOptions,
) (*types.ChatResponse, error) {
	m.owner.calls++
	var prompt strings.Builder
	for _, message := range messages {
		prompt.WriteString(message.Content)
		prompt.WriteString("\n")
	}
	m.owner.lastPrompt = prompt.String()
	return &types.ChatResponse{Content: m.owner.response}, nil
}

func (m *stubChatModel) ChatStream(
	_ context.Context, _ []chat.Message, _ *chat.ChatOptions,
) (<-chan types.StreamResponse, error) {
	return nil, errors.New("not used")
}

func (m *stubChatModel) GetModelName() string { return "stub" }
func (m *stubChatModel) GetModelID() string   { return "stub" }

// stubEnqueuer records enqueued tasks instead of touching Redis.
type stubEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
}

func (s *stubEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, task)
	return &asynq.TaskInfo{ID: "stub", Type: task.Type()}, nil
}
