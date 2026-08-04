package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const processingArtifactsTestDDL = `
CREATE TABLE IF NOT EXISTS processing_artifacts (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id        INTEGER NOT NULL,
    stage            VARCHAR(64) NOT NULL,
    key_version      INTEGER NOT NULL,
    artifact_key     CHAR(64) NOT NULL,
    processor_digest CHAR(64) NOT NULL DEFAULT '',
    output_digest    CHAR(64) NOT NULL DEFAULT '',
    output_schema    VARCHAR(64) NOT NULL DEFAULT '',
    codec            VARCHAR(32) NOT NULL DEFAULT 'json',
    inline_payload   BOOLEAN NOT NULL DEFAULT 1,
    payload          BLOB,
    object_ref       TEXT NOT NULL DEFAULT '',
    payload_checksum CHAR(64) NOT NULL DEFAULT '',
    size_bytes       BIGINT NOT NULL DEFAULT 0,
    hit_count        BIGINT NOT NULL DEFAULT 0,
    last_hit_at      DATETIME,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, stage, key_version, artifact_key)
);
`

func setupArtifactPruneDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupHousekeepingDB(t)
	require.NoError(t, db.Exec(processingArtifactsTestDDL).Error)
	return db
}

// insertArtifact writes one artifact row. lastHit nil means the entry has
// never been read back, so created_at is what the retention window sees.
func insertArtifact(t *testing.T, db *gorm.DB, key string, inline bool, createdAt time.Time, lastHit *time.Time) {
	t.Helper()
	objectRef := ""
	if !inline {
		objectRef = "s3://bucket/" + key
	}
	require.NoError(t, db.Exec(
		`INSERT INTO processing_artifacts
		 (tenant_id, stage, key_version, artifact_key, inline_payload, object_ref, created_at, last_hit_at)
		 VALUES (1, 'embedding', 1, ?, ?, ?, ?, ?)`,
		key, inline, objectRef, createdAt, lastHit,
	).Error)
}

func artifactKeys(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var keys []string
	require.NoError(t, db.Table("processing_artifacts").Order("artifact_key").Pluck("artifact_key", &keys).Error)
	return keys
}

func TestPruneProcessingArtifactsDropsOnlyColdInlineEntries(t *testing.T) {
	db := setupArtifactPruneDB(t)
	h := &HousekeepingService{db: db}
	ctx := context.Background()

	old := time.Now().Add(-90 * 24 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)

	insertArtifact(t, db, "cold-never-read", true, old, nil)
	insertArtifact(t, db, "cold-but-recently-read", true, old, &recent)
	insertArtifact(t, db, "fresh", true, recent, nil)
	insertArtifact(t, db, "cold-offloaded", false, old, nil)

	h.pruneProcessingArtifacts(ctx)

	assert.Equal(t, []string{"cold-but-recently-read", "cold-offloaded", "fresh"}, artifactKeys(t, db),
		"a recent read renews the entry, and an offloaded payload must keep its manifest")
}

func TestPruneProcessingArtifactsDisabledByZeroRetention(t *testing.T) {
	db := setupArtifactPruneDB(t)
	h := &HousekeepingService{db: db}
	t.Setenv("WEKNORA_ARTIFACT_RETENTION_DAYS", "0")

	insertArtifact(t, db, "cold", true, time.Now().Add(-365*24*time.Hour), nil)

	h.pruneProcessingArtifacts(context.Background())

	assert.Equal(t, []string{"cold"}, artifactKeys(t, db))
}

func TestPruneProcessingArtifactsHonoursConfiguredWindow(t *testing.T) {
	db := setupArtifactPruneDB(t)
	h := &HousekeepingService{db: db}
	t.Setenv("WEKNORA_ARTIFACT_RETENTION_DAYS", "2")

	insertArtifact(t, db, "idle-3d", true, time.Now().Add(-3*24*time.Hour), nil)
	insertArtifact(t, db, "idle-1d", true, time.Now().Add(-24*time.Hour), nil)

	h.pruneProcessingArtifacts(context.Background())

	assert.Equal(t, []string{"idle-1d"}, artifactKeys(t, db))
}

func TestArtifactRetentionFallsBackOnUnparseableValue(t *testing.T) {
	t.Setenv("WEKNORA_ARTIFACT_RETENTION_DAYS", "not-a-number")
	assert.Equal(t, 30*24*time.Hour, artifactRetention())

	t.Setenv("WEKNORA_ARTIFACT_RETENTION_DAYS", "-5")
	assert.Equal(t, 30*24*time.Hour, artifactRetention())
}

// A missing table (an operator who has not run the migration yet) must
// degrade to a warning rather than aborting the rest of the sweep.
func TestPruneProcessingArtifactsToleratesMissingTable(t *testing.T) {
	db := setupHousekeepingDB(t)
	h := &HousekeepingService{db: db}
	assert.NotPanics(t, func() { h.pruneProcessingArtifacts(context.Background()) })
}
