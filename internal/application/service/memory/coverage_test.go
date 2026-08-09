package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

// This file is about one property: while memory is switched on, every user
// message is eventually read by distillation.
//
// It exists because the first version of the scheduler compared the current
// time against the last run and returned early inside the interval, which
// silently discarded every turn in that window — the feature looked enabled and
// quietly learned nothing. Timers may delay a message; they may not lose it.

func userMessage(sessionID, content string, at time.Time) *types.Message {
	return &types.Message{
		ID:        content,
		SessionID: sessionID,
		Role:      "user",
		Content:   content,
		CreatedAt: at,
	}
}

// drainExtractions runs every task the service queued, plus any follow-ups
// those runs queue, until the queue is empty. Bounded so a scheduling bug
// shows up as a failure rather than a hang.
func drainExtractions(t *testing.T, svc *Service, enqueuer *stubEnqueuer) int {
	t.Helper()
	runs := 0
	for i := 0; i < 50; i++ {
		task := enqueuer.pop()
		if task == nil {
			return runs
		}
		require.NoError(t, svc.Handle(context.Background(), task))
		runs++
	}
	t.Fatal("extraction did not settle: follow-up tasks kept queueing")
	return runs
}

// TestEveryTurnIsEventuallyRead is the headline guarantee. Turns arrive faster
// than the debounce window, so most of them are recorded while a run is already
// in flight; all of them must still reach the model.
func TestEveryTurnIsEventuallyRead(t *testing.T) {
	svc, tenantRepo, messages, models, enqueuer := newExtractionHarness(t)
	ctx := enabledCtx(t, tenantRepo, 1, "alice")
	tenantRepo.set(1, &types.MemoryConfig{
		Enabled: true, WriteMode: types.MemoryWriteAuto,
		ExtractDelaySeconds: 5, ExtractMinIntervalSeconds: 1,
	})
	models.response = `{"memories":[]}`

	base := time.Now().Add(-time.Hour)
	var transcript []*types.Message
	for i := 0; i < 12; i++ {
		content := fmt.Sprintf("第 %d 句话", i)
		transcript = append(transcript, userMessage("session-1", content, base.Add(time.Duration(i)*time.Second)))
		messages.set("session-1", transcript)
		svc.ScheduleExtraction(ctx, "session-1", fmt.Sprintf("message-%d", i), "model-1")
	}

	drainExtractions(t, svc, enqueuer)

	seen := models.seenTranscripts()
	for i := 0; i < 12; i++ {
		require.Contains(t, seen, fmt.Sprintf("第 %d 句话", i),
			"turn %d was never read by distillation", i)
	}
}

// TestTurnsDuringARunAreNotLost covers the narrow window that the queue exists
// for: a message that arrives after a run has already taken its work.
func TestTurnsDuringARunAreNotLost(t *testing.T) {
	svc, tenantRepo, messages, models, enqueuer := newExtractionHarness(t)
	ctx := enabledCtx(t, tenantRepo, 1, "alice")
	tenantRepo.set(1, &types.MemoryConfig{
		Enabled: true, WriteMode: types.MemoryWriteAuto, ExtractDelaySeconds: 5,
	})
	models.response = `{"memories":[]}`

	base := time.Now().Add(-time.Hour)
	messages.set("session-1", []*types.Message{userMessage("session-1", "第一句", base)})
	svc.ScheduleExtraction(ctx, "session-1", "message-1", "model-1")

	first := enqueuer.pop()
	require.NotNil(t, first)

	// The second turn lands while the first run is still queued.
	messages.set("session-1", []*types.Message{
		userMessage("session-1", "第一句", base),
		userMessage("session-1", "第二句", base.Add(time.Second)),
	})
	svc.ScheduleExtraction(ctx, "session-1", "message-2", "model-1")

	require.NoError(t, svc.Handle(context.Background(), first))
	drainExtractions(t, svc, enqueuer)

	seen := models.seenTranscripts()
	require.Contains(t, seen, "第一句")
	require.Contains(t, seen, "第二句")
}

