package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Scope isolation is the one property of long-term memory where a bug is not a
// bug but a privacy incident: every read and write has to be confined to one
// memory space, and one workspace. The plan listed this as a Phase 0 exit
// criterion, so it is pinned here rather than assumed from the fact that every
// query happens to mention space_id today.
//
// These run against a real (in-memory) SQLite database rather than a mock, so
// they exercise the actual WHERE clauses.

func newMemoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_foreign_keys=on"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Start from a clean slate: the shared in-memory DSN is reused across tests
	// in the same process, so leftovers would make results order-dependent.
	_ = db.Migrator().DropTable(
		&types.MemorySpace{}, &types.MemoryPage{},
		&types.MemoryNote{}, &types.MemoryAnchor{}, &types.MemoryPageRevision{},
	)
	if err := db.AutoMigrate(
		&types.MemorySpace{}, &types.MemoryPage{},
		&types.MemoryNote{}, &types.MemoryAnchor{}, &types.MemoryPageRevision{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

type memoryFixture struct {
	db      *gorm.DB
	spaces  interface{}
	pages   *memoryPageRepository
	notes   *memoryNoteRepository
	anchors *memoryAnchorRepository
	spaceA  string
	spaceB  string
}

// seedTwoSpaces creates two spaces in different workspaces, each holding one
// memory, one note and one anchor with the same slug and target. Identical
// content is deliberate: it means any cross-space leak shows up as a wrong
// value rather than as an empty result that a weak assertion could miss.
func seedTwoSpaces(t *testing.T) memoryFixture {
	t.Helper()
	db := newMemoryTestDB(t)
	ctx := context.Background()

	f := memoryFixture{
		db:      db,
		pages:   &memoryPageRepository{db: db},
		notes:   &memoryNoteRepository{db: db},
		anchors: &memoryAnchorRepository{db: db},
		spaceA:  uuid.New().String(),
		spaceB:  uuid.New().String(),
	}

	for i, spec := range []struct {
		spaceID  string
		tenantID uint64
		summary  string
	}{
		{f.spaceA, 1, "alice summary"},
		{f.spaceB, 2, "bob summary"},
	} {
		if err := db.Create(&types.MemorySpace{
			ID: spec.spaceID, TenantID: spec.tenantID, ScopeType: types.MemorySpaceScopeUser,
			OwnerPrincipalType: types.PrincipalWebUser,
			OwnerPrincipalID:   []string{"u-alice", "u-bob"}[i],
			Status:             types.MemorySpaceStatusActive,
		}).Error; err != nil {
			t.Fatalf("create space: %v", err)
		}
		if err := f.pages.Create(ctx, &types.MemoryPage{
			ID: uuid.New().String(), TenantID: spec.tenantID, SpaceID: spec.spaceID,
			Slug: "preference/answer-style", Title: "Answer style", Summary: spec.summary,
			PageType: types.MemoryTypePreference, Status: types.MemoryPageStatusActive,
			Strength: 1, Version: 1,
		}); err != nil {
			t.Fatalf("create page: %v", err)
		}
		if err := f.notes.CreateBatch(ctx, []*types.MemoryNote{{
			ID: uuid.New().String(), TenantID: spec.tenantID, SpaceID: spec.spaceID,
			NoteType: types.MemoryTypeEpisode, Statement: spec.summary,
			Status: types.MemoryNoteStatusPending, NormalizedHash: "same-hash",
		}}); err != nil {
			t.Fatalf("create note: %v", err)
		}
		if err := f.anchors.Upsert(ctx, &types.MemoryAnchorUpsert{
			SpaceID: spec.spaceID, TenantID: spec.tenantID,
			KnowledgeBaseID: "kb1", TargetKind: types.MemoryAnchorTargetWikiPage,
			TargetRef: "concept/rag", Relation: types.MemoryRelationAskedAbout,
		}); err != nil {
			t.Fatalf("upsert anchor: %v", err)
		}
	}
	return f
}

func TestMemoryPageReadsAreConfinedToTheirSpace(t *testing.T) {
	f := seedTwoSpaces(t)
	ctx := context.Background()

	page, err := f.pages.GetBySlug(ctx, f.spaceA, "preference/answer-style")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if page.Summary != "alice summary" {
		t.Errorf("summary = %q, want alice's: a shared slug must resolve per space", page.Summary)
	}

	// The same slug exists in the other space and must not be reachable.
	other, err := f.pages.GetBySlug(ctx, f.spaceB, "preference/answer-style")
	if err != nil {
		t.Fatalf("GetBySlug for the second space: %v", err)
	}
	if other.Summary != "bob summary" {
		t.Errorf("summary = %q, want bob's", other.Summary)
	}

	list, total, err := f.pages.List(ctx, &types.MemoryPageListRequest{SpaceID: f.spaceA})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("list returned %d/%d rows, want exactly this space's one", len(list), total)
	}

	all, err := f.pages.ListAll(ctx, f.spaceA)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("ListAll returned %d rows, want 1", len(all))
	}

	found, err := f.pages.Search(ctx, f.spaceA, "summary", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(found) != 1 || found[0].Summary != "alice summary" {
		t.Errorf("search leaked across spaces: %+v", found)
	}
}

func TestMemoryPageWritesCannotReachAnotherSpace(t *testing.T) {
	f := seedTwoSpaces(t)
	ctx := context.Background()

	victim, err := f.pages.GetBySlug(ctx, f.spaceB, "preference/answer-style")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}

	// Bob's page id with Alice's space id must not update anything: the id alone
	// is not authority, which is what makes a leaked identifier harmless.
	forged := *victim
	forged.SpaceID = f.spaceA
	forged.Summary = "overwritten"
	if err := f.pages.Update(ctx, &forged, 0); err != ErrMemoryPageNotFound {
		t.Errorf("Update across spaces returned %v, want ErrMemoryPageNotFound", err)
	}

	unchanged, err := f.pages.GetBySlug(ctx, f.spaceB, "preference/answer-style")
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if unchanged.Summary != "bob summary" {
		t.Errorf("summary = %q, want it untouched", unchanged.Summary)
	}

	if err := f.pages.Delete(ctx, f.spaceA, victim.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := f.pages.GetBySlug(ctx, f.spaceB, "preference/answer-style"); err != nil {
		t.Errorf("a delete scoped to another space removed the row: %v", err)
	}

	if n, err := f.pages.DeleteAll(ctx, f.spaceA); err != nil || n != 1 {
		t.Errorf("DeleteAll removed %d rows (err %v), want only this space's 1", n, err)
	}
	if count, err := f.pages.Count(ctx, f.spaceB, nil); err != nil || count != 1 {
		t.Errorf("the other space now has %d pages (err %v), want 1", count, err)
	}
}

func TestMemoryNoteScoping(t *testing.T) {
	f := seedTwoSpaces(t)
	ctx := context.Background()

	// Both spaces hold a note with the same normalised hash. De-duplication must
	// be per space, or one person's memory would suppress another's.
	exists, err := f.notes.ExistsHash(ctx, f.spaceA, "same-hash")
	if err != nil || !exists {
		t.Errorf("ExistsHash in own space = %v (err %v), want true", exists, err)
	}

	pending, err := f.notes.ListPending(ctx, f.spaceA, 10)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 || pending[0].Statement != "alice summary" {
		t.Errorf("pending notes leaked across spaces: %+v", pending)
	}

	if err := f.notes.UpdateStatus(
		ctx, f.spaceA, pending[0].ID, types.MemoryNoteStatusRejected, "",
	); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	// The other space's note must be untouched, and addressing it with the wrong
	// space must fail rather than succeed silently.
	bobNotes, _, err := f.notes.List(ctx, &types.MemoryNoteListRequest{
		SpaceID: f.spaceB, Statuses: []string{types.MemoryNoteStatusPending},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(bobNotes) != 1 {
		t.Fatalf("the other space has %d pending notes, want 1", len(bobNotes))
	}
	if err := f.notes.UpdateStatus(
		ctx, f.spaceA, bobNotes[0].ID, types.MemoryNoteStatusRejected, "",
	); err != ErrMemoryNoteNotFound {
		t.Errorf("cross-space UpdateStatus returned %v, want ErrMemoryNoteNotFound", err)
	}
}

func TestMemoryAnchorScoping(t *testing.T) {
	f := seedTwoSpaces(t)
	ctx := context.Background()

	// The same target in the same knowledge base, anchored by two different
	// people, must stay two separate anchors.
	overlayA, err := f.anchors.ListOverlay(ctx, f.spaceA, "kb1", types.MemoryAnchorTargetWikiPage)
	if err != nil {
		t.Fatalf("ListOverlay: %v", err)
	}
	if len(overlayA) != 1 {
		t.Errorf("overlay returned %d anchors, want only this space's 1", len(overlayA))
	}

	countA, err := f.anchors.Count(ctx, f.spaceA)
	if err != nil || countA != 1 {
		t.Errorf("count = %d (err %v), want 1", countA, err)
	}

	if n, err := f.anchors.DeleteAll(ctx, f.spaceA); err != nil || n != 1 {
		t.Errorf("DeleteAll removed %d anchors (err %v), want 1", n, err)
	}
	if countB, err := f.anchors.Count(ctx, f.spaceB); err != nil || countB != 1 {
		t.Errorf("the other space now has %d anchors (err %v), want 1", countB, err)
	}
}

// The insights aggregate is the one query that deliberately spans spaces. It
// must still respect the workspace boundary, and must never expose which space
// contributed what — only how many did.
func TestMemoryAnchorAggregateStaysWithinTheWorkspace(t *testing.T) {
	f := seedTwoSpaces(t)
	ctx := context.Background()

	rows, err := f.anchors.AggregateByTarget(ctx, 1, "kb1")
	if err != nil {
		t.Fatalf("AggregateByTarget: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("aggregate returned %d rows, want 1", len(rows))
	}
	if rows[0].DistinctSpaces != 1 {
		t.Errorf("distinct spaces = %d, want 1: the other workspace must not be counted",
			rows[0].DistinctSpaces)
	}
}

func TestMemorySpaceLookupIsScopedToWorkspaceAndPrincipal(t *testing.T) {
	f := seedTwoSpaces(t)
	ctx := context.Background()
	repo := &memorySpaceRepository{db: f.db}

	alice := types.Principal{Type: types.PrincipalWebUser, ID: "u-alice"}
	space, err := repo.GetByOwner(ctx, 1, types.MemorySpaceScopeUser, alice)
	if err != nil {
		t.Fatalf("GetByOwner: %v", err)
	}
	if space.ID != f.spaceA {
		t.Errorf("resolved space %s, want %s", space.ID, f.spaceA)
	}

	// The same principal id in a different workspace is a different person as
	// far as memory is concerned, and must not resolve.
	if _, err := repo.GetByOwner(ctx, 2, types.MemorySpaceScopeUser, alice); err != ErrMemorySpaceNotFound {
		t.Errorf("cross-workspace lookup returned %v, want ErrMemorySpaceNotFound", err)
	}

	if _, err := repo.GetByID(ctx, 1, f.spaceB); err != ErrMemorySpaceNotFound {
		t.Errorf("reading another workspace's space returned %v, want ErrMemorySpaceNotFound", err)
	}
}

func TestMemoryPageHitBumpIsScoped(t *testing.T) {
	f := seedTwoSpaces(t)
	ctx := context.Background()

	victim, err := f.pages.GetBySlug(ctx, f.spaceB, "preference/answer-style")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if err := f.pages.BumpHits(ctx, f.spaceA, []string{victim.ID}, time.Now()); err != nil {
		t.Fatalf("BumpHits: %v", err)
	}
	after, err := f.pages.GetBySlug(ctx, f.spaceB, "preference/answer-style")
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.HitCount != 0 {
		t.Errorf("hit count = %d, want 0: usage recorded against the wrong space", after.HitCount)
	}
}
