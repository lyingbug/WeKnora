package types

import "time"

// Request and response shapes for the memory API. They are deliberately
// separate from the storage models: the wire contract should be able to stay
// stable while columns move around, and a few storage fields (tenant id, raw
// note refs) have no business being echoed back to a browser.

// ---------------------------------------------------------------------------
// Pages
// ---------------------------------------------------------------------------

// MemoryPageListRequest filters the page list.
type MemoryPageListRequest struct {
	SpaceID  string
	Types    []string
	Statuses []string
	Query    string
	Page     int
	PageSize int
	SortBy   string
	Desc     bool
}

// Normalize applies defaults and caps to a page list request.
func (r *MemoryPageListRequest) Normalize() {
	if r.Page < 1 {
		r.Page = 1
	}
	if r.PageSize < 1 {
		r.PageSize = 20
	}
	if r.PageSize > 200 {
		r.PageSize = 200
	}
	switch r.SortBy {
	case "created_at", "updated_at", "strength", "hit_count", "title", "slug":
	default:
		r.SortBy = "updated_at"
		r.Desc = true
	}
	if len(r.Statuses) == 0 {
		r.Statuses = []string{MemoryPageStatusActive}
	}
}

// MemoryPageListResponse is a paginated page list.
type MemoryPageListResponse struct {
	Pages    []*MemoryPage `json:"pages"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// MemoryPageWriteRequest creates or updates a memory page.
type MemoryPageWriteRequest struct {
	Slug       string            `json:"slug"`
	Title      string            `json:"title"`
	PageType   string            `json:"page_type"`
	Content    string            `json:"content"`
	Summary    string            `json:"summary"`
	Structured *MemoryPreference `json:"structured,omitempty"`
	Aliases    []string          `json:"aliases,omitempty"`
	FolderPath []string          `json:"folder_path,omitempty"`
	Pinned     *bool             `json:"pinned,omitempty"`
	Status     string            `json:"status,omitempty"`
	Confidence *float64          `json:"confidence,omitempty"`
	// Version enables optimistic locking on update. Zero skips the check,
	// which is what agent and pipeline writers use.
	Version int `json:"version,omitempty"`
	// EditSource records who is writing; defaults to user for API calls.
	EditSource string `json:"-"`
	EditorID   string `json:"-"`
}

// MemoryRevertRequest rolls a page back to an earlier revision.
type MemoryRevertRequest struct {
	Slug string `json:"slug"    binding:"required"`
	// Version is the revision to restore.
	Version int `json:"version" binding:"required"`
	// ExpectedVersion is the caller's view of the page's current version. When
	// supplied, a revert that would overwrite an edit made since the caller read
	// the page is refused with a conflict instead of silently winning.
	ExpectedVersion int `json:"expected_version,omitempty"`
}

// ---------------------------------------------------------------------------
// Notes
// ---------------------------------------------------------------------------

// MemoryNoteListRequest filters the note (inbox) list.
type MemoryNoteListRequest struct {
	SpaceID  string
	Statuses []string
	Types    []string
	Page     int
	PageSize int
}

// Normalize applies defaults and caps to a note list request.
func (r *MemoryNoteListRequest) Normalize() {
	if r.Page < 1 {
		r.Page = 1
	}
	if r.PageSize < 1 {
		r.PageSize = 20
	}
	if r.PageSize > 200 {
		r.PageSize = 200
	}
}

// MemoryNoteListResponse is a paginated note list.
type MemoryNoteListResponse struct {
	Notes    []*MemoryNote `json:"notes"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// MemoryNotePromoteRequest turns a pending note into a page, optionally with
// user edits applied first.
type MemoryNotePromoteRequest struct {
	Statement string `json:"statement,omitempty"`
	NoteType  string `json:"note_type,omitempty"`
	Slug      string `json:"slug,omitempty"`
	Title     string `json:"title,omitempty"`
}

// ---------------------------------------------------------------------------
// Anchors
// ---------------------------------------------------------------------------

// MemoryAnchorUpsert describes an anchor to create or reinforce.
type MemoryAnchorUpsert struct {
	SpaceID         string
	TenantID        uint64
	MemoryPageID    string
	KnowledgeBaseID string
	TargetKind      string
	TargetRef       string
	Relation        string
	Source          string
	Confidence      float64
	Evidence        MemoryAnchorEvidence
	// Delta is added to hit_count; defaults to 1 when zero.
	Delta int
}

// MemoryAnchorRequest is the API body for a user-asserted anchor.
type MemoryAnchorRequest struct {
	KnowledgeBaseID string `json:"knowledge_base_id" binding:"required"`
	TargetKind      string `json:"target_kind"`
	TargetRef       string `json:"target_ref"       binding:"required"`
	Relation        string `json:"relation"         binding:"required"`
	MemoryPageSlug  string `json:"memory_page_slug,omitempty"`
}

// MemoryAnchorView is an anchor plus the memory page it belongs to, which is
// what the "your memory" sidebar on a wiki page needs.
type MemoryAnchorView struct {
	*MemoryAnchor
	MemoryPageSlug  string `json:"memory_page_slug,omitempty"`
	MemoryPageTitle string `json:"memory_page_title,omitempty"`
}

// ---------------------------------------------------------------------------
// Graph
// ---------------------------------------------------------------------------

// Memory graph modes.
const (
	// MemoryGraphModePersonal shows only memory pages.
	MemoryGraphModePersonal = "personal"
	// MemoryGraphModeBridged adds the knowledge-base pages those memories are
	// anchored to, so the user can see where their understanding attaches to
	// the organisation's knowledge.
	MemoryGraphModeBridged = "bridged"
)

// Memory graph node kinds.
const (
	MemoryGraphNodeMemory = "memory"
	MemoryGraphNodeWiki   = "wiki"
)

// Memory graph edge kinds.
const (
	MemoryGraphEdgeLink   = "link"
	MemoryGraphEdgeAnchor = "anchor"
)

// MemoryGraphRequest selects a slice of the memory graph.
type MemoryGraphRequest struct {
	SpaceID string
	Mode    string
	Center  string
	Depth   int
	Types   []string
	Limit   int
}

// Normalize applies defaults and caps.
func (r *MemoryGraphRequest) Normalize() {
	if r.Mode != MemoryGraphModeBridged {
		r.Mode = MemoryGraphModePersonal
	}
	if r.Depth < 1 {
		r.Depth = 1
	}
	if r.Depth > 4 {
		r.Depth = 4
	}
	if r.Limit <= 0 {
		r.Limit = 200
	}
	if r.Limit > 1000 {
		r.Limit = 1000
	}
}

// MemoryGraphNode is one node in the memory graph.
type MemoryGraphNode struct {
	// ID is "memory:<slug>" or "wiki:<kbID>:<slug>" so the two node families
	// cannot collide in the frontend's lookup maps.
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	// Type is the memory page type for memory nodes, the wiki page type for
	// wiki nodes.
	Type            string  `json:"type,omitempty"`
	LinkCount       int     `json:"link_count"`
	Strength        float64 `json:"strength,omitempty"`
	KnowledgeBaseID string  `json:"knowledge_base_id,omitempty"`
}

// MemoryGraphEdge connects two nodes.
type MemoryGraphEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Kind     string `json:"kind"`
	Relation string `json:"relation,omitempty"`
}

