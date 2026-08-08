package memory

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// The write path had to be reachable before any of the machinery behind it
// mattered. It was not: the default write mode honours only direct requests, and
// nothing in the product ever issued one, so a fresh deployment could converse
// forever and remember nothing.
//
// These run the real gate against real repositories on SQLite, so what is pinned
// is whether a memory actually lands in the database — not whether some
// intermediate function was called.

func newWritePathFixture(t *testing.T) (*writerService, string, context.Context) {
	t.Helper()

	db, err := gorm.Open(
		sqlite.Open("file:memwrite?mode=memory&cache=shared&_foreign_keys=on"),
		&gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)},
	)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	models := []any{
		&types.MemorySpace{}, &types.MemoryPage{},
		&types.MemoryNote{}, &types.MemoryAnchor{}, &types.MemoryPageRevision{},
	}
	_ = db.Migrator().DropTable(models...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	spaces := repository.NewMemorySpaceRepository(db)
	pages := repository.NewMemoryPageRepository(db)
	notes := repository.NewMemoryNoteRepository(db)
	anchors := repository.NewMemoryAnchorRepository(db)

	service := NewService(spaces, pages, notes, anchors, nil)
	// The dependencies left nil are the ones a direct request never touches: no
	// transcript is read and no model is called, which is exactly what makes the
	// explicit-only mode free.
	writer := &writerService{
		spaces:  spaces,
		pages:   pages,
		notes:   notes,
		service: service,
	}

	const tenantID uint64 = 7
	space := &types.MemorySpace{
		ID:                 uuid.New().String(),
		TenantID:           tenantID,
		ScopeType:          types.MemorySpaceScopeUser,
		OwnerPrincipalType: types.PrincipalWebUser,
		OwnerPrincipalID:   "u-wizard",
		DisplayName:        "wizard",
		Status:             types.MemorySpaceStatusActive,
	}
	if err := spaces.Create(context.Background(), space); err != nil {
		t.Fatalf("create space: %v", err)
	}

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)
	return writer, space.ID, ctx
}

// explicitOnlySettings mirrors a deployment nobody has configured: memory on,
// write mode at its default.
func explicitOnlySettings() types.MemorySettings {
	settings := types.ResolveMemorySettings().Settings
	if settings.WriteMode != types.MemoryWriteModeExplicit {
		panic("the default write mode changed; this test is about that default")
	}
	settings.Enabled = true
	return settings
}

func listPages(t *testing.T, writer *writerService, spaceID string) []*types.MemoryPage {
	t.Helper()
	got, err := writer.pages.ListAll(context.Background(), spaceID)
	if err != nil {
		t.Fatalf("list pages: %v", err)
	}
	return got
}

func TestConsiderSessionStoresADirectRequestUnderTheDefaultWriteMode(t *testing.T) {
	writer, spaceID, ctx := newWritePathFixture(t)

	writer.ConsiderSession(ctx, types.MemoryExtractTrigger{
		TenantID:  7,
		SpaceID:   spaceID,
		SessionID: "session-1",
		MessageID: "message-1",
		Settings:  explicitOnlySettings(),
		UserText:  "记住我是 wizard，一个程序工程师",
		TurnIndex: 1,
	})

	stored := listPages(t, writer, spaceID)
	if len(stored) != 1 {
		t.Fatalf("stored %d memories, want 1 — the direct request was dropped", len(stored))
	}
	if stored[0].Content != "我是 wizard，一个程序工程师" {
		t.Fatalf("content = %q, want the statement without the imperative", stored[0].Content)
	}
	if stored[0].LastEditSource != types.MemoryEditSourceUser {
		t.Fatalf("edit source = %q, want %q", stored[0].LastEditSource, types.MemoryEditSourceUser)
	}
	if !stored[0].Saved {
		t.Fatal("a direct remember request was not marked saved")
	}

	// The evidence trail matters as much as the memory: the user must be able to
	// see which message produced it.
	kept, err := writer.notes.ListAll(ctx, spaceID)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("recorded %d notes, want 1", len(kept))
	}
	if len(kept[0].SourceMessageIDs) != 1 || kept[0].SourceMessageIDs[0] != "message-1" {
		t.Fatalf("source messages = %v, want [message-1]", kept[0].SourceMessageIDs)
	}
	if !stored[0].NoteRefs.Contains(kept[0].ID) {
		t.Fatalf("page note refs = %v, want explicit evidence note %s", stored[0].NoteRefs, kept[0].ID)
	}
}

func TestDirectPreferenceBecomesStructuredSavedMemory(t *testing.T) {
	writer, spaceID, ctx := newWritePathFixture(t)
	writer.ConsiderSession(ctx, types.MemoryExtractTrigger{
		TenantID: 7, SpaceID: spaceID, SessionID: "session-1", MessageID: "message-1",
		Settings: explicitOnlySettings(), UserText: "记住我偏好简体中文回答", TurnIndex: 1,
	})

	stored := listPages(t, writer, spaceID)
	if len(stored) != 1 {
		t.Fatalf("stored %d memories, want 1", len(stored))
	}
	if stored[0].PageType != types.MemoryTypePreference || stored[0].Structured.Language != "zh" {
		t.Fatalf("preference was not typed: type=%q structured=%+v", stored[0].PageType, stored[0].Structured)
	}
}

