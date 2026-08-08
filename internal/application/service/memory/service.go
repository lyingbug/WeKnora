package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

var (
	// ErrMemoryNotEnabled is returned when a caller reaches a memory endpoint
	// while the feature is switched off for them at some layer.
	ErrMemoryNotEnabled = errors.New("memory is not enabled for this caller")
	// ErrNoPrincipal is returned when the request has no identity a private
	// store could belong to.
	ErrNoPrincipal = errors.New("no principal in context")
	// ErrForbidden is returned when a setting forbids the requested action.
	ErrForbidden = errors.New("action not permitted by memory settings")
	// ErrInvalidRequest is returned for a malformed request the caller can fix.
	ErrInvalidRequest = errors.New("invalid memory request")
)

// Service is the memory façade used by handlers, tools and the chat pipeline.
type Service struct {
	spaces   interfaces.MemorySpaceRepository
	pages    interfaces.MemoryPageRepository
	notes    interfaces.MemoryNoteRepository
	anchors  interfaces.MemoryAnchorRepository
	settings interfaces.MemorySettingsService
}

// NewService creates the memory service.
func NewService(
	spaces interfaces.MemorySpaceRepository,
	pages interfaces.MemoryPageRepository,
	notes interfaces.MemoryNoteRepository,
	anchors interfaces.MemoryAnchorRepository,
	settings interfaces.MemorySettingsService,
) *Service {
	return &Service{spaces: spaces, pages: pages, notes: notes, anchors: anchors, settings: settings}
}

// Compile-time check that the concrete service satisfies the interface.
var _ interfaces.MemoryService = (*Service)(nil)

// ---------------------------------------------------------------------------
// Request scope
// ---------------------------------------------------------------------------

// scope bundles everything derived once per request: who is asking, which
// workspace, what their effective settings are and which space they own.
// Resolving it in one place is what keeps every method below from having to
// re-derive identity, and makes it impossible to accidentally query one
// person's memory with another person's scope.
type scope struct {
	TenantID  uint64
	Principal types.Principal
	Settings  types.MemorySettings
	Space     *types.MemorySpace
}

func (s *Service) resolveScope(ctx context.Context, create bool) (*scope, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return nil, ErrNoPrincipal
	}
	principal, ok := types.PrincipalFromContext(ctx)
	if !ok || !principal.Valid() {
		return nil, ErrNoPrincipal
	}

	userID, _ := types.UserIDFromContext(ctx)
	resolution, err := s.settings.Resolve(ctx, types.MemorySettingsResolveOptions{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, err
	}
	settings := resolution.Settings
	if !settings.Enabled {
		return nil, ErrMemoryNotEnabled
	}
	if !settings.ChannelAllowed(channelForPrincipal(principal)) {
		return nil, ErrMemoryNotEnabled
	}

	space, err := s.lookupSpace(ctx, tenantID, principal, settings, create)
	if err != nil {
		return nil, err
	}
	if space == nil {
		return nil, ErrMemoryNotEnabled
	}

	// Space-level overrides are the narrowest layer, so they are folded in
	// only once the space is known.
	if len(space.Config) > 0 {
		refined, rerr := s.settings.Resolve(ctx, types.MemorySettingsResolveOptions{
			TenantID:   tenantID,
			UserID:     userID,
			SpaceID:    space.ID,
			SpacePatch: space.Config,
		})
		if rerr == nil {
			settings = refined.Settings
		}
	}

	return &scope{TenantID: tenantID, Principal: principal, Settings: settings, Space: space}, nil
}

func (s *Service) lookupSpace(
	ctx context.Context, tenantID uint64, principal types.Principal,
	settings types.MemorySettings, create bool,
) (*types.MemorySpace, error) {
	space, err := s.spaces.GetByOwner(ctx, tenantID, types.MemorySpaceScopeUser, principal)
	if err == nil {
		return space, nil
	}
	if !errors.Is(err, repository.ErrMemorySpaceNotFound) {
		return nil, err
	}
	if !create {
		return nil, nil
	}
	// Embed visitors carry a client-supplied id, so a persistent store for them
	// is opt-in: by default their memory lives and dies with the session.
	if principal.Type == types.PrincipalEmbedVisitor &&
		settings.EmbedVisitorSpace != types.MemoryEmbedSpacePersistent {
		return nil, nil
	}

	space = &types.MemorySpace{
		ID:                 uuid.New().String(),
		TenantID:           tenantID,
		ScopeType:          types.MemorySpaceScopeUser,
		OwnerPrincipalType: principal.Type,
		OwnerPrincipalID:   principal.ID,
		DisplayName:        defaultSpaceName(principal),
		Status:             types.MemorySpaceStatusActive,
		Config:             types.MemorySettingsPatch{},
	}
	if err := s.spaces.Create(ctx, space); err != nil {
		// Another request for the same principal won the race; re-read.
		existing, getErr := s.spaces.GetByOwner(ctx, tenantID, types.MemorySpaceScopeUser, principal)
		if getErr == nil {
			return existing, nil
		}
		return nil, err
	}
	logger.Infof(ctx, "memory: created space %s for principal %s", space.ID, principal.StorageID())
	return space, nil
}

