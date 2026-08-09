package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/hibiken/asynq"
)

// MemoryScope is the resolved (workspace, principal) pair a memory operation
// runs under. It is always derived from the request context so no caller can
// address another user's memory by passing an id.
type MemoryScope struct {
	TenantID  uint64
	SubjectID string
}

func (s MemoryScope) Valid() bool {
	return s.TenantID > 0 && s.SubjectID != ""
}

// MemoryRepository is the storage contract for long-term memory. Every method
// takes an explicit scope rather than reading it from ctx, so a background
// worker cannot accidentally operate on whatever the ambient context happens
// to hold.
type MemoryRepository interface {
	// GetSubject returns the memory space, or (nil, nil) when it does not exist.
	GetSubject(ctx context.Context, scope MemoryScope) (*types.MemorySubject, error)
	// EnsureSubject returns the memory space, creating it on first use.
	EnsureSubject(ctx context.Context, scope MemoryScope) (*types.MemorySubject, error)
	// UpdateSubjectEnabled flips the per-user opt out.
	UpdateSubjectEnabled(ctx context.Context, scope MemoryScope, enabled bool) error
	// UpdateSubjectBlock stores the rendered resident block and item count.
	UpdateSubjectBlock(ctx context.Context, scope MemoryScope, block string, itemCount int) error
	// EnqueuePendingSession records that a session has turns past the cursor
	// and claims the in-flight slot when no run is already scheduled. It
	// returns the subject as it was before the update plus whether this call
	// is the one responsible for enqueuing the task, so a burst of turns
	// produces exactly one task and never a dropped turn.
	EnqueuePendingSession(
		ctx context.Context, scope MemoryScope, sessionID string, inFlightTimeout time.Duration,
	) (*types.MemorySubject, bool, error)
	// ClaimPendingSessions takes the pending queue for processing, returning
	// the sessions to drain and the cursor to walk forward from.
	ClaimPendingSessions(ctx context.Context, scope MemoryScope) ([]string, time.Time, error)
	// FinishExtraction advances the watermark and releases the in-flight slot.
	// A zero cursor leaves the watermark untouched.
	FinishExtraction(ctx context.Context, scope MemoryScope, cursor time.Time) error
	// ReleaseExtractionSlot clears the in-flight marker without advancing the
	// watermark, used when a run ends early.
	ReleaseExtractionSlot(ctx context.Context, scope MemoryScope) error

	// CreateItem inserts one memory item.
	CreateItem(ctx context.Context, item *types.MemoryItem) error
	// GetItem returns one item inside the scope, or (nil, nil) when absent.
	GetItem(ctx context.Context, scope MemoryScope, id string) (*types.MemoryItem, error)
	// ListActiveByKinds returns active items of the given kinds, newest first.
	ListActiveByKinds(ctx context.Context, scope MemoryScope, kinds []string, limit int) ([]*types.MemoryItem, error)
	// ListActiveResident returns the items that belong in the always-injected
	// block: stable traits, plus anything the user explicitly asked to keep.
	ListActiveResident(ctx context.Context, scope MemoryScope, limit int) ([]*types.MemoryItem, error)
	// ListItems returns items for the memory manager, filtered by status.
	ListItems(ctx context.Context, scope MemoryScope, status string, limit, offset int) ([]*types.MemoryItem, int64, error)
	// FindActiveByKey returns the active item occupying a topic key, if any.
	FindActiveByKey(ctx context.Context, scope MemoryScope, normalizedKey string) (*types.MemoryItem, error)
	// UpdateItemContent rewrites an item edited in the memory manager.
	UpdateItemContent(ctx context.Context, scope MemoryScope, id, content, normalizedKey string, importance int) error
	// SupersedeItem marks an outdated item as replaced by another one.
	SupersedeItem(ctx context.Context, scope MemoryScope, id, supersededBy string) error
	// DeleteItem physically removes one item. Forgetting means forgetting.
	DeleteItem(ctx context.Context, scope MemoryScope, id string) error
	// DeleteAll removes every item in the scope and returns how many were removed.
	DeleteAll(ctx context.Context, scope MemoryScope) (int64, error)
	// TouchUsed records that items were injected into a turn.
	TouchUsed(ctx context.Context, scope MemoryScope, ids []string) error
	// AddTombstone records that a statement was deliberately forgotten. Only
	// the topic, a fingerprint and the originating message are stored, never
	// the statement itself.
	AddTombstone(ctx context.Context, scope MemoryScope, topic, fingerprint, sourceMessageID string) error
	// HasTombstoneForMessage reports whether a memory derived from this message
	// was rejected within the given window. The window matters: this rule
	// exists to stop the re-derivation that happens one debounce later, not to
	// ban a message for good.
	HasTombstoneForMessage(
		ctx context.Context, scope MemoryScope, sourceMessageID string, within time.Duration,
	) (bool, error)
	// ListTombstones returns the most recent rejections for this subject.
	ListTombstones(ctx context.Context, scope MemoryScope, limit int) ([]*types.MemoryTombstone, error)
	// HasTombstone reports whether this exact statement was already forgotten.
	HasTombstone(ctx context.Context, scope MemoryScope, fingerprint string) (bool, error)
	// ExpireOverdue archives items whose expires_at has passed and returns how
	// many were archived.
	ExpireOverdue(ctx context.Context, scope MemoryScope) (int64, error)
	// ArchiveLowestRanked archives active items beyond the capacity cap and
	// returns how many were archived.
	ArchiveLowestRanked(ctx context.Context, scope MemoryScope, keep int) (int64, error)
	// CountActive returns the number of active items in the scope.
	CountActive(ctx context.Context, scope MemoryScope) (int64, error)
}

