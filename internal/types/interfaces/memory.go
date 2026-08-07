package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// Repositories -------------------------------------------------------------

// MemorySpaceRepository persists memory spaces.
//
// Every method takes the tenant explicitly rather than reading it from the
// context: memory is the one subsystem where a scoping mistake leaks one
// person's private notes to another, so the scope is a parameter the caller
// cannot forget to pass.
type MemorySpaceRepository interface {
	Create(ctx context.Context, space *types.MemorySpace) error
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.MemorySpace, error)
	GetByOwner(ctx context.Context, tenantID uint64, scopeType string, principal types.Principal) (*types.MemorySpace, error)
	Update(ctx context.Context, space *types.MemorySpace) error
	Delete(ctx context.Context, tenantID uint64, id string) error
	ListByTenant(ctx context.Context, tenantID uint64) ([]*types.MemorySpace, error)
	// ListActiveIDs returns space ids for background jobs (decay, retention).
	ListActiveIDs(ctx context.Context, limit, offset int) ([]*types.MemorySpace, error)
}

// MemoryPageRepository persists memory pages and their revisions.
type MemoryPageRepository interface {
	Create(ctx context.Context, page *types.MemoryPage) error
	// Update writes the page. When expectVersion > 0 the write is rejected on
	// a version mismatch, giving the UI optimistic locking; background writers
	// pass 0 and take last-write-wins.
	Update(ctx context.Context, page *types.MemoryPage, expectVersion int) error
	UpdateWithRevision(ctx context.Context, page *types.MemoryPage, rev *types.MemoryPageRevision, expectVersion int) error
	GetByID(ctx context.Context, spaceID, id string) (*types.MemoryPage, error)
	GetBySlug(ctx context.Context, spaceID, slug string) (*types.MemoryPage, error)
	GetBySlugs(ctx context.Context, spaceID string, slugs []string) ([]*types.MemoryPage, error)
	List(ctx context.Context, req *types.MemoryPageListRequest) ([]*types.MemoryPage, int64, error)
	ListAll(ctx context.Context, spaceID string) ([]*types.MemoryPage, error)
	ListByTypes(ctx context.Context, spaceID string, types_ []string, statuses []string, limit int) ([]*types.MemoryPage, error)
	Search(ctx context.Context, spaceID, query string, limit int) ([]*types.MemoryPage, error)
	Delete(ctx context.Context, spaceID, id string) error
	DeleteBySlugs(ctx context.Context, spaceID string, slugs []string) (int64, error)
	DeleteAll(ctx context.Context, spaceID string) (int64, error)
	Count(ctx context.Context, spaceID string, statuses []string) (int64, error)
	CountByType(ctx context.Context, spaceID string) (map[string]int64, error)
	// BumpHits records that these pages were injected into a prompt.
	BumpHits(ctx context.Context, spaceID string, pageIDs []string, seenAt time.Time) error
	ListRevisions(ctx context.Context, spaceID, pageID string) ([]*types.MemoryPageRevision, error)
	GetRevision(ctx context.Context, spaceID, pageID string, version int) (*types.MemoryPageRevision, error)
	// ListForDecay returns active pages whose strength should be recomputed.
	ListForDecay(ctx context.Context, spaceID string, limit int) ([]*types.MemoryPage, error)
}

// MemoryNoteRepository persists extracted observations.
type MemoryNoteRepository interface {
	CreateBatch(ctx context.Context, notes []*types.MemoryNote) error
	GetByID(ctx context.Context, spaceID, id string) (*types.MemoryNote, error)
	List(ctx context.Context, req *types.MemoryNoteListRequest) ([]*types.MemoryNote, int64, error)
	ListPending(ctx context.Context, spaceID string, limit int) ([]*types.MemoryNote, error)
	ExistsHash(ctx context.Context, spaceID, hash string) (bool, error)
	UpdateStatus(ctx context.Context, spaceID, id, status, mergedPageID string) error
	MarkExpired(ctx context.Context, spaceID string, before time.Time) (int64, error)
	Count(ctx context.Context, spaceID string, statuses []string) (int64, error)
	DeleteAll(ctx context.Context, spaceID string) (int64, error)
	DeleteByPage(ctx context.Context, spaceID, pageID string) (int64, error)
	ListAll(ctx context.Context, spaceID string) ([]*types.MemoryNote, error)
}

// MemoryAnchorRepository persists the memory-to-knowledge-base bridge.
type MemoryAnchorRepository interface {
	Upsert(ctx context.Context, anchor *types.MemoryAnchorUpsert) error
	ListBySpace(ctx context.Context, spaceID string, kbID string) ([]*types.MemoryAnchor, error)
	// ListOverlay returns the minimal projection the illumination layer needs.
	ListOverlay(ctx context.Context, spaceID, kbID, targetKind string) ([]types.MemoryOverlayAnchor, error)
	ListByPage(ctx context.Context, spaceID, memoryPageID string) ([]*types.MemoryAnchor, error)
	ListByTarget(ctx context.Context, spaceID, kbID, targetKind, targetRef string) ([]*types.MemoryAnchor, error)
	// AggregateByTarget powers the anonymised insights view; it deliberately
	// spans spaces and therefore never returns per-space detail.
	AggregateByTarget(ctx context.Context, tenantID uint64, kbID string) ([]types.MemoryAnchorAggregate, error)
	Delete(ctx context.Context, spaceID, id string) error
	DeleteByPage(ctx context.Context, spaceID, memoryPageID string) (int64, error)
	DeleteAll(ctx context.Context, spaceID string) (int64, error)
	Count(ctx context.Context, spaceID string) (int64, error)
	ListAnchoredKBs(ctx context.Context, spaceID string) ([]string, error)
	ListAll(ctx context.Context, spaceID string) ([]*types.MemoryAnchor, error)
}