func TestCandidateEvidenceUsesOnlyCitedMessages(t *testing.T) {
	writer, spaceID, _ := newWritePathFixture(t)
	settings := explicitOnlySettings()
	settings.MinConfidence = 0.6
	settings.MaxNotesPerWindow = 10
	messages := []*types.Message{
		{ID: "user-empty", Role: "user", Content: "   "},
		{ID: "user-1", Role: "user", Content: "我用 Go"},
		{ID: "assistant-1", Role: "assistant", Content: "你用 SQLite"},
		{ID: "user-2", Role: "user", Content: "项目数据库是 PostgreSQL"},
	}
	space := &types.MemorySpace{ID: spaceID}
	notes := writer.candidatesToNotes(context.Background(), 7, space, "session-1", messages, settings,
		[]memoryCandidate{{
			Type: types.MemoryTypeProject, Key: "project/database", Statement: "项目数据库是 PostgreSQL",
			Confidence: 0.95, Evidence: []string{"m1"},
		}})
	if len(notes) != 1 || len(notes[0].SourceMessageIDs) != 1 || notes[0].SourceMessageIDs[0] != "user-2" {
		t.Fatalf("evidence = %+v, want only user-2", notes)
	}

	missing := writer.candidatesToNotes(context.Background(), 7, space, "session-1", messages, settings,
		[]memoryCandidate{{
			Type: types.MemoryTypeProject, Key: "project/database", Statement: "项目数据库是 PostgreSQL",
			Confidence: 0.95,
		}})
	if len(missing) != 0 {
		t.Fatalf("accepted %d candidates without evidence", len(missing))
	}
}

func TestDifferentMemoryKeysDoNotOverwriteEachOther(t *testing.T) {
	writer, spaceID, ctx := newWritePathFixture(t)
	space, err := writer.spaces.GetByID(ctx, 7, spaceID)
	if err != nil {
		t.Fatal(err)
	}
	sc := &scope{TenantID: 7, Space: space, Settings: explicitOnlySettings()}
	notes := []*types.MemoryNote{
		{ID: "note-db", TenantID: 7, SpaceID: spaceID, NoteType: types.MemoryTypeProject,
			MemoryKey: "project/weknora/database", Statement: "WeKnora 使用 PostgreSQL", Confidence: 0.9,
			Status: types.MemoryNoteStatusPending, NormalizedHash: "db"},
		{ID: "note-lang", TenantID: 7, SpaceID: spaceID, NoteType: types.MemoryTypeProject,
			MemoryKey: "project/weknora/language", Statement: "WeKnora 后端使用 Go", Confidence: 0.9,
			Status: types.MemoryNoteStatusPending, NormalizedHash: "lang"},
	}
	if err := writer.notes.CreateBatch(ctx, notes); err != nil {
		t.Fatal(err)
	}
	for _, note := range notes {
		if _, err := writer.consolidateNote(ctx, sc, note); err != nil {
			t.Fatalf("consolidate %s: %v", note.ID, err)
		}
	}
	if got := listPages(t, writer, spaceID); len(got) != 2 {
		t.Fatalf("got %d pages, want two independent project facts", len(got))
	}
}