func defaultSpaceName(principal types.Principal) string {
	return "Memory of " + principal.StorageID()
}

// channelForPrincipal maps a principal onto the channel allow-list.
func channelForPrincipal(principal types.Principal) string {
	switch principal.Type {
	case types.PrincipalWebUser:
		return types.MemoryChannelWeb
	case types.PrincipalIMUser:
		return types.MemoryChannelIM
	case types.PrincipalEmbedChannel, types.PrincipalEmbedSession, types.PrincipalEmbedVisitor:
		return types.MemoryChannelEmbed
	default:
		return types.MemoryChannelAPI
	}
}

// ---------------------------------------------------------------------------
// Space
// ---------------------------------------------------------------------------

// EnsureSpace resolves, and lazily creates, the caller's space.
func (s *Service) EnsureSpace(ctx context.Context) (*types.MemorySpace, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		if errors.Is(err, ErrMemoryNotEnabled) || errors.Is(err, ErrNoPrincipal) {
			return nil, nil
		}
		return nil, err
	}
	return sc.Space, nil
}

// GetSpace returns the caller's space without creating one.
func (s *Service) GetSpace(ctx context.Context) (*types.MemorySpace, error) {
	sc, err := s.resolveScope(ctx, false)
	if err != nil {
		return nil, err
	}
	return sc.Space, nil
}

// SpaceView returns the space with the statistics and capability flags the
// memory centre header needs.
func (s *Service) SpaceView(ctx context.Context) (*types.MemorySpaceView, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	stats, err := s.statsFor(ctx, sc)
	if err != nil {
		return nil, err
	}
	return &types.MemorySpaceView{
		Space:        sc.Space,
		Stats:        *stats,
		Capabilities: s.settings.Capabilities(ctx, sc.Settings),
	}, nil
}

// ---------------------------------------------------------------------------
// Pages
// ---------------------------------------------------------------------------

// ListPages returns a page of memories.
func (s *Service) ListPages(
	ctx context.Context, req *types.MemoryPageListRequest,
) (*types.MemoryPageListResponse, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	req.SpaceID = sc.Space.ID
	pages, total, err := s.pages.List(ctx, req)
	if err != nil {
		return nil, err
	}
	return &types.MemoryPageListResponse{
		Pages: pages, Total: total, Page: req.Page, PageSize: req.PageSize,
	}, nil
}

// GetPage returns one memory by slug.
func (s *Service) GetPage(ctx context.Context, slug string) (*types.MemoryPage, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	return s.pages.GetBySlug(ctx, sc.Space.ID, slug)
}

// WritePage creates or updates a memory.
//
// Create and update share one entry point because from the user's point of view
// they are the same act — "make my memory say this" — and splitting them would
// duplicate the slug, link and revision handling that has to be identical
// either way.
func (s *Service) WritePage(
	ctx context.Context, req *types.MemoryPageWriteRequest,
) (*types.MemoryPage, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	if !sc.Settings.WritesAllowed() {
		return nil, ErrForbidden
	}
	return s.writePageInScope(ctx, sc, req)
}

// screenMemoryText applies the write-path content rules to a page request.
//
// A memory is a durable fact about the user that gets injected into future
// prompts, so an instruction and a credential are both refused outright, and
// direct identifiers are handled according to the workspace's PII policy.
func screenMemoryText(settings types.MemorySettings, req *types.MemoryPageWriteRequest) error {
	for _, text := range []string{req.Content, req.Summary} {
		if strings.TrimSpace(text) == "" {
			continue
		}
		if LooksLikeInstruction(text) {
			return fmt.Errorf("%w: a memory records a fact about you, not an instruction", ErrForbidden)
		}
		if MatchesBlockedPattern(text, settings.BlockedPatterns) {
			return fmt.Errorf("%w: this looks like a credential and was not stored", ErrForbidden)
		}
		if settings.PIIRedaction == types.MemoryPIIBlock && ContainsPII(text) {
			return fmt.Errorf("%w: this contains personal identifiers and was not stored", ErrForbidden)
		}
	}
	if settings.PIIRedaction == types.MemoryPIIRedact {
		req.Content = RedactPII(req.Content)
		req.Summary = RedactPII(req.Summary)
	}
	return nil
}