// MemoryGraphMeta describes what the slice covers.
type MemoryGraphMeta struct {
	Mode      string `json:"mode"`
	Total     int    `json:"total"`
	Returned  int    `json:"returned"`
	Truncated bool   `json:"truncated"`
	Center    string `json:"center,omitempty"`
	Depth     int    `json:"depth,omitempty"`
}

// MemoryGraphData is the graph payload the UI renders.
type MemoryGraphData struct {
	Nodes []MemoryGraphNode `json:"nodes"`
	Edges []MemoryGraphEdge `json:"edges"`
	Meta  MemoryGraphMeta   `json:"meta"`
}

// ---------------------------------------------------------------------------
// Space, stats, settings
// ---------------------------------------------------------------------------

// MemoryCapability reports whether a feature is usable in this deployment.
type MemoryCapability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Capability keys and reasons.
const (
	MemoryCapabilitySharedSpace     = "shared_space"
	MemoryCapabilityRelevanceRecall = "relevance_recall"
	MemoryCapabilityAutoExtract     = "auto_extract"
	MemoryCapabilityInsights        = "insights"

	MemoryCapabilityReasonLite     = "lite_edition"
	MemoryCapabilityReasonDisabled = "disabled_by_settings"
)

// MemoryStats summarises a space for the memory centre header.
type MemoryStats struct {
	TotalPages    int64            `json:"total_pages"`
	ActivePages   int64            `json:"active_pages"`
	ArchivedPages int64            `json:"archived_pages"`
	PendingNotes  int64            `json:"pending_notes"`
	TotalAnchors  int64            `json:"total_anchors"`
	ByType        map[string]int64 `json:"by_type"`
	LastUpdatedAt *time.Time       `json:"last_updated_at,omitempty"`
	AnchoredKBs   []string         `json:"anchored_kbs,omitempty"`
}