func TestChatHistoryCannotOverwriteSavedMemory(t *testing.T) {
	writer, spaceID, ctx := newWritePathFixture(t)
	saved, err := writer.RememberExplicit(ctx, types.MemoryExplicitWriteRequest{
		TenantID: 7, SpaceID: spaceID, Statement: "WeKnora 使用 PostgreSQL",
		NoteType: types.MemoryTypeProject, MemoryKey: "project/weknora/database",
		Settings: explicitOnlySettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	note := &types.MemoryNote{
		ID: "note-conflict", TenantID: 7, SpaceID: spaceID, NoteType: types.MemoryTypeProject,
		MemoryKey: "project/weknora/database", Statement: "WeKnora 使用 SQLite", Confidence: 0.9,
		Status: types.MemoryNoteStatusPending, NormalizedHash: "conflict",
	}
	if err := writer.notes.CreateBatch(ctx, []*types.MemoryNote{note}); err != nil {
		t.Fatal(err)
	}
	space, _ := writer.spaces.GetByID(ctx, 7, spaceID)
	if page, err := writer.consolidateNote(ctx, &scope{
		TenantID: 7, Space: space, Settings: explicitOnlySettings(),
	}, note); err != nil || page != nil {
		t.Fatalf("conflicting history result page=%+v err=%v, want ignored", page, err)
	}
	kept, err := writer.pages.GetByID(ctx, spaceID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !kept.Saved || kept.Summary != "WeKnora 使用 PostgreSQL" {
		t.Fatalf("saved memory was changed by history: %+v", kept)
	}
}

func TestRecallAlwaysConsidersSavedMemoriesRegardlessOfType(t *testing.T) {
	writer, spaceID, ctx := newWritePathFixture(t)
	if err := writer.pages.Create(ctx, &types.MemoryPage{
		ID: uuid.New().String(), TenantID: 7, SpaceID: spaceID, Slug: "episode/saved",
		Title: "Saved", PageType: types.MemoryTypeEpisode, Status: types.MemoryPageStatusActive,
		Saved: true, Content: "用户明确要求记住这件事", Summary: "用户明确要求记住这件事", Strength: 1,
	}); err != nil {
		t.Fatal(err)
	}
	settings := explicitOnlySettings()
	settings.RecallEnabled = true
	settings.InjectionTokenBudget = 200
	settings.RecallMaxItems = 8
	recall := NewRecallService(writer.spaces, writer.pages, nil)
	result := recall.Recall(ctx, types.MemoryRecallRequest{
		SpaceID: spaceID, Query: "完全无关的问题", Language: "zh", Settings: settings,
	})
	if result == nil || len(result.Items) != 1 || result.Items[0].Slug != "episode/saved" {
		t.Fatalf("saved memory was not resident: %+v", result)
	}
}

// The message that exposed all of this. It is a statement, not a request, so the
// default write mode declining it is correct — and worth pinning, because the
// obvious over-correction is to store every sentence containing "I am".
func TestConsiderSessionLeavesABareStatementAloneUnderTheDefaultWriteMode(t *testing.T) {
	writer, spaceID, ctx := newWritePathFixture(t)

	writer.ConsiderSession(ctx, types.MemoryExtractTrigger{
		TenantID:  7,
		SpaceID:   spaceID,
		SessionID: "session-1",
		Settings:  explicitOnlySettings(),
		UserText:  "我是wizard，一个程序工程师，你知道我是谁吗",
		TurnIndex: 1,
	})

	if stored := listPages(t, writer, spaceID); len(stored) != 0 {
		t.Fatalf("stored %d memories, want none for a bare statement", len(stored))
	}
}

func TestConsiderSessionIgnoresADirectRequestWhileWritesAreOff(t *testing.T) {
	writer, spaceID, ctx := newWritePathFixture(t)

	settings := explicitOnlySettings()
	settings.WriteMode = types.MemoryWriteModeOff

	writer.ConsiderSession(ctx, types.MemoryExtractTrigger{
		TenantID: 7,
		SpaceID:  spaceID,
		Settings: settings,
		UserText: "记住我是 wizard，一个程序工程师",
	})

	// "Off" has to mean off, direct request or not: it is the switch a workspace
	// admin uses to guarantee nothing is retained.
	if stored := listPages(t, writer, spaceID); len(stored) != 0 {
		t.Fatalf("stored %d memories, want none while writes are off", len(stored))
	}
}

// stubChatModel stands in for a configured chat model. Extraction only needs a
// parsable envelope back; what it contains is the extractor's concern, not this
// test's.
type stubChatModel struct{}

func (*stubChatModel) Chat(
	context.Context, []chat.Message, *chat.ChatOptions,
) (*types.ChatResponse, error) {
	return &types.ChatResponse{Content: `{"candidates":[]}`}, nil
}

func (*stubChatModel) ChatStream(
	context.Context, []chat.Message, *chat.ChatOptions,
) (<-chan types.StreamResponse, error) {
	return nil, nil
}

func (*stubChatModel) GetModelName() string { return "stub" }
func (*stubChatModel) GetModelID() string   { return "stub" }

// stubModelService resolves exactly one model id and records which was asked for.
type stubModelService struct {
	interfaces.ModelService
	asked string
}

func (s *stubModelService) GetChatModel(_ context.Context, id string) (chat.Chat, error) {
	s.asked = id
	return &stubChatModel{}, nil
}

// The extraction model setting is documented as "leave blank to use the
// conversation's model". It used to refuse to run instead, which made every
// automatic mode fail on a deployment that had never touched the setting.
func TestExtractionFallsBackToTheConversationModel(t *testing.T) {
	models := &stubModelService{}
	writer := &writerService{models: models}
	settings := explicitOnlySettings()

	if _, err := writer.callExtractor(
		context.Background(), settings, "我是 wizard", "conversation-model",
	); err != nil {
		t.Fatalf("extraction failed with no configured model: %v", err)
	}
	if models.asked != "conversation-model" {
		t.Fatalf("asked for model %q, want the conversation's model", models.asked)
	}
}

// A configured extraction model still wins: the point of the setting is to let a
// workspace send extraction to a cheaper model than the one it converses with.
func TestExtractionPrefersTheConfiguredModel(t *testing.T) {
	models := &stubModelService{}
	writer := &writerService{models: models}
	settings := explicitOnlySettings()
	settings.ExtractionModelID = "cheap-model"

	if _, err := writer.callExtractor(
		context.Background(), settings, "我是 wizard", "conversation-model",
	); err != nil {
		t.Fatalf("extraction failed: %v", err)
	}
	if models.asked != "cheap-model" {
		t.Fatalf("asked for model %q, want the configured one", models.asked)
	}
}