// MemoryRecall is what one turn pulls in: the resident block plus any
// situational items matched against the query.
type MemoryRecall struct {
	// Prompt is the ready-to-append envelope, empty when nothing was recalled.
	Prompt string
	// Items are the situational items plus the resident items that produced
	// Prompt, in the order they appear. Surfaced to the chat UI.
	Items []*types.MemoryItem
}

// MemoryService is the read/write API for long-term memory.
type MemoryService interface {
	// Recall assembles the memory to inject for one turn. It performs no LLM
	// calls and returns an empty recall (never an error) whenever memory is
	// disabled at any level, so callers can use it unconditionally.
	Recall(ctx context.Context, query string) MemoryRecall
	// Remember stores a statement the user explicitly asked to keep.
	Remember(ctx context.Context, item types.MemoryItem) (*types.MemoryItem, error)
	// ScheduleExtraction debounces and enqueues background distillation for a
	// finished turn. Best effort: failures are logged, never returned.
	ScheduleExtraction(ctx context.Context, sessionID, messageID, chatModelID string)
	// Handle runs the background distillation task.
	Handle(ctx context.Context, task *asynq.Task) error

	// ListItems backs the memory manager list.
	ListItems(ctx context.Context, status string, limit, offset int) ([]*types.MemoryItem, int64, error)
	// CreateItem adds a memory typed by the user in the memory manager.
	CreateItem(ctx context.Context, kind, content string, importance int) (*types.MemoryItem, error)
	// UpdateItem edits one item's content and importance.
	UpdateItem(ctx context.Context, id, content string, importance int) (*types.MemoryItem, error)
	// DeleteItem forgets one item.
	DeleteItem(ctx context.Context, id string) error
	// Clear forgets everything in the caller's memory space.
	Clear(ctx context.Context) (int64, error)
	// GetSettings returns the effective per-user memory settings.
	GetSettings(ctx context.Context) (*types.MemorySettings, error)
	// SetEnabled flips the per-user opt out.
	SetEnabled(ctx context.Context, enabled bool) error
}
