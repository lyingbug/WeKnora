package repository

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Content-addressed chunk IDs are stable, so a paragraph that is deleted and
// later restored resolves to the very same primary key. DeleteChunks is a soft
// delete, so the tombstone still occupies that key while staying invisible to
// the reconciliation read path — reinserting it must not fail.
func TestPurgeSoftDeletedChunksAllowsStableIDReinsert(t *testing.T) {
	db := setupChunkTestDB(t)
	repo := NewChunkRepository(db)
	ctx := context.Background()

	kbID := uuid.New().String()
	knowledgeID := uuid.New().String()
	stableID := "5c6b0b3e-4a3a-5a2f-9b1d-2f4c8e7a1d90"

	chunk := makeChunk(kbID, knowledgeID, types.ChunkTypeText)
	chunk.ID = stableID
	require.NoError(t, repo.CreateChunks(ctx, []*types.Chunk{chunk}))

	require.NoError(t, repo.DeleteChunks(ctx, 1, []string{stableID}))

	active, err := repo.ListAllChunksByKnowledgeID(ctx, 1, knowledgeID)
	require.NoError(t, err)
	require.Empty(t, active, "tombstone must stay invisible to the reconciliation read path")

	purged, err := repo.PurgeSoftDeletedChunks(ctx, 1, []string{stableID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), purged)

	restored := makeChunk(kbID, knowledgeID, types.ChunkTypeText)
	restored.ID = stableID
	require.NoError(t, repo.CreateChunks(ctx, []*types.Chunk{restored}),
		"restoring content under its stable ID must not collide with the tombstone")

	active, err = repo.ListAllChunksByKnowledgeID(ctx, 1, knowledgeID)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, stableID, active[0].ID)
}

// Purging must be surgical: it may only reclaim tombstoned keys the caller is
// about to reinsert, never a live row that a concurrent attempt still owns.
func TestPurgeSoftDeletedChunksLeavesLiveRowsIntact(t *testing.T) {
	db := setupChunkTestDB(t)
	repo := NewChunkRepository(db)
	ctx := context.Background()

	kbID := uuid.New().String()
	knowledgeID := uuid.New().String()

	live := makeChunk(kbID, knowledgeID, types.ChunkTypeText)
	tombstoned := makeChunk(kbID, knowledgeID, types.ChunkTypeText)
	require.NoError(t, repo.CreateChunks(ctx, []*types.Chunk{live, tombstoned}))
	require.NoError(t, repo.DeleteChunks(ctx, 1, []string{tombstoned.ID}))

	purged, err := repo.PurgeSoftDeletedChunks(ctx, 1, []string{live.ID, tombstoned.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), purged, "only the tombstoned row may be reclaimed")

	active, err := repo.ListAllChunksByKnowledgeID(ctx, 1, knowledgeID)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, live.ID, active[0].ID)
}

// Another tenant's tombstone must never be reclaimed, even on an ID collision.
func TestPurgeSoftDeletedChunksIsTenantScoped(t *testing.T) {
	db := setupChunkTestDB(t)
	repo := NewChunkRepository(db)
	ctx := context.Background()

	other := makeChunk(uuid.New().String(), uuid.New().String(), types.ChunkTypeText)
	other.TenantID = 2
	require.NoError(t, repo.CreateChunks(ctx, []*types.Chunk{other}))
	require.NoError(t, repo.DeleteChunks(ctx, 2, []string{other.ID}))

	purged, err := repo.PurgeSoftDeletedChunks(ctx, 1, []string{other.ID})
	require.NoError(t, err)
	assert.Zero(t, purged)

	var count int64
	require.NoError(t, db.Unscoped().Model(&types.Chunk{}).Where("id = ?", other.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
