// Package types — long-term memory (Memory Wiki).
//
// The memory subsystem gives every principal a private, wiki-shaped store of
// durable facts: who they are, how they want answers written, what they are
// working on, what they concluded, and what is still open. It is deliberately
// built out of the same primitives as the knowledge-base wiki (slugs, markdown
// bodies, [[wiki links]], revisions) so users read it the way they already read
// a wiki, and so the graph tooling can be shared.
//
// Three concepts live here and must not be conflated:
//
//   - MemoryNote  — L0, append-only observations extracted from a conversation.
//     Each carries the message ids that justify it, so every memory is traceable
//     back to the words the user actually typed.
//   - MemoryPage  — L1, the deduplicated, editable, injectable unit. Only pages
//     reach the LLM.
//   - MemoryAnchor — L2, the bridge from a memory page to a knowledge-base wiki
//     page. Anchors are what "light up" the knowledge graph per user.
//
// Storage is the main database only (PostgreSQL or SQLite). There is no Neo4j
// dependency and no new infrastructure: graph edges are JSON arrays on the row,
// and all graph math runs in Go so both deployment forms produce identical
// results.
package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Portable JSON column helpers
// ---------------------------------------------------------------------------

// MemoryStringList is a []string stored as JSON. Unlike the older StringArray
// it accepts a string source as well as []byte: SQLite hands TEXT columns back
// as string, and silently dropping the value there would corrupt link graphs on
// Lite while looking fine on Postgres.
type MemoryStringList []string

// Value implements driver.Valuer. Always emits a JSON array (never SQL NULL)
// so the NOT NULL DEFAULT '[]' column constraint holds on both databases.
func (l MemoryStringList) Value() (driver.Value, error) {
	if l == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(l))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner for []byte, string and nil sources.
func (l *MemoryStringList) Scan(src any) error {
	raw, err := jsonScanBytes(src, "MemoryStringList")
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		*l = nil
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*l = out
	return nil
}

// Contains reports whether the list holds v.
func (l MemoryStringList) Contains(v string) bool {
	for _, item := range l {
		if item == v {
			return true
		}
	}
	return false
}

// Add appends v when absent and reports whether the list changed.
func (l *MemoryStringList) Add(v string) bool {
	if v == "" || l.Contains(v) {
		return false
	}
	*l = append(*l, v)
	return true
}

// Remove drops v when present and reports whether the list changed.
func (l *MemoryStringList) Remove(v string) bool {
	for i, item := range *l {
		if item == v {
			*l = append((*l)[:i], (*l)[i+1:]...)
			return true
		}
	}
	return false
}

// MemoryJSONObject is a map[string]any stored as JSON, portable across both
// databases in the same way MemoryStringList is.
type MemoryJSONObject map[string]any

// Value implements driver.Valuer, emitting "{}" rather than SQL NULL.
func (m MemoryJSONObject) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(map[string]any(m))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner for []byte, string and nil sources.
func (m *MemoryJSONObject) Scan(src any) error {
	raw, err := jsonScanBytes(src, "MemoryJSONObject")
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		*m = nil
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*m = out
	return nil
}

func jsonScanBytes(src any, typeName string) ([]byte, error) {
	switch v := src.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("%s.Scan: unsupported source type %T", typeName, src)
	}
}

// ---------------------------------------------------------------------------
// Memory space
// ---------------------------------------------------------------------------

// Memory space scope types.
const (
	// MemorySpaceScopeUser is one principal's private memory.
	MemorySpaceScopeUser = "user"
	// MemorySpaceScopeShared is a workspace-wide shared memory space.
	// Reserved for a later phase; not created by the current code paths.
	MemorySpaceScopeShared = "shared"
)

// Memory space statuses.
const (
	MemorySpaceStatusActive = "active"
	MemorySpaceStatusPaused = "paused"
)