func (s *Service) writePageInScope(
	ctx context.Context, sc *scope, req *types.MemoryPageWriteRequest,
) (*types.MemoryPage, error) {
	pageType := strings.TrimSpace(req.PageType)
	if pageType == "" {
		pageType = types.MemoryTypeEpisode
	}
	if !types.IsValidMemoryType(pageType) {
		return nil, fmt.Errorf("%w: unknown memory type %q", ErrInvalidRequest, pageType)
	}
	if !sc.Settings.TypeAllowed(pageType) {
		return nil, fmt.Errorf("%w: memories of type %q are disabled", ErrForbidden, pageType)
	}

	// Screening belongs here because this is the only path every write shares.
	// It previously lived in the two callers that happened to think of it, so an
	// agent calling memory_remember, or anyone using the memory editor, could
	// store an instruction that is then prepended to every later turn, and could
	// store a credential verbatim regardless of the deny-pattern and PII
	// settings. Consolidation and explicit remember screen their input too; the
	// checks are idempotent, and the duplication is cheaper than a bypass.
	if err := screenMemoryText(sc.Settings, req); err != nil {
		return nil, err
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = DeriveMemoryTitle(firstNonEmpty(req.Summary, req.Content))
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = BuildMemorySlug(pageType, title)
	}

	status := req.Status
	if status == "" {
		status = types.MemoryPageStatusActive
	}
	if !types.IsValidMemoryPageStatus(status) {
		return nil, fmt.Errorf("%w: unknown memory status %q", ErrInvalidRequest, status)
	}

	editSource := req.EditSource
	if editSource == "" {
		editSource = types.MemoryEditSourceUser
	}

	existing, err := s.pages.GetBySlug(ctx, sc.Space.ID, slug)
	switch {
	case err == nil:
		return s.updateExistingPage(ctx, sc, existing, req, title, pageType, status, editSource)
	case errors.Is(err, repository.ErrMemoryPageNotFound):
		return s.createPage(ctx, sc, req, slug, title, pageType, status, editSource)
	default:
		return nil, err
	}
}

func (s *Service) createPage(
	ctx context.Context, sc *scope, req *types.MemoryPageWriteRequest,
	slug, title, pageType, status, editSource string,
) (*types.MemoryPage, error) {
	count, err := s.pages.Count(ctx, sc.Space.ID, []string{types.MemoryPageStatusActive})
	if err == nil && sc.Settings.MaxPagesPerSpace > 0 && count >= int64(sc.Settings.MaxPagesPerSpace) {
		return nil, fmt.Errorf("%w: this memory space is full (%d pages)", ErrForbidden, count)
	}

	page := &types.MemoryPage{
		ID:             uuid.New().String(),
		TenantID:       sc.TenantID,
		SpaceID:        sc.Space.ID,
		Slug:           slug,
		Title:          title,
		PageType:       pageType,
		MemoryKey:      strings.TrimSpace(req.MemoryKey),
		Status:         status,
		Content:        req.Content,
		Summary:        firstNonEmpty(strings.TrimSpace(req.Summary), DeriveMemoryTitle(req.Content)),
		Aliases:        req.Aliases,
		FolderPath:     req.FolderPath,
		OutLinks:       s.resolveLinkTargets(ctx, sc.Space.ID, req.Content),
		Strength:       1,
		Confidence:     0.9,
		LastEditSource: editSource,
	}
	// API/user/agent writes are saved memories. Background consolidation opts
	// out explicitly and becomes referenceable chat history instead.
	page.Saved = editSource != types.MemoryEditSourcePipeline
	if req.Saved != nil {
		page.Saved = *req.Saved
	}
	if req.Structured != nil {
		page.Structured = req.Structured.Sanitize()
	}
	if req.Pinned != nil {
		page.Pinned = *req.Pinned
	}
	if req.Confidence != nil {
		page.Confidence = clampUnit(*req.Confidence)
	}
	now := time.Now()
	page.LastSeenAt = &now

	if err := s.pages.Create(ctx, page); err != nil {
		return nil, err
	}
	s.syncInboundLinks(ctx, sc.Space.ID, page.Slug, nil, page.OutLinks)
	return page, nil
}

func (s *Service) updateExistingPage(
	ctx context.Context, sc *scope, page *types.MemoryPage, req *types.MemoryPageWriteRequest,
	title, pageType, status, editSource string,
) (*types.MemoryPage, error) {
	revision := snapshotPage(page, editSource)
	previousLinks := append([]string(nil), page.OutLinks...)

	page.Title = title
	page.PageType = pageType
	if req.MemoryKey != "" {
		page.MemoryKey = strings.TrimSpace(req.MemoryKey)
	}
	if req.Saved != nil {
		page.Saved = *req.Saved
	}
	page.Status = status
	page.Content = req.Content
	page.Summary = firstNonEmpty(strings.TrimSpace(req.Summary), page.Summary)
	page.OutLinks = s.resolveLinkTargets(ctx, sc.Space.ID, req.Content)
	page.LastEditSource = editSource
	if req.Aliases != nil {
		page.Aliases = req.Aliases
	}
	if req.FolderPath != nil {
		page.FolderPath = req.FolderPath
	}
	if req.Structured != nil {
		page.Structured = req.Structured.Sanitize()
	}
	if req.Pinned != nil {
		page.Pinned = *req.Pinned
	}
	if req.Confidence != nil {
		page.Confidence = clampUnit(*req.Confidence)
	}
	// A human touching a memory is itself a signal that it still matters, so an
	// edit restores full strength rather than leaving it mid-decay.
	if editSource == types.MemoryEditSourceUser {
		page.Strength = 1
		now := time.Now()
		page.LastSeenAt = &now
	}

	if err := s.pages.UpdateWithRevision(ctx, page, revision, req.Version); err != nil {
		return nil, err
	}
	s.syncInboundLinks(ctx, sc.Space.ID, page.Slug, previousLinks, page.OutLinks)
	return page, nil
}