// MemorySpaceView is the space plus everything the UI needs to render its
// header and decide which controls to show.
type MemorySpaceView struct {
	Space        *MemorySpace                `json:"space"`
	Stats        MemoryStats                 `json:"stats"`
	Capabilities map[string]MemoryCapability `json:"capabilities"`
}

// MemorySettingsView is the settings API response: effective values with their
// provenance, plus the descriptor catalogue so the UI can render controls,
// bounds and help text without hardcoding anything.
type MemorySettingsView struct {
	Values      map[string]MemorySettingValue `json:"values"`
	Descriptors []MemorySettingDescriptor     `json:"descriptors"`
	// EditableLevel is the layer the caller is editing at.
	EditableLevel string `json:"editable_level"`
	// Editable lists the keys the caller can actually change right now.
	Editable     map[string]bool             `json:"editable"`
	Capabilities map[string]MemoryCapability `json:"capabilities"`
}

// MemorySettingsUpdateRequest patches settings at one layer.
type MemorySettingsUpdateRequest struct {
	Settings MemorySettingsPatch `json:"settings" binding:"required"`
}

// MemorySettingsUpdateResponse echoes the new state plus any adjustments the
// server made, so the UI can explain a clamped value instead of silently
// showing something different from what the user typed.
type MemorySettingsUpdateResponse struct {
	View  MemorySettingsView `json:"view"`
	Notes []string           `json:"notes,omitempty"`
}

// ---------------------------------------------------------------------------
// Forget / export
// ---------------------------------------------------------------------------

// MemoryForgetRequest deletes memories in bulk.
type MemoryForgetRequest struct {
	// Scope is "slugs", "type" or "all".
	Scope string   `json:"scope" binding:"required"`
	Slugs []string `json:"slugs,omitempty"`
	Types []string `json:"types,omitempty"`
	// PurgeNotes also clears the underlying observations. Without it the
	// evidence trail survives, which is usually what a user wants when they
	// delete a single wrong memory but not when they say "forget everything".
	PurgeNotes bool `json:"purge_notes,omitempty"`
}

// MemoryForgetResponse reports what was removed.
type MemoryForgetResponse struct {
	PagesDeleted   int64 `json:"pages_deleted"`
	NotesDeleted   int64 `json:"notes_deleted"`
	AnchorsDeleted int64 `json:"anchors_deleted"`
}

// MemoryExport is the portable dump of a space.
type MemoryExport struct {
	ExportedAt time.Time           `json:"exported_at"`
	Space      *MemorySpace        `json:"space"`
	Pages      []*MemoryPage       `json:"pages"`
	Notes      []*MemoryNote       `json:"notes"`
	Anchors    []*MemoryAnchor     `json:"anchors"`
	Settings   MemorySettingsPatch `json:"settings,omitempty"`
}

// ---------------------------------------------------------------------------
// Insights
// ---------------------------------------------------------------------------

// Insight kinds.
const (
	MemoryInsightThinButHot  = "thin_but_hot"
	MemoryInsightContested   = "contested"
	MemoryInsightNeverLit    = "never_lit"
	MemoryInsightMissingPage = "missing_page"
)

// MemoryInsight is one aggregated, anonymised observation about a knowledge
// base derived from how people actually use it.
type MemoryInsight struct {
	Kind           string `json:"kind"`
	TargetRef      string `json:"target_ref"`
	Title          string `json:"title,omitempty"`
	ContentLength  int    `json:"content_length,omitempty"`
	Interactions   int    `json:"interactions"`
	DistinctPeople int    `json:"distinct_people"`
	Detail         string `json:"detail,omitempty"`
}