// Services -----------------------------------------------------------------

// MemorySettingsService resolves the layered settings for a request.
type MemorySettingsService interface {
	// Resolve returns the effective settings for the caller, folding
	// deployment, tenant, agent, user and space layers in that order.
	Resolve(ctx context.Context, opts types.MemorySettingsResolveOptions) (types.MemorySettingsResolution, error)
	// View renders the settings for a UI at the given editable layer.
	View(ctx context.Context, opts types.MemorySettingsResolveOptions, editableLevel string) (*types.MemorySettingsView, error)
	// UpdateTenant patches the workspace layer.
	UpdateTenant(ctx context.Context, tenantID uint64, patch types.MemorySettingsPatch) ([]string, error)
	// UpdateUser patches the user layer.
	UpdateUser(ctx context.Context, tenantID uint64, userID string, patch types.MemorySettingsPatch) ([]string, error)
	// UpdateSpace patches the space layer.
	UpdateSpace(ctx context.Context, tenantID uint64, spaceID string, patch types.MemorySettingsPatch) ([]string, error)
	// Capabilities reports which features are usable in this deployment.
	Capabilities(ctx context.Context, settings types.MemorySettings) map[string]types.MemoryCapability
}

// MemoryService is the façade the rest of the application talks to.
type MemoryService interface {
	// EnsureSpace resolves (and lazily creates) the caller's memory space.
	// Returns nil without error when memory is disabled for this caller, so
	// callers can treat "off" as an ordinary state rather than a failure.
	EnsureSpace(ctx context.Context) (*types.MemorySpace, error)
	GetSpace(ctx context.Context) (*types.MemorySpace, error)
	SpaceView(ctx context.Context) (*types.MemorySpaceView, error)
	UpdateSpaceConfig(ctx context.Context, patch types.MemorySettingsPatch) ([]string, error)

	ListPages(ctx context.Context, req *types.MemoryPageListRequest) (*types.MemoryPageListResponse, error)
	GetPage(ctx context.Context, slug string) (*types.MemoryPage, error)
	WritePage(ctx context.Context, req *types.MemoryPageWriteRequest) (*types.MemoryPage, error)
	DeletePage(ctx context.Context, slug string) error
	SearchPages(ctx context.Context, query string, limit int) ([]*types.MemoryPage, error)
	ListRevisions(ctx context.Context, slug string) ([]*types.MemoryPageRevision, error)
	RevertPage(ctx context.Context, req *types.MemoryRevertRequest) (*types.MemoryPage, error)

	ListNotes(ctx context.Context, req *types.MemoryNoteListRequest) (*types.MemoryNoteListResponse, error)
	PromoteNote(ctx context.Context, noteID string, req *types.MemoryNotePromoteRequest) (*types.MemoryPage, error)
	RejectNote(ctx context.Context, noteID string) error

	Graph(ctx context.Context, req *types.MemoryGraphRequest) (*types.MemoryGraphData, error)
	Stats(ctx context.Context) (*types.MemoryStats, error)

	ListAnchors(ctx context.Context, kbID string) ([]*types.MemoryAnchorView, error)
	AddAnchor(ctx context.Context, req *types.MemoryAnchorRequest) (*types.MemoryAnchor, error)
	DeleteAnchor(ctx context.Context, anchorID string) error

	Forget(ctx context.Context, req *types.MemoryForgetRequest) (*types.MemoryForgetResponse, error)
	Export(ctx context.Context) (*types.MemoryExport, error)

	// Coverage reports how much of a knowledge base the caller has lit up.
	Coverage(ctx context.Context, kbID string, pages []types.MemoryCoveragePage) (*types.MemoryCoverage, error)
	// Overlay returns per-wiki-slug illumination for the caller.
	Overlay(ctx context.Context, kbID string) (map[string]types.MemoryOverlayNode, error)
	// Insights returns the anonymised aggregate view for workspace admins.
	Insights(ctx context.Context, kbID string, pages []types.MemoryInsightPage) (*types.MemoryInsightsResponse, error)
}

// MemoryRecallService selects memories for injection and records usage.
type MemoryRecallService interface {
	// Recall gathers the memories worth injecting for one turn. It must never
	// return an error that fails the chat: on any problem it returns nil and
	// the pipeline proceeds without memory.
	Recall(ctx context.Context, req types.MemoryRecallRequest) *types.MemoryRecallResult
	// RecordUsage marks the injected pages as hit.
	RecordUsage(ctx context.Context, spaceID string, slugs []string)
	// RecordRetrievalAnchors records asked_about anchors for cited wiki pages.
	// This is the zero-cost source of illumination data.
	RecordRetrievalAnchors(ctx context.Context, req types.MemoryAnchorRecordRequest)
}

// MemoryWriterService owns the asynchronous write path.
type MemoryWriterService interface {
	// ConsiderSession applies the write-mode gate and enqueues extraction.
	ConsiderSession(ctx context.Context, req types.MemoryExtractTrigger)
	// Extract runs one extraction window. Called by the task worker.
	Extract(ctx context.Context, tenantID uint64, spaceID, sessionID string) error
	// Consolidate folds pending notes into pages. Called by the task worker.
	Consolidate(ctx context.Context, tenantID uint64, spaceID string) error
	// Decay applies strength decay, archival and retention to one space.
	Decay(ctx context.Context, tenantID uint64, spaceID string) error
	// DecayAll sweeps every active space; the scheduled entry point.
	DecayAll(ctx context.Context) error
	// RememberExplicit stores a memory the user asked for directly.
	RememberExplicit(ctx context.Context, req types.MemoryExplicitWriteRequest) (*types.MemoryPage, error)
}