// resolveLinkTargets turns the raw text inside [[...]] into canonical slugs.
//
// People write the title they see — [[检索召回率]] — not the addressable slug
// — [[project/检索召回率]] — and a link that silently fails to resolve is worse
// than no link, because the graph quietly loses an edge the user believes they
// drew. Resolution happens once at write time so both the backlink bookkeeping
// and the graph read plain slugs, and an unresolvable target is preserved
// verbatim rather than dropped: the user may be linking to something they are
// about to write.
func (s *Service) resolveLinkTargets(ctx context.Context, spaceID, content string) types.MemoryStringList {
	raw := ParseMemoryLinks(content)
	if len(raw) == 0 {
		return nil
	}

	direct, err := s.pages.GetBySlugs(ctx, spaceID, raw)
	if err != nil {
		return raw
	}
	known := make(map[string]struct{}, len(direct))
	for _, page := range direct {
		known[page.Slug] = struct{}{}
	}

	unresolved := make([]string, 0, len(raw))
	for _, target := range raw {
		if _, ok := known[target]; !ok {
			unresolved = append(unresolved, target)
		}
	}
	if len(unresolved) == 0 {
		return raw
	}

	byTitle := map[string]string{}
	if all, err := s.pages.ListAll(ctx, spaceID); err == nil {
		for _, page := range all {
			byTitle[strings.ToLower(strings.TrimSpace(page.Title))] = page.Slug
			for _, alias := range page.Aliases {
				byTitle[strings.ToLower(strings.TrimSpace(alias))] = page.Slug
			}
		}
	}

	out := make(types.MemoryStringList, 0, len(raw))
	seen := map[string]struct{}{}
	for _, target := range raw {
		resolved := target
		if _, ok := known[target]; !ok {
			if slug, found := byTitle[strings.ToLower(strings.TrimSpace(target))]; found {
				resolved = slug
			}
		}
		if _, dup := seen[resolved]; dup {
			continue
		}
		seen[resolved] = struct{}{}
		out = append(out, resolved)
	}
	return out
}

// syncInboundLinks keeps the reverse edges consistent.
//
// Backlinks are stored denormalised on each row (exactly as the knowledge-base
// wiki does) so rendering the graph needs one query rather than a join per
// node. The cost is this bookkeeping on write, which is the cheaper side of the
// trade for a store that is read far more often than written.
func (s *Service) syncInboundLinks(ctx context.Context, spaceID, slug string, before, after []string) {
	removed := differenceStrings(before, after)
	added := differenceStrings(after, before)
	if len(removed) == 0 && len(added) == 0 {
		return
	}

	apply := func(slugs []string, add bool) {
		if len(slugs) == 0 {
			return
		}
		targets, err := s.pages.GetBySlugs(ctx, spaceID, slugs)
		if err != nil {
			logger.Warnf(ctx, "memory: backlink sync lookup failed: %v", err)
			return
		}
		for _, target := range targets {
			changed := false
			if add {
				changed = target.InLinks.Add(slug)
			} else {
				changed = target.InLinks.Remove(slug)
			}
			if !changed {
				continue
			}
			if err := s.pages.UpdateLinks(ctx, target); err != nil {
				logger.Warnf(ctx, "memory: backlink sync write failed for %s: %v", target.Slug, err)
			}
		}
	}
	apply(removed, false)
	apply(added, true)
}

// DeletePage removes a memory and everything that hangs off it.
func (s *Service) DeletePage(ctx context.Context, slug string) error {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return err
	}
	if !sc.Settings.ForgetEnabled {
		return fmt.Errorf("%w: deleting memories is disabled", ErrForbidden)
	}
	page, err := s.pages.GetBySlug(ctx, sc.Space.ID, slug)
	if err != nil {
		return err
	}
	if err := s.pages.Delete(ctx, sc.Space.ID, page.ID); err != nil {
		return err
	}
	s.syncInboundLinks(ctx, sc.Space.ID, page.Slug, page.OutLinks, nil)
	// The anchors only meant anything as statements about this memory, so they
	// go with it; leaving them would keep lighting up wiki pages on behalf of
	// something the user just deleted.
	if _, err := s.anchors.DeleteByPage(ctx, sc.Space.ID, page.ID); err != nil {
		logger.Warnf(ctx, "memory: failed to delete anchors for %s: %v", page.Slug, err)
	}
	return nil
}