// TestMessagesBeyondOneRunsCapAreFollowedUp covers a subject who said more in
// one window than a single run is allowed to read.
func TestMessagesBeyondOneRunsCapAreFollowedUp(t *testing.T) {
	svc, tenantRepo, messages, models, enqueuer := newExtractionHarness(t)
	ctx := enabledCtx(t, tenantRepo, 1, "alice")
	tenantRepo.set(1, &types.MemoryConfig{
		Enabled: true, WriteMode: types.MemoryWriteAuto, ExtractDelaySeconds: 5,
	})
	models.response = `{"memories":[]}`

	base := time.Now().Add(-time.Hour)
	total := extractMaxMessagesPerRun*2 + 5
	var transcript []*types.Message
	for i := 0; i < total; i++ {
		transcript = append(transcript,
			userMessage("session-1", fmt.Sprintf("消息%d号", i), base.Add(time.Duration(i)*time.Second)))
	}
	messages.set("session-1", transcript)
	svc.ScheduleExtraction(ctx, "session-1", "message-last", "model-1")

	runs := drainExtractions(t, svc, enqueuer)
	require.Greater(t, runs, 1, "a backlog larger than one run must produce follow-up runs")

	seen := models.seenTranscripts()
	require.Contains(t, seen, "消息0号", "the oldest unread message must not be skipped")
	require.Contains(t, seen, fmt.Sprintf("消息%d号", total-1))
}

// TestParallelSessionsAreAllRead: a person talking in two conversations must
// not have one of them ignored because the other triggered the run.
func TestParallelSessionsAreAllRead(t *testing.T) {
	svc, tenantRepo, messages, models, enqueuer := newExtractionHarness(t)
	ctx := enabledCtx(t, tenantRepo, 1, "alice")
	tenantRepo.set(1, &types.MemoryConfig{
		Enabled: true, WriteMode: types.MemoryWriteAuto, ExtractDelaySeconds: 5,
	})
	models.response = `{"memories":[]}`

	base := time.Now().Add(-time.Hour)
	messages.set("session-a", []*types.Message{userMessage("session-a", "会话A说的话", base)})
	messages.set("session-b", []*types.Message{userMessage("session-b", "会话B说的话", base.Add(time.Second))})

	svc.ScheduleExtraction(ctx, "session-a", "message-a", "model-1")
	svc.ScheduleExtraction(ctx, "session-b", "message-b", "model-1")
	drainExtractions(t, svc, enqueuer)

	seen := models.seenTranscripts()
	require.Contains(t, seen, "会话A说的话")
	require.Contains(t, seen, "会话B说的话")
}

// TestAlreadyReadMessagesAreNotReread keeps the guarantee from degenerating
// into "read everything every time", which would make cost grow with history.
func TestAlreadyReadMessagesAreNotReread(t *testing.T) {
	svc, tenantRepo, messages, models, enqueuer := newExtractionHarness(t)
	ctx := enabledCtx(t, tenantRepo, 1, "alice")
	tenantRepo.set(1, &types.MemoryConfig{
		Enabled: true, WriteMode: types.MemoryWriteAuto, ExtractDelaySeconds: 5,
	})
	models.response = `{"memories":[]}`

	base := time.Now().Add(-time.Hour)
	messages.set("session-1", []*types.Message{userMessage("session-1", "旧的一句", base)})
	svc.ScheduleExtraction(ctx, "session-1", "message-1", "model-1")
	drainExtractions(t, svc, enqueuer)
	require.Equal(t, 1, models.calls)

	messages.set("session-1", []*types.Message{
		userMessage("session-1", "旧的一句", base),
		userMessage("session-1", "新的一句", base.Add(time.Minute)),
	})
	svc.ScheduleExtraction(ctx, "session-1", "message-2", "model-1")
	drainExtractions(t, svc, enqueuer)

	require.Equal(t, 2, models.calls)
	// The earlier message may appear as read-only context, but it must not be
	// inside the block the model extracts from, or it would be re-derived into
	// a memory on every run.
	transcript := transcriptBlock(models.lastPrompt)
	require.Contains(t, transcript, "新的一句")
	require.NotContains(t, transcript, "旧的一句",
		"a message already behind the watermark must not be extracted from twice")
	require.Contains(t, models.lastPrompt, "context only",
		"the earlier turn should still be visible as context")
}