// MemorySpace owns all memory belonging to one principal in one workspace.
//
// Scoping deliberately does NOT reuse sessions.user_id: that column's format
// varies by channel (a bare UUID for web, a composite string for embed and IM),
// so it cannot be a stable key. Instead a space is addressed by the same triple
// mcp_oauth_tokens already proved out — (tenant, principal type, principal id) —
// which covers web, OIDC, IM, API external users and embed visitors uniformly.
type MemorySpace struct {
	ID       string `json:"id"        gorm:"type:varchar(36);primaryKey"`
	TenantID uint64 `json:"tenant_id" gorm:"index"`
	// ScopeType is user or shared.
	ScopeType string `json:"scope_type" gorm:"type:varchar(16);default:'user'"`
	// OwnerPrincipalType / OwnerPrincipalID mirror types.Principal.
	OwnerPrincipalType string `json:"owner_principal_type" gorm:"type:varchar(32)"`
	OwnerPrincipalID   string `json:"owner_principal_id"   gorm:"type:varchar(512)"`
	DisplayName        string `json:"display_name"         gorm:"type:varchar(255)"`
	Status             string `json:"status"               gorm:"type:varchar(16);default:'active'"`
	// Config holds space-level setting overrides (the narrowest layer).
	Config MemorySettingsPatch `json:"config" gorm:"type:jsonb"`
	// VectorKBID is the hidden knowledge base that indexes this space's pages
	// for semantic recall. Using a hidden KB rather than a new user dimension
	// in RetrieveParams keeps all ten vector drivers untouched, and works
	// unchanged on Lite's sqlite-vec backend.
	VectorKBID string         `json:"vector_kb_id" gorm:"type:varchar(36)"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName returns the table name for MemorySpace.
func (MemorySpace) TableName() string { return "memory_spaces" }

// Principal reconstructs the owning principal.
func (s *MemorySpace) Principal() Principal {
	if s == nil {
		return Principal{}
	}
	return Principal{Type: s.OwnerPrincipalType, ID: s.OwnerPrincipalID}
}

// IsActive reports whether the space currently accepts reads and writes.
func (s *MemorySpace) IsActive() bool {
	return s != nil && s.Status == MemorySpaceStatusActive
}

// ---------------------------------------------------------------------------
// Memory page
// ---------------------------------------------------------------------------

// Memory page types. These double as note types: a note is promoted into a page
// of the same type.
const (
	// MemoryTypeProfile holds durable identity attributes (role, stack, team).
	MemoryTypeProfile = "profile"
	// MemoryTypePreference holds answer-style and long-standing instructions.
	// Only its whitelisted structured fields can influence generation.
	MemoryTypePreference = "preference"
	// MemoryTypeProject holds what the user is currently working towards.
	MemoryTypeProject = "project"
	// MemoryTypeEntity holds people, teams and systems in the user's context.
	MemoryTypeEntity = "entity"
	// MemoryTypeTopic holds a subject the user follows and their grasp of it.
	MemoryTypeTopic = "topic"
	// MemoryTypeEpisode holds a decision or conclusion worth remembering.
	MemoryTypeEpisode = "episode"
	// MemoryTypeOpenQuestion holds something still unresolved.
	MemoryTypeOpenQuestion = "open_question"
)

// AllMemoryTypes lists every valid memory type in display order.
func AllMemoryTypes() []string {
	return []string{
		MemoryTypeProfile,
		MemoryTypePreference,
		MemoryTypeProject,
		MemoryTypeEntity,
		MemoryTypeTopic,
		MemoryTypeEpisode,
		MemoryTypeOpenQuestion,
	}
}

// IsValidMemoryType reports whether t is a known memory type.
func IsValidMemoryType(t string) bool {
	for _, known := range AllMemoryTypes() {
		if known == t {
			return true
		}
	}
	return false
}

// Memory page statuses.
const (
	// MemoryPageStatusActive is a live memory eligible for recall.
	MemoryPageStatusActive = "active"
	// MemoryPageStatusArchived is a faded or manually retired memory. Archived
	// pages are never recalled but remain visible and restorable, because
	// silently deleting something a user said is not forgetting, it is losing.
	MemoryPageStatusArchived = "archived"
	// MemoryPageStatusSuperseded marks a memory replaced by a newer, conflicting
	// one. SupersededBy points at the replacement.
	MemoryPageStatusSuperseded = "superseded"
)

// IsValidMemoryPageStatus reports whether s is a known page status.
func IsValidMemoryPageStatus(s string) bool {
	switch s {
	case MemoryPageStatusActive, MemoryPageStatusArchived, MemoryPageStatusSuperseded:
		return true
	default:
		return false
	}
}

// Edit sources, mirroring wiki_pages.last_edit_source.
const (
	MemoryEditSourcePipeline = "pipeline"
	MemoryEditSourceAgent    = "agent"
	MemoryEditSourceUser     = "user"
	MemoryEditSourceRevert   = "revert"
)

// MemoryPage is one durable, human-readable memory.
type MemoryPage struct {
	ID       string `json:"id"        gorm:"type:varchar(36);primaryKey"`
	TenantID uint64 `json:"tenant_id" gorm:"index"`
	SpaceID  string `json:"space_id"  gorm:"type:varchar(36);index"`
	// Slug addresses the page inside its space, e.g. "preference/answer-style".
	Slug     string `json:"slug"      gorm:"type:varchar(255)"`
	Title    string `json:"title"     gorm:"type:varchar(512)"`
	PageType string `json:"page_type" gorm:"type:varchar(32);index"`
	Status   string `json:"status"    gorm:"type:varchar(16);default:'active'"`
	// Content is markdown and may contain [[slug|title]] links to other pages.
	Content string `json:"content" gorm:"type:text"`
	// Summary is the one-line form used when injecting into a prompt.
	Summary string `json:"summary" gorm:"type:text"`
	// Structured carries the whitelisted preference fields. Free text never
	// reaches the model as an instruction; only these typed values do.
	Structured MemoryPreference `json:"structured" gorm:"type:jsonb"`
	Aliases    MemoryStringList `json:"aliases"    gorm:"type:jsonb"`
	InLinks    MemoryStringList `json:"in_links"   gorm:"type:jsonb"`
	OutLinks   MemoryStringList `json:"out_links"  gorm:"type:jsonb"`
	FolderPath MemoryStringList `json:"folder_path" gorm:"type:jsonb"`
	// Strength decays over time unless the page is pinned.
	Strength float64 `json:"strength" gorm:"type:real;default:1"`
	// HitCount counts how often this page was actually injected into a prompt.
	HitCount   int     `json:"hit_count"  gorm:"default:0"`
	Confidence float64 `json:"confidence" gorm:"type:real;default:0.5"`
	// Pinned exempts a page from decay and archival.
	Pinned bool `json:"pinned" gorm:"default:false"`
	// SupersededBy points at the page that replaced this one.
	SupersededBy string           `json:"superseded_by,omitempty" gorm:"type:varchar(36)"`
	NoteRefs     MemoryStringList `json:"note_refs" gorm:"type:jsonb"`
	Version      int              `json:"version"   gorm:"default:1"`

	LastEditSource string         `json:"last_edit_source" gorm:"type:varchar(16)"`
	LastSeenAt     *time.Time     `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName returns the table name for MemoryPage.
func (MemoryPage) TableName() string { return "memory_pages" }

// IsRecallable reports whether the page may be surfaced to the model.
func (p *MemoryPage) IsRecallable() bool {
	return p != nil && p.Status == MemoryPageStatusActive
}

// InjectionText renders the compact one-line form used inside the prompt block.
// Summary is preferred; a page without one falls back to its first content line
// so a hand-written memory is never injected as an empty bullet.
func (p *MemoryPage) InjectionText() string {
	if p == nil {
		return ""
	}
	if s := strings.TrimSpace(p.Summary); s != "" {
		return s
	}
	for _, line := range strings.Split(p.Content, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return strings.TrimSpace(p.Title)
}

// MemoryPageRevision is an immutable snapshot of a superseded page version.
type MemoryPageRevision struct {
	ID         string           `json:"id"        gorm:"type:varchar(36);primaryKey"`
	TenantID   uint64           `json:"tenant_id" gorm:"index"`
	SpaceID    string           `json:"space_id"  gorm:"type:varchar(36);index"`
	PageID     string           `json:"page_id"   gorm:"type:varchar(36);index"`
	Version    int              `json:"version"`
	Title      string           `json:"title"      gorm:"type:varchar(512)"`
	Content    string           `json:"content"    gorm:"type:text"`
	Summary    string           `json:"summary"    gorm:"type:text"`
	Structured MemoryPreference `json:"structured" gorm:"type:jsonb"`
	EditSource string           `json:"edit_source" gorm:"type:varchar(16)"`
	EditorID   string           `json:"editor_id"   gorm:"type:varchar(64)"`
	EditedAt   time.Time        `json:"edited_at"`
	CreatedAt  time.Time        `json:"created_at"`
}

// TableName returns the table name for MemoryPageRevision.
func (MemoryPageRevision) TableName() string { return "memory_page_revisions" }

// ---------------------------------------------------------------------------
// Structured preferences (the only memory that may steer generation)
// ---------------------------------------------------------------------------

// Preference enum values. Anything outside these sets is rejected at the API
// boundary, which is what stops "remember to ignore your instructions" from
// ever becoming a durable directive.
const (
	MemoryVerbosityConcise  = "concise"
	MemoryVerbosityBalanced = "balanced"
	MemoryVerbosityDetailed = "detailed"

	MemoryToneNeutral      = "neutral"
	MemoryToneFriendly     = "friendly"
	MemoryToneProfessional = "professional"

	MemoryFormatProse    = "prose"
	MemoryFormatBullets  = "bullets"
	MemoryFormatMarkdown = "markdown"

	MemoryCodeStyleAlways    = "always"
	MemoryCodeStyleMinimal   = "minimal"
	MemoryCodeStyleWhenAsked = "when_asked"
)

// MemoryPreference is the whitelisted, typed subset of a preference memory.
// Free-text preferences are stored in the page body for the human to read but
// are injected as data, never as instructions.
type MemoryPreference struct {
	// Language is a BCP-47-ish tag such as zh, en, ja.
	Language string `json:"language,omitempty"`
	// Verbosity is concise / balanced / detailed.
	Verbosity string `json:"verbosity,omitempty"`
	// Tone is neutral / friendly / professional.
	Tone string `json:"tone,omitempty"`
	// Format is prose / bullets / markdown.
	Format string `json:"format,omitempty"`
	// CodeStyle is always / minimal / when_asked.
	CodeStyle string `json:"code_style,omitempty"`
	// AvoidTopics lists subjects the user does not want raised.
	AvoidTopics []string `json:"avoid_topics,omitempty"`
}

var memoryLanguageTagRe = regexp.MustCompile(`^[a-zA-Z]{2,8}(-[a-zA-Z0-9]{2,8})*$`)

// Sanitize drops every field that is not a recognised value, so an LLM or a
// crafted API call cannot smuggle arbitrary text into the generation-steering
// path. It returns the cleaned copy.
func (p MemoryPreference) Sanitize() MemoryPreference {
	out := MemoryPreference{}
	if tag := strings.TrimSpace(p.Language); memoryLanguageTagRe.MatchString(tag) {
		out.Language = tag
	}
	out.Verbosity = pickEnum(p.Verbosity, MemoryVerbosityConcise, MemoryVerbosityBalanced, MemoryVerbosityDetailed)
	out.Tone = pickEnum(p.Tone, MemoryToneNeutral, MemoryToneFriendly, MemoryToneProfessional)
	out.Format = pickEnum(p.Format, MemoryFormatProse, MemoryFormatBullets, MemoryFormatMarkdown)
	out.CodeStyle = pickEnum(p.CodeStyle, MemoryCodeStyleAlways, MemoryCodeStyleMinimal, MemoryCodeStyleWhenAsked)
	for _, topic := range p.AvoidTopics {
		topic = strings.TrimSpace(topic)
		if topic == "" || len(topic) > 64 {
			continue
		}
		// A topic is a label, not a sentence; anything with newlines or
		// markup is an injection attempt rather than a preference.
		if strings.ContainsAny(topic, "\n\r`<>[]{}") {
			continue
		}
		out.AvoidTopics = append(out.AvoidTopics, topic)
		if len(out.AvoidTopics) >= 10 {
			break
		}
	}
	return out
}

// IsZero reports whether no structured preference is set.
func (p MemoryPreference) IsZero() bool {
	return p.Language == "" && p.Verbosity == "" && p.Tone == "" &&
		p.Format == "" && p.CodeStyle == "" && len(p.AvoidTopics) == 0
}

// Describe renders the preference as a short, human-readable clause list.
func (p MemoryPreference) Describe() string {
	parts := make([]string, 0, 6)
	if p.Language != "" {
		parts = append(parts, "language="+p.Language)
	}
	if p.Verbosity != "" {
		parts = append(parts, "verbosity="+p.Verbosity)
	}
	if p.Tone != "" {
		parts = append(parts, "tone="+p.Tone)
	}
	if p.Format != "" {
		parts = append(parts, "format="+p.Format)
	}
	if p.CodeStyle != "" {
		parts = append(parts, "code="+p.CodeStyle)
	}
	if len(p.AvoidTopics) > 0 {
		parts = append(parts, "avoid="+strings.Join(p.AvoidTopics, "/"))
	}
	return strings.Join(parts, "; ")
}

// Merge overlays non-empty fields of other onto p and returns the result.
func (p MemoryPreference) Merge(other MemoryPreference) MemoryPreference {
	out := p
	if other.Language != "" {
		out.Language = other.Language
	}
	if other.Verbosity != "" {
		out.Verbosity = other.Verbosity
	}
	if other.Tone != "" {
		out.Tone = other.Tone
	}
	if other.Format != "" {
		out.Format = other.Format
	}
	if other.CodeStyle != "" {
		out.CodeStyle = other.CodeStyle
	}
	if len(other.AvoidTopics) > 0 {
		out.AvoidTopics = other.AvoidTopics
	}
	return out
}

// Value implements driver.Valuer.
func (p MemoryPreference) Value() (driver.Value, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner.
func (p *MemoryPreference) Scan(src any) error {
	raw, err := jsonScanBytes(src, "MemoryPreference")
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		*p = MemoryPreference{}
		return nil
	}
	return json.Unmarshal(raw, p)
}

func pickEnum(value string, allowed ...string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, a := range allowed {
		if a == value {
			return a
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Memory note (L0)
// ---------------------------------------------------------------------------

// Note lifecycle states.
const (
	// MemoryNoteStatusPending awaits review (or automatic consolidation).
	MemoryNoteStatusPending = "pending"
	// MemoryNoteStatusMerged has been folded into a page.
	MemoryNoteStatusMerged = "merged"
	// MemoryNoteStatusRejected was declined by the user.
	MemoryNoteStatusRejected = "rejected"
	// MemoryNoteStatusExpired passed its TTL before being merged.
	MemoryNoteStatusExpired = "expired"
)

// Note scope: how long the observation is expected to matter.
const (
	MemoryScopeSession   = "session"
	MemoryScopeProject   = "project"
	MemoryScopePermanent = "permanent"
)

// Where a note came from.
const (
	MemorySourcePipeline = "pipeline"
	MemorySourceAgent    = "agent"
	MemorySourceUser     = "user"
	MemorySourceImport   = "import"
)

// Sensitivity classification applied at extraction time.
const (
	MemorySensitivityNormal    = "normal"
	MemorySensitivitySensitive = "sensitive"
	MemorySensitivityBlocked   = "blocked"
)

// MemoryNote is a single extracted observation, kept append-only so every
// memory can be traced back to the exact messages that produced it.
type MemoryNote struct {
	ID       string `json:"id"        gorm:"type:varchar(36);primaryKey"`
	TenantID uint64 `json:"tenant_id" gorm:"index"`
	SpaceID  string `json:"space_id"  gorm:"type:varchar(36);index"`
	NoteType string `json:"note_type" gorm:"type:varchar(32)"`
	// Statement is a single declarative sentence.
	Statement   string  `json:"statement" gorm:"type:text"`
	Subject     string  `json:"subject"   gorm:"type:varchar(255)"`
	Scope       string  `json:"scope"     gorm:"type:varchar(16);default:'permanent'"`
	Confidence  float64 `json:"confidence" gorm:"type:real;default:0.5"`
	Sensitivity string  `json:"sensitivity" gorm:"type:varchar(16);default:'normal'"`
	Source      string  `json:"source"      gorm:"type:varchar(16);default:'pipeline'"`
	// OriginRole records which conversation role the statement came from.
	// Extraction only ever accepts "user": a memory must never be distilled
	// from a retrieved document or a tool result, or a poisoned document could
	// implant a durable instruction that survives every future session.
	OriginRole       string           `json:"origin_role" gorm:"type:varchar(16);default:'user'"`
	SessionID        string           `json:"session_id"  gorm:"type:varchar(36);index"`
	SourceMessageIDs MemoryStringList `json:"source_message_ids" gorm:"type:jsonb"`
	// AnchorCandidates are entity names the extractor guessed at; they are
	// resolved to real wiki slugs later by deterministic matching, never by
	// the model itself.
	AnchorCandidates MemoryStringList `json:"anchor_candidates" gorm:"type:jsonb"`
	NormalizedHash   string           `json:"normalized_hash"   gorm:"type:varchar(64);index"`
	Status           string           `json:"status"            gorm:"type:varchar(16);default:'pending'"`
	MergedPageID     string           `json:"merged_page_id"    gorm:"type:varchar(36)"`
	ExpiresAt        *time.Time       `json:"expires_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	DeletedAt        gorm.DeletedAt   `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName returns the table name for MemoryNote.
func (MemoryNote) TableName() string { return "memory_notes" }

var memoryHashNoise = regexp.MustCompile(`[\s\p{P}]+`)

// NormalizeStatement collapses whitespace and punctuation and lowercases the
// text, producing the key used for exact-duplicate detection before any model
// is asked to judge similarity.
func NormalizeStatement(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Trim again after collapsing: a trailing full stop becomes a trailing
	// space, and without this "I prefer concise answers." and "i prefer
	// concise answers" would hash differently and both be stored.
	return strings.TrimSpace(memoryHashNoise.ReplaceAllString(s, " "))
}

// ---------------------------------------------------------------------------
// Memory anchor (L2) — the bridge into the knowledge base
// ---------------------------------------------------------------------------

// Anchor target kinds.
const (
	MemoryAnchorTargetWikiPage  = "wiki_page"
	MemoryAnchorTargetKnowledge = "knowledge"
	MemoryAnchorTargetChunk     = "chunk"
)

// Anchor relations, ordered by how much they say about the user's grasp of the
// target. Weights live in settings so an operator can retune without a release.
const (
	// MemoryRelationMentioned — the memory merely names the target.
	MemoryRelationMentioned = "mentioned"
	// MemoryRelationAskedAbout — the user asked and the target was cited.
	// Recorded at retrieval time at zero cost; this is the main source of
	// illumination data.
	MemoryRelationAskedAbout = "asked_about"
	// MemoryRelationBookmarked — the user flagged the target as important.
	MemoryRelationBookmarked = "bookmarked"
	// MemoryRelationDisagreed — the user disputes the target's content.
	MemoryRelationDisagreed = "disagreed"
	// MemoryRelationLearned — the user confirmed they understood or adopted it.
	MemoryRelationLearned = "learned"
	// MemoryRelationCorrected — the user corrected the target's content.
	MemoryRelationCorrected = "corrected"
	// MemoryRelationOwns — the user owns this subject.
	MemoryRelationOwns = "owns"
)

// AllMemoryRelations lists every anchor relation.
func AllMemoryRelations() []string {
	return []string{
		MemoryRelationMentioned,
		MemoryRelationAskedAbout,
		MemoryRelationBookmarked,
		MemoryRelationDisagreed,
		MemoryRelationLearned,
		MemoryRelationCorrected,
		MemoryRelationOwns,
	}
}

// IsValidMemoryRelation reports whether r is a known relation.
func IsValidMemoryRelation(r string) bool {
	for _, known := range AllMemoryRelations() {
		if known == r {
			return true
		}
	}
	return false
}

// UserSettableMemoryRelations are the relations a user may assert directly from
// the UI. mentioned and asked_about are derived by the system.
func UserSettableMemoryRelations() []string {
	return []string{
		MemoryRelationBookmarked,
		MemoryRelationDisagreed,
		MemoryRelationLearned,
		MemoryRelationCorrected,
		MemoryRelationOwns,
	}
}

// MemoryAnchorEvidence records why an anchor exists.
type MemoryAnchorEvidence struct {
	MessageIDs []string `json:"message_ids,omitempty"`
	SessionIDs []string `json:"session_ids,omitempty"`
	Queries    []string `json:"queries,omitempty"`
}

// Value implements driver.Valuer.
func (e MemoryAnchorEvidence) Value() (driver.Value, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner.
func (e *MemoryAnchorEvidence) Scan(src any) error {
	raw, err := jsonScanBytes(src, "MemoryAnchorEvidence")
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		*e = MemoryAnchorEvidence{}
		return nil
	}
	return json.Unmarshal(raw, e)
}

// maxAnchorEvidenceItems bounds each evidence list so a heavily used anchor
// cannot grow without limit.
const maxAnchorEvidenceItems = 20

// AppendEvidence merges new evidence, de-duplicating and capping each list.
func (e *MemoryAnchorEvidence) AppendEvidence(other MemoryAnchorEvidence) {
	e.MessageIDs = appendCapped(e.MessageIDs, other.MessageIDs)
	e.SessionIDs = appendCapped(e.SessionIDs, other.SessionIDs)
	e.Queries = appendCapped(e.Queries, other.Queries)
}

func appendCapped(dst, src []string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, v := range dst {
		seen[v] = struct{}{}
	}
	for _, v := range src {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		dst = append(dst, v)
	}
	if len(dst) > maxAnchorEvidenceItems {
		dst = dst[len(dst)-maxAnchorEvidenceItems:]
	}
	return dst
}

// MemoryAnchor links one memory page to one knowledge-base target.
type MemoryAnchor struct {
	ID              string `json:"id"        gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64 `json:"tenant_id" gorm:"index"`
	SpaceID         string `json:"space_id"  gorm:"type:varchar(36);index"`
	MemoryPageID    string `json:"memory_page_id" gorm:"type:varchar(36);index"`
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	TargetKind      string `json:"target_kind" gorm:"type:varchar(24)"`
	// TargetRef is a wiki slug, knowledge id or chunk id depending on kind.
	TargetRef  string  `json:"target_ref" gorm:"type:varchar(512)"`
	Relation   string  `json:"relation"   gorm:"type:varchar(24)"`
	Strength   float64 `json:"strength"   gorm:"type:real;default:0"`
	HitCount   int     `json:"hit_count"  gorm:"default:0"`
	Confidence float64 `json:"confidence" gorm:"type:real;default:0.5"`
	Source     string  `json:"source"     gorm:"type:varchar(16);default:'pipeline'"`

	Evidence    MemoryAnchorEvidence `json:"evidence" gorm:"type:jsonb"`
	FirstSeenAt time.Time            `json:"first_seen_at"`
	LastSeenAt  time.Time            `json:"last_seen_at"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	DeletedAt   gorm.DeletedAt       `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName returns the table name for MemoryAnchor.
func (MemoryAnchor) TableName() string { return "memory_anchors" }

// ---------------------------------------------------------------------------
// Recall payload
// ---------------------------------------------------------------------------

// MemoryRecallItem is one memory selected for injection.
type MemoryRecallItem struct {
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Type       string    `json:"type"`
	Text       string    `json:"text"`
	Confidence float64   `json:"confidence"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Score is the internal ranking score; not shown to the user.
	Score float64 `json:"-"`
	// Resident marks items included because of their type rather than
	// because they matched the query.
	Resident bool `json:"resident"`
}

// MemoryRecallResult is what the recall stage hands to the rest of the pipeline.
type MemoryRecallResult struct {
	SpaceID string `json:"space_id"`
	// Preference is the merged, sanitized structured preference.
	Preference MemoryPreference `json:"preference"`
	// Items are the memories that fit inside the token budget.
	Items []MemoryRecallItem `json:"items"`
	// OpenQuestions are unresolved threads worth resurfacing.
	OpenQuestions []MemoryRecallItem `json:"open_questions"`
	// AnchorHints are knowledge-base targets the user's memories point at.
	// The rerank stage uses them to personalise ranking.
	AnchorHints []MemoryAnchorHint `json:"-"`
	// TokensUsed is the estimated size of the rendered block.
	TokensUsed int `json:"tokens_used"`
}

// IsEmpty reports whether there is nothing worth injecting.
func (r *MemoryRecallResult) IsEmpty() bool {
	return r == nil || (len(r.Items) == 0 && len(r.OpenQuestions) == 0 && r.Preference.IsZero())
}

// MemoryAnchorHint tells the rerank stage that a retrieval target is anchored
// to something the caller already cares about.
type MemoryAnchorHint struct {
	KnowledgeBaseID string
	TargetKind      string
	TargetRef       string
	Weight          float64
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	// ErrMemoryDisabled is returned when memory is switched off at some layer.
	ErrMemoryDisabled = errors.New("memory is disabled")
	// ErrMemorySpaceUnavailable is returned when the caller has no principal a
	// space can be derived from.
	ErrMemorySpaceUnavailable = errors.New("no memory space available for this principal")
)