// SearchPages runs a keyword search across the caller's memories.
func (s *Service) SearchPages(
	ctx context.Context, query string, limit int,
) ([]*types.MemoryPage, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	return s.pages.Search(ctx, sc.Space.ID, query, limit)
}

// ListRevisions returns the edit history of a memory.
func (s *Service) ListRevisions(
	ctx context.Context, slug string,
) ([]*types.MemoryPageRevision, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	page, err := s.pages.GetBySlug(ctx, sc.Space.ID, slug)
	if err != nil {
		return nil, err
	}
	return s.pages.ListRevisions(ctx, sc.Space.ID, page.ID)
}

// RevertPage restores an earlier revision.
func (s *Service) RevertPage(
	ctx context.Context, req *types.MemoryRevertRequest,
) (*types.MemoryPage, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	page, err := s.pages.GetBySlug(ctx, sc.Space.ID, req.Slug)
	if err != nil {
		return nil, err
	}
	revision, err := s.pages.GetRevision(ctx, sc.Space.ID, page.ID, req.Version)
	if err != nil {
		return nil, err
	}

	// Reverting is itself an edit: the version being replaced is snapshotted
	// first, so a revert can always be reverted.
	snapshot := snapshotPage(page, types.MemoryEditSourceRevert)
	previousLinks := append([]string(nil), page.OutLinks...)

	page.Title = revision.Title
	page.Content = revision.Content
	page.Summary = revision.Summary
	page.Structured = revision.Structured
	page.OutLinks = s.resolveLinkTargets(ctx, sc.Space.ID, revision.Content)
	page.LastEditSource = types.MemoryEditSourceRevert

	// Guard the write with a version. The page's own version closes the
	// read-modify-write race inside this function; a caller that tells us what it
	// last saw additionally gets protection from having reverted over an edit it
	// never knew about. Passing 0 disabled both.
	expected := page.Version
	if req.ExpectedVersion > 0 {
		expected = req.ExpectedVersion
	}
	if err := s.pages.UpdateWithRevision(ctx, page, snapshot, expected); err != nil {
		return nil, err
	}
	s.syncInboundLinks(ctx, sc.Space.ID, page.Slug, previousLinks, page.OutLinks)
	return page, nil
}