// MemoryInsightsResponse carries the insight list plus the privacy parameters
// it was computed under, so the reader knows what was suppressed.
type MemoryInsightsResponse struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	KAnonymity      int    `json:"k_anonymity"`
	Suppressed      int    `json:"suppressed"`
	// SuppressedNeverLit counts untouched pages left out of the capped list, so a
	// reader can tell a short list from a truncated one.
	SuppressedNeverLit int             `json:"suppressed_never_lit,omitempty"`
	Insights           []MemoryInsight `json:"insights"`
}

// MemoryAnchorAggregate is one target's usage summed across every space. It
// carries a distinct-people count and never a space id, because the whole point
// of the aggregate is that an administrator learns about the knowledge base
// without learning about any individual.
type MemoryAnchorAggregate struct {
	TargetKind     string
	TargetRef      string
	Relation       string
	Interactions   int
	DistinctSpaces int
}

// MemoryInsightPage is the wiki-page projection insights need.
type MemoryInsightPage struct {
	Slug          string
	Title         string
	ContentLength int
}

// ---------------------------------------------------------------------------
// Cross-service request shapes
// ---------------------------------------------------------------------------

// MemorySettingsResolveOptions selects which layers to fold.
//
// Zero values simply skip a layer, so a background job that only knows a tenant
// and a space resolves correctly without inventing an agent or user.
type MemorySettingsResolveOptions struct {
	TenantID uint64
	UserID   string
	AgentID  string
	SpaceID  string
	// AgentPatch lets a caller that already loaded the agent pass its settings
	// straight through instead of forcing a second lookup on the chat path.
	AgentPatch MemorySettingsPatch
	// SpacePatch does the same for an already-loaded space.
	SpacePatch MemorySettingsPatch
}

// MemoryRecallRequest is one turn's recall input.
type MemoryRecallRequest struct {
	TenantID uint64
	SpaceID  string
	// Query is the (rewritten) user question. Callers must fall back to the
	// original question when rewriting produced nothing: since v0.7.2 an
	// unparsable rewrite leaves RewriteQuery empty rather than echoing text.
	Query    string
	Settings MemorySettings
	// KnowledgeBaseIDs scopes which anchors may become ranking hints.
	KnowledgeBaseIDs []string
	// Language selects the labels used when rendering the injected block.
	Language string
}

// MemoryAnchorRecordRequest records retrieval-time anchors.
type MemoryAnchorRecordRequest struct {
	TenantID  uint64
	SpaceID   string
	SessionID string
	MessageID string
	Query     string
	Settings  MemorySettings
	// Targets are the wiki pages that were actually cited.
	Targets []MemoryAnchorTarget
}

// MemoryAnchorTarget identifies one cited knowledge-base object.
type MemoryAnchorTarget struct {
	KnowledgeBaseID string
	TargetKind      string
	TargetRef       string
}

// MemoryExtractTrigger asks the writer to consider a session for extraction.
type MemoryExtractTrigger struct {
	TenantID  uint64
	SpaceID   string
	SessionID string
	Settings  MemorySettings
	// UserText is the newest user message, used by the keyword gate.
	UserText string
	// TurnIndex is how many completed exchanges the session now has.
	TurnIndex int
	// Explicit marks a user-initiated "remember this", which bypasses the gate.
	Explicit bool
	// KnowledgeBaseIDs are the knowledge bases the conversation was scoped to.
	// Anchor resolution needs them: an entity name only means something relative
	// to a particular wiki.
	KnowledgeBaseIDs []string
	// ChatModelID is the model this turn used. Extraction falls back to it when
	// no dedicated extraction model is configured, which is what the setting's
	// own description promises.
	ChatModelID string
	// MessageID is the user message the trigger came from, kept so an explicit
	// "remember this" has the same evidence trail as an extracted memory.
	MessageID string
	// AgentID is the agent that served the turn, if any. The background task
	// re-resolves settings and needs the same layers to reach the same verdict.
	AgentID string
	// SessionOwnerID is the sessions.user_id scope of whoever asked. Captured
	// here because it is only knowable inside the request: the mapping from a
	// principal to its session scope differs per channel and, for tenant API
	// keys, depends on the key id, none of which a background task can recover.
	SessionOwnerID string
}

// MemoryExplicitWriteRequest stores a memory the user asked for by hand.
type MemoryExplicitWriteRequest struct {
	TenantID  uint64
	SpaceID   string
	SessionID string
	MessageID string
	Statement string
	NoteType  string
	Source    string
	Settings  MemorySettings
}