// TestFailedRunLeavesMessagesUnread: a model error must not consume the
// messages it failed on, or a transient outage would silently erase them.
func TestFailedRunLeavesMessagesUnread(t *testing.T) {
	svc, tenantRepo, messages, models, enqueuer := newExtractionHarness(t)
	ctx := enabledCtx(t, tenantRepo, 1, "alice")
	tenantRepo.set(1, &types.MemoryConfig{
		Enabled: true, WriteMode: types.MemoryWriteAuto, ExtractDelaySeconds: 5,
	})

	base := time.Now().Add(-time.Hour)
	messages.set("session-1", []*types.Message{userMessage("session-1", "重要的一句", base)})
	models.failNext = true
	svc.ScheduleExtraction(ctx, "session-1", "message-1", "model-1")

	task := enqueuer.pop()
	require.NotNil(t, task)
	require.Error(t, svc.Handle(context.Background(), task))

	// The next turn schedules a fresh run, which must see the message again.
	models.response = `{"memories":[]}`
	svc.ScheduleExtraction(ctx, "session-1", "message-2", "model-1")
	drainExtractions(t, svc, enqueuer)
	require.Contains(t, models.seenTranscripts(), "重要的一句")
}

// TestScheduleUsesTheConfiguredDelay pins that the timers are configuration,
// not constants.
func TestScheduleUsesTheConfiguredDelay(t *testing.T) {
	svc, tenantRepo, _, _, enqueuer := newExtractionHarness(t)
	ctx := enabledCtx(t, tenantRepo, 1, "alice")
	tenantRepo.set(1, &types.MemoryConfig{
		Enabled: true, WriteMode: types.MemoryWriteAuto, ExtractDelaySeconds: 7,
	})

	svc.ScheduleExtraction(ctx, "session-1", "message-1", "model-1")
	require.Len(t, enqueuer.options, 1)
	require.Equal(t, 7*time.Second, enqueuer.options[0].processIn)
}

// TestMinIntervalDefersInsteadOfDropping is the exact behaviour change: a turn
// arriving soon after a run is queued further out, not discarded.
func TestMinIntervalDefersInsteadOfDropping(t *testing.T) {
	svc, tenantRepo, messages, models, enqueuer := newExtractionHarness(t)
	ctx := enabledCtx(t, tenantRepo, 1, "alice")
	tenantRepo.set(1, &types.MemoryConfig{
		Enabled: true, WriteMode: types.MemoryWriteAuto,
		ExtractDelaySeconds: 5, ExtractMinIntervalSeconds: 600,
	})
	models.response = `{"memories":[]}`

	base := time.Now().Add(-time.Hour)
	messages.set("session-1", []*types.Message{userMessage("session-1", "第一句", base)})
	svc.ScheduleExtraction(ctx, "session-1", "message-1", "model-1")
	first := enqueuer.pop()
	require.NotNil(t, first)
	require.NoError(t, svc.Handle(context.Background(), first))

	// Immediately after a run: well inside the ten-minute floor.
	messages.set("session-1", []*types.Message{
		userMessage("session-1", "第一句", base),
		userMessage("session-1", "第二句", base.Add(time.Second)),
	})
	svc.ScheduleExtraction(ctx, "session-1", "message-2", "model-1")

	require.Len(t, enqueuer.tasks, 1, "the turn must still be scheduled, not dropped")
	last := enqueuer.options[len(enqueuer.options)-1]
	require.Greater(t, last.processIn, 5*time.Second,
		"the minimum interval must push the run out rather than discard the turn")

	require.NoError(t, svc.Handle(context.Background(), enqueuer.pop()))
	require.Contains(t, models.seenTranscripts(), "第二句")
}

// TestNothingIsScheduledWhileMemoryIsOff is the other half of the promise: the
// guarantee applies while the switch is on, and costs nothing while it is off.
func TestNothingIsScheduledWhileMemoryIsOff(t *testing.T) {
	svc, tenantRepo, messages, models, enqueuer := newExtractionHarness(t)
	tenantRepo.set(1, &types.MemoryConfig{Enabled: false})
	ctx := context.WithValue(t.Context(), types.TenantIDContextKey, uint64(1))
	ctx = types.WithPrincipal(ctx, types.Principal{Type: types.PrincipalWebUser, ID: "alice"})

	messages.set("session-1", []*types.Message{userMessage("session-1", "一句话", time.Now())})
	svc.ScheduleExtraction(ctx, "session-1", "message-1", "model-1")

	require.Empty(t, enqueuer.tasks)
	require.Zero(t, models.calls)
}

// transcriptBlock returns just the part of the prompt the model is asked to
// extract from, so a test can distinguish "shown as context" from "extracted".
func transcriptBlock(prompt string) string {
	start := strings.Index(prompt, "<transcript>")
	end := strings.Index(prompt, "</transcript>")
	if start < 0 || end <= start {
		return ""
	}
	return prompt[start:end]
}

var (
	_ = json.Marshal
	_ asynq.Task
)