func snapshotPage(page *types.MemoryPage, editSource string) *types.MemoryPageRevision {
	return &types.MemoryPageRevision{
		ID:         uuid.New().String(),
		TenantID:   page.TenantID,
		SpaceID:    page.SpaceID,
		PageID:     page.ID,
		Version:    page.Version,
		Title:      page.Title,
		Content:    page.Content,
		Summary:    page.Summary,
		Structured: page.Structured,
		EditSource: editSource,
		EditedAt:   time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Notes
// ---------------------------------------------------------------------------

// ListNotes returns the observation inbox.
func (s *Service) ListNotes(
	ctx context.Context, req *types.MemoryNoteListRequest,
) (*types.MemoryNoteListResponse, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	req.SpaceID = sc.Space.ID
	notes, total, err := s.notes.List(ctx, req)
	if err != nil {
		return nil, err
	}
	return &types.MemoryNoteListResponse{
		Notes: notes, Total: total, Page: req.Page, PageSize: req.PageSize,
	}, nil
}

// PromoteNote turns a pending observation into a memory, applying any edits the
// user made while reviewing it.
func (s *Service) PromoteNote(
	ctx context.Context, noteID string, req *types.MemoryNotePromoteRequest,
) (*types.MemoryPage, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	note, err := s.notes.GetByID(ctx, sc.Space.ID, noteID)
	if err != nil {
		return nil, err
	}

	statement := note.Statement
	noteType := note.NoteType
	if req != nil {
		if v := strings.TrimSpace(req.Statement); v != "" {
			statement = v
		}
		if v := strings.TrimSpace(req.NoteType); v != "" && types.IsValidMemoryType(v) {
			noteType = v
		}
	}
	title := DeriveMemoryTitle(statement)
	slug := ""
	if req != nil {
		title = firstNonEmpty(strings.TrimSpace(req.Title), title)
		slug = strings.TrimSpace(req.Slug)
	}

	page, err := s.writePageInScope(ctx, sc, &types.MemoryPageWriteRequest{
		Slug:       slug,
		Title:      title,
		PageType:   noteType,
		Saved:      boolPtr(true),
		MemoryKey:  note.MemoryKey,
		Content:    statement,
		Summary:    statement,
		Structured: preferencePtr(note.Structured),
		EditSource: types.MemoryEditSourceUser,
	})
	if err != nil {
		return nil, err
	}

	page.NoteRefs.Add(note.ID)
	if err := s.pages.Update(ctx, page, 0); err != nil {
		logger.Warnf(ctx, "memory: failed to record note ref on %s: %v", page.Slug, err)
	}
	if err := s.notes.UpdateStatus(ctx, sc.Space.ID, note.ID, types.MemoryNoteStatusMerged, page.ID); err != nil {
		return nil, err
	}
	return page, nil
}

// RejectNote declines a pending observation.
func (s *Service) RejectNote(ctx context.Context, noteID string) error {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return err
	}
	return s.notes.UpdateStatus(ctx, sc.Space.ID, noteID, types.MemoryNoteStatusRejected, "")
}

// ---------------------------------------------------------------------------
// Graph
// ---------------------------------------------------------------------------

// Graph builds the memory graph slice the UI asked for.
func (s *Service) Graph(
	ctx context.Context, req *types.MemoryGraphRequest,
) (*types.MemoryGraphData, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	req.Normalize()

	pages, err := s.pages.ListAll(ctx, sc.Space.ID)
	if err != nil {
		return nil, err
	}
	var anchors []*types.MemoryAnchor
	if req.Mode == types.MemoryGraphModeBridged {
		if anchors, err = s.anchors.ListBySpace(ctx, sc.Space.ID, ""); err != nil {
			return nil, err
		}
	}
	return BuildMemoryGraph(pages, anchors, req), nil
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

// Stats summarises the caller's space.
func (s *Service) Stats(ctx context.Context) (*types.MemoryStats, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	return s.statsFor(ctx, sc)
}

func (s *Service) statsFor(ctx context.Context, sc *scope) (*types.MemoryStats, error) {
	stats := &types.MemoryStats{ByType: map[string]int64{}}

	total, err := s.pages.Count(ctx, sc.Space.ID, nil)
	if err != nil {
		return nil, err
	}
	stats.TotalPages = total

	if stats.ActivePages, err = s.pages.Count(
		ctx, sc.Space.ID, []string{types.MemoryPageStatusActive},
	); err != nil {
		return nil, err
	}
	if stats.ArchivedPages, err = s.pages.Count(
		ctx, sc.Space.ID, []string{types.MemoryPageStatusArchived},
	); err != nil {
		return nil, err
	}
	if stats.PendingNotes, err = s.notes.Count(
		ctx, sc.Space.ID, []string{types.MemoryNoteStatusPending},
	); err != nil {
		return nil, err
	}
	if stats.TotalAnchors, err = s.anchors.Count(ctx, sc.Space.ID); err != nil {
		return nil, err
	}
	if stats.ByType, err = s.pages.CountByType(ctx, sc.Space.ID); err != nil {
		return nil, err
	}
	if kbs, err := s.anchors.ListAnchoredKBs(ctx, sc.Space.ID); err == nil {
		stats.AnchoredKBs = kbs
	}
	return stats, nil
}

// ---------------------------------------------------------------------------
// Anchors
// ---------------------------------------------------------------------------

// ListAnchors returns the caller's anchors, optionally for one knowledge base,
// joined with the memory page each belongs to.
func (s *Service) ListAnchors(ctx context.Context, kbID string) ([]*types.MemoryAnchorView, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	anchors, err := s.anchors.ListBySpace(ctx, sc.Space.ID, kbID)
	if err != nil {
		return nil, err
	}
	return s.decorateAnchors(ctx, sc.Space.ID, anchors), nil
}

func (s *Service) decorateAnchors(
	ctx context.Context, spaceID string, anchors []*types.MemoryAnchor,
) []*types.MemoryAnchorView {
	pageIDs := make([]string, 0, len(anchors))
	seen := map[string]struct{}{}
	for _, a := range anchors {
		if a.MemoryPageID == "" {
			continue
		}
		if _, ok := seen[a.MemoryPageID]; ok {
			continue
		}
		seen[a.MemoryPageID] = struct{}{}
		pageIDs = append(pageIDs, a.MemoryPageID)
	}

	byID := make(map[string]*types.MemoryPage, len(pageIDs))
	for _, id := range pageIDs {
		if page, err := s.pages.GetByID(ctx, spaceID, id); err == nil {
			byID[id] = page
		}
	}

	out := make([]*types.MemoryAnchorView, 0, len(anchors))
	for _, a := range anchors {
		view := &types.MemoryAnchorView{MemoryAnchor: a}
		if page, ok := byID[a.MemoryPageID]; ok {
			view.MemoryPageSlug = page.Slug
			view.MemoryPageTitle = page.Title
		}
		out = append(out, view)
	}
	return out
}

// AddAnchor records a user-asserted relationship to a knowledge-base target.
func (s *Service) AddAnchor(
	ctx context.Context, req *types.MemoryAnchorRequest,
) (*types.MemoryAnchor, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	if !containsStr(types.UserSettableMemoryRelations(), req.Relation) {
		return nil, fmt.Errorf(
			"%w: relation %q is derived from usage and cannot be asserted directly",
			ErrInvalidRequest, req.Relation)
	}
	if strings.TrimSpace(req.KnowledgeBaseID) == "" {
		return nil, fmt.Errorf("%w: an anchor needs a knowledge base", ErrInvalidRequest)
	}
	if strings.TrimSpace(req.TargetRef) == "" {
		return nil, fmt.Errorf("%w: an anchor needs a target", ErrInvalidRequest)
	}
	targetKind := req.TargetKind
	if targetKind == "" {
		targetKind = types.MemoryAnchorTargetWikiPage
	}

	pageID := ""
	if req.MemoryPageSlug != "" {
		page, err := s.pages.GetBySlug(ctx, sc.Space.ID, req.MemoryPageSlug)
		if err != nil {
			return nil, err
		}
		pageID = page.ID
	}

	if err := s.anchors.Upsert(ctx, &types.MemoryAnchorUpsert{
		SpaceID:         sc.Space.ID,
		TenantID:        sc.TenantID,
		MemoryPageID:    pageID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		TargetKind:      targetKind,
		TargetRef:       req.TargetRef,
		Relation:        req.Relation,
		Source:          types.MemorySourceUser,
		Confidence:      1,
	}); err != nil {
		return nil, err
	}

	anchors, err := s.anchors.ListByTarget(
		ctx, sc.Space.ID, req.KnowledgeBaseID, targetKind, req.TargetRef,
	)
	if err != nil {
		return nil, err
	}
	for _, a := range anchors {
		if a.Relation == req.Relation && a.MemoryPageID == pageID {
			return a, nil
		}
	}
	// The upsert succeeded, so the row exists; not finding it means the read-back
	// disagrees with the write. Returning (nil, nil) here answered 200 with a
	// null body, which reads as success and gives the caller nothing.
	return nil, fmt.Errorf("anchor was stored but could not be read back")
}

// DeleteAnchor removes one anchor.
func (s *Service) DeleteAnchor(ctx context.Context, anchorID string) error {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return err
	}
	return s.anchors.Delete(ctx, sc.Space.ID, anchorID)
}

// ---------------------------------------------------------------------------
// Forget and export
// ---------------------------------------------------------------------------

// Forget deletes memories in bulk.
func (s *Service) Forget(
	ctx context.Context, req *types.MemoryForgetRequest,
) (*types.MemoryForgetResponse, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	if !sc.Settings.ForgetEnabled {
		return nil, fmt.Errorf("%w: forgetting is disabled", ErrForbidden)
	}
	resp := &types.MemoryForgetResponse{}

	switch req.Scope {
	case "all":
		if resp.PagesDeleted, err = s.pages.DeleteAll(ctx, sc.Space.ID); err != nil {
			return nil, err
		}
		if resp.AnchorsDeleted, err = s.anchors.DeleteAll(ctx, sc.Space.ID); err != nil {
			return nil, err
		}
		// "Forget everything" has to mean the evidence too, or the next
		// extraction would happily rebuild what was just deleted.
		if resp.NotesDeleted, err = s.notes.DeleteAll(ctx, sc.Space.ID); err != nil {
			return nil, err
		}

	case "type":
		slugs, err := s.slugsForTypes(ctx, sc.Space.ID, req.Types)
		if err != nil {
			return nil, err
		}
		if resp, err = s.forgetSlugs(ctx, sc, slugs, req.PurgeNotes); err != nil {
			return nil, err
		}

	case "slugs":
		if resp, err = s.forgetSlugs(ctx, sc, req.Slugs, req.PurgeNotes); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("%w: unknown forget scope %q", ErrInvalidRequest, req.Scope)
	}

	return resp, nil
}

func (s *Service) slugsForTypes(
	ctx context.Context, spaceID string, pageTypes []string,
) ([]string, error) {
	pages, err := s.pages.ListByTypes(ctx, spaceID, pageTypes, nil, 0)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(pages))
	for _, p := range pages {
		slugs = append(slugs, p.Slug)
	}
	return slugs, nil
}

