package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// A background task runs without the request that queued it, and everything the
// request had on its context has to be put back deliberately. Two things were
// not, and each failed silently in its own way:
//
//   - Settings were resolved without the user layer, so a write mode chosen in
//     personal settings was invisible. The task decided extraction was not
//     allowed and reported success having written nothing.
//   - The workspace id was absent, so the first tenant-scoped query failed with
//     "workspace id not found" the moment the task got past that gate.
//
// Both are pinned here at the writer's entry point, which is where the
// restoration has to happen: the decay sweep arrives from a ticker and never
// passes through the task handler at all.

// settingsSpy records what it was asked to resolve, and mimics a setting that
// only exists on the user layer.
type settingsSpy struct {
	interfaces.MemorySettingsService
	lastOpts types.MemorySettingsResolveOptions
	// userLayerMode is returned only when a user id is supplied, standing in for
	// a patch stored on the user record.
	userLayerMode string
}

func (s *settingsSpy) Resolve(
	_ context.Context, opts types.MemorySettingsResolveOptions,
) (types.MemorySettingsResolution, error) {
	s.lastOpts = opts
	settings := types.ResolveMemorySettings().Settings
	settings.Enabled = true
	if opts.UserID != "" && s.userLayerMode != "" {
		settings.WriteMode = s.userLayerMode
	}
	return types.MemorySettingsResolution{Settings: settings}, nil
}

// messagesSpy stands in for the message service, which reads the workspace id
// from the context exactly as the real repository does.
type messagesSpy struct {
	interfaces.MessageService
	called bool
}

var errNoWorkspace = errors.New("workspace id not found in context")

func (m *messagesSpy) GetRecentMessagesBySession(
	ctx context.Context, _ string, _ int,
) ([]*types.Message, error) {
	m.called = true
	if _, ok := types.TenantIDFromContext(ctx); !ok {
		return nil, errNoWorkspace
	}
	return nil, nil
}

func newBackgroundFixture(userLayerMode string) (*writerService, *settingsSpy, *messagesSpy, *types.MemorySpace) {
	settings := &settingsSpy{userLayerMode: userLayerMode}
	messages := &messagesSpy{}
	space := &types.MemorySpace{
		ID:                 "space-1",
		TenantID:           7,
		ScopeType:          types.MemorySpaceScopeUser,
		OwnerPrincipalType: types.PrincipalWebUser,
		OwnerPrincipalID:   "u-wizard",
		Status:             types.MemorySpaceStatusActive,
	}
	writer := &writerService{
		spaces:   &spaceStub{space: space},
		pages:    emptyPages{},
		notes:    emptyNotes{},
		messages: messages,
		settings: settings,
	}
	return writer, settings, messages, space
}

// spaceStub returns one space regardless of what is asked for.
type spaceStub struct {
	interfaces.MemorySpaceRepository
	space *types.MemorySpace
}

func (s *spaceStub) GetByID(context.Context, uint64, string) (*types.MemorySpace, error) {
	return s.space, nil
}

// emptyNotes and emptyPages let the decay sweep run to completion over nothing,
// so the test can assert on how it resolved settings rather than on what it aged.
type emptyNotes struct {
	interfaces.MemoryNoteRepository
}

func (emptyNotes) MarkExpired(context.Context, string, time.Time) (int64, error) { return 0, nil }

type emptyPages struct {
	interfaces.MemoryPageRepository
}

func (emptyPages) ListForDecay(context.Context, string, int) ([]*types.MemoryPage, error) {
	return nil, nil
}

func (emptyPages) Count(context.Context, string, []string) (int64, error) { return 0, nil }

func (emptyPages) PurgeArchivedBefore(context.Context, string, time.Time, int) (int64, error) {
	return 0, nil
}

func TestExtractResolvesSettingsWithTheSpaceOwnersUserLayer(t *testing.T) {
	writer, settings, messages, space := newBackgroundFixture(types.MemoryWriteModeAlwaysAuto)

	err := writer.Extract(context.Background(), types.MemoryExtractPayload{
		TenantID:  7,
		SpaceID:   space.ID,
		SessionID: "session-1",
		AgentID:   "agent-9",
	})
	if err != nil {
		t.Fatalf("Extract returned %v", err)
	}

	if settings.lastOpts.UserID != "u-wizard" {
		t.Fatalf("resolved with user id %q, want the space owner — a write mode set in "+
			"personal settings is otherwise invisible to the task", settings.lastOpts.UserID)
	}
	if settings.lastOpts.AgentID != "agent-9" {
		t.Fatalf("resolved with agent id %q, want the one carried in the payload", settings.lastOpts.AgentID)
	}
	// Reaching the transcript at all proves the gate accepted the user-layer
	// mode; the previous behaviour returned before this point.
	if !messages.called {
		t.Fatal("extraction stopped before reading the conversation, so the user layer was ignored")
	}
}

func TestExtractPutsTheWorkspaceBackOnTheContext(t *testing.T) {
	writer, _, messages, space := newBackgroundFixture(types.MemoryWriteModeAlwaysAuto)

	// A bare context, which is what asynq and the decay ticker both hand over.
	if err := writer.Extract(context.Background(), types.MemoryExtractPayload{
		TenantID:  7,
		SpaceID:   space.ID,
		SessionID: "session-1",
	}); err != nil {
		t.Fatalf("Extract returned %v", err)
	}
	if !messages.called {
		t.Fatal("the transcript was never read")
	}
}

// The re-check itself is worth keeping: settings can be tightened during the
// debounce window, and work queued under the old settings must not run.
func TestExtractDropsWorkWhenAutomaticWritesWereTurnedOff(t *testing.T) {
	writer, _, messages, space := newBackgroundFixture("")

	if err := writer.Extract(context.Background(), types.MemoryExtractPayload{
		TenantID:  7,
		SpaceID:   space.ID,
		SessionID: "session-1",
	}); err != nil {
		t.Fatalf("Extract returned %v", err)
	}
	if messages.called {
		t.Fatal("extraction read the conversation even though automatic writes are off")
	}
}

func TestDecayPutsTheWorkspaceBackOnTheContext(t *testing.T) {
	writer, settings, _, space := newBackgroundFixture("")

	// Decay reads no messages, so the assertion is on the resolve options: the
	// sweep must see the owner's preferences, and no agent governs it.
	if err := writer.Decay(context.Background(), 7, space.ID); err != nil {
		t.Fatalf("Decay returned %v", err)
	}

	if settings.lastOpts.UserID != "u-wizard" {
		t.Fatalf("sweep resolved with user id %q, want the space owner", settings.lastOpts.UserID)
	}
	if settings.lastOpts.AgentID != "" {
		t.Fatalf("sweep resolved with agent id %q, want none", settings.lastOpts.AgentID)
	}
}