func (s *Service) forgetSlugs(
	ctx context.Context, sc *scope, slugs []string, purgeNotes bool,
) (*types.MemoryForgetResponse, error) {
	resp := &types.MemoryForgetResponse{}
	pages, err := s.pages.GetBySlugs(ctx, sc.Space.ID, slugs)
	if err != nil {
		return nil, err
	}
	for _, page := range pages {
		if n, err := s.anchors.DeleteByPage(ctx, sc.Space.ID, page.ID); err == nil {
			resp.AnchorsDeleted += n
		}
		if purgeNotes {
			if n, err := s.notes.DeleteByPage(ctx, sc.Space.ID, page.ID); err == nil {
				resp.NotesDeleted += n
			}
		}
		s.syncInboundLinks(ctx, sc.Space.ID, page.Slug, page.OutLinks, nil)
	}
	if resp.PagesDeleted, err = s.pages.DeleteBySlugs(ctx, sc.Space.ID, slugs); err != nil {
		return nil, err
	}
	return resp, nil
}

// Export dumps the whole space for portability.
func (s *Service) Export(ctx context.Context) (*types.MemoryExport, error) {
	sc, err := s.resolveScope(ctx, true)
	if err != nil {
		return nil, err
	}
	if !sc.Settings.ExportEnabled {
		return nil, fmt.Errorf("%w: exporting is disabled", ErrForbidden)
	}
	export := &types.MemoryExport{ExportedAt: time.Now(), Space: sc.Space, Settings: sc.Space.Config}
	if export.Pages, err = s.pages.ListAll(ctx, sc.Space.ID); err != nil {
		return nil, err
	}
	if export.Notes, err = s.notes.ListAll(ctx, sc.Space.ID); err != nil {
		return nil, err
	}
	if export.Anchors, err = s.anchors.ListAll(ctx, sc.Space.ID); err != nil {
		return nil, err
	}
	return export, nil
}

// ---------------------------------------------------------------------------
// Illumination
// ---------------------------------------------------------------------------

// Overlay returns per-target illumination for the caller.
//
// targetKind selects what is being lit: wiki pages, or the documents of an
// ordinary knowledge base. The maths is the same either way — the anchors only
// differ in what their target_ref points at — so an ordinary knowledge base gets
// the same four states and the same decay as a wiki.
//
// Returns nil (not an error) when memory is off or the overlay is disabled, so
// the wiki graph endpoint can call it unconditionally and simply omit the
// overlay field when there is nothing to add.
func (s *Service) Overlay(
	ctx context.Context, kbID, targetKind string,
) (map[string]types.MemoryOverlayNode, error) {
	sc, err := s.resolveScope(ctx, false)
	if err != nil || sc == nil || sc.Space == nil {
		return nil, nil
	}
	if !sc.Settings.OverlayEnabled {
		return nil, nil
	}
	if targetKind == "" {
		targetKind = types.MemoryAnchorTargetWikiPage
	}
	anchors, err := s.anchors.ListOverlay(ctx, sc.Space.ID, kbID, targetKind)
	if err != nil {
		return nil, err
	}
	if len(anchors) == 0 {
		return map[string]types.MemoryOverlayNode{}, nil
	}
	return types.ComputeMemoryOverlay(anchors, types.MemoryOverlayOptionsFrom(sc.Settings, time.Now())), nil
}

// Coverage reports how much of a knowledge base the caller has lit up.
func (s *Service) Coverage(
	ctx context.Context, kbID string, pages []types.MemoryCoveragePage, targetKind string,
) (*types.MemoryCoverage, error) {
	overlay, err := s.Overlay(ctx, kbID, targetKind)
	if err != nil {
		return nil, err
	}
	if overlay == nil {
		overlay = map[string]types.MemoryOverlayNode{}
	}
	coverage := types.ComputeMemoryCoverage(kbID, pages, overlay)
	return &coverage, nil
}

// Insights returns the anonymised aggregate view for workspace administrators.
//
// The k-anonymity gate is applied here rather than in the UI: an insight is
// only shown once enough distinct people contributed to it that it says
// something about the knowledge base instead of about a person.
func (s *Service) Insights(
	ctx context.Context, kbID string, pages []types.MemoryInsightPage, targetKind string,
) (*types.MemoryInsightsResponse, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return nil, ErrNoPrincipal
	}
	userID, _ := types.UserIDFromContext(ctx)
	resolution, err := s.settings.Resolve(ctx, types.MemorySettingsResolveOptions{
		TenantID: tenantID, UserID: userID,
	})
	if err != nil {
		return nil, err
	}
	settings := resolution.Settings
	if !settings.Enabled || !settings.InsightsEnabled {
		return nil, fmt.Errorf("%w: insights are disabled", ErrForbidden)
	}

	aggregates, err := s.anchors.AggregateByTarget(ctx, tenantID, kbID)
	if err != nil {
		return nil, err
	}
	resp := BuildMemoryInsights(kbID, aggregates, pages, settings.InsightsKAnonymity, targetKind)
	return resp, nil
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func containsStr(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func differenceStrings(a, b []string) []string {
	if len(a) == 0 {
		return nil
	}
	inB := make(map[string]struct{}, len(b))
	for _, v := range b {
		inB[v] = struct{}{}
	}
	out := make([]string, 0, len(a))
	for _, v := range a {
		if _, ok := inB[v]; !ok {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
