package repository

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// The repository layer builds dialect-specific SQL, and the differences that
// matter most are the ones SQLite happens to tolerate. SQLite accepts the
// PostgreSQL bare-key form `metadata->>'external_id'`; MySQL rejects it with
// error 3143 as soon as a row holds non-null JSON. A suite that only ever runs
// on SQLite therefore reports green for queries that cannot run in production
// on MySQL at all, which is exactly how that shape of bug reaches users.
//
// These tests execute the real repositories against a real MySQL server using
// the schema from migrations/mysql. They are skipped unless
// WEKNORA_MYSQL_TEST_DSN points at a throwaway database; CI supplies one.
const mysqlTestDSNEnv = "WEKNORA_MYSQL_TEST_DSN"

// mysqlBaselinePath is the consolidated MySQL schema. Tests load it directly
// rather than going through golang-migrate so that a schema mistake surfaces
// as a failing query rather than as a migration-tooling error.
const mysqlBaselinePath = "../../../migrations/mysql/000000_init.up.sql"

func setupMySQLTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv(mysqlTestDSNEnv))
	if dsn == "" {
		t.Skipf("%s is not set; skipping MySQL integration tests", mysqlTestDSNEnv)
	}

	baseline, err := os.ReadFile(mysqlBaselinePath)
	require.NoError(t, err, "read MySQL baseline schema")

	admin, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "connect to MySQL")
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	defer func() { _ = adminSQL.Close() }()

	// A dedicated database per test keeps runs independent and lets a failing
	// test leave its rows behind for inspection without affecting the others.
	schema := "weknora_it_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24]
	require.NoError(t, admin.Exec("CREATE DATABASE `"+schema+"` CHARACTER SET utf8mb4").Error)

	db, err := gorm.Open(mysql.Open(mysqlDSNWithDatabase(t, dsn, schema)), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)

	for _, statement := range splitSQLStatements(string(baseline)) {
		require.NoError(t, db.Exec(statement).Error, "apply baseline statement: %.120s", statement)
	}

	t.Cleanup(func() {
		_ = sqlDB.Close()
		cleanup, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err != nil {
			return
		}
		defer func() {
			if raw, err := cleanup.DB(); err == nil {
				_ = raw.Close()
			}
		}()
		_ = cleanup.Exec("DROP DATABASE IF EXISTS `" + schema + "`").Error
	})

	return db
}

// mysqlDSNWithDatabase rewrites the database name in a go-sql-driver DSN,
// whose shape is user:pass@tcp(host:port)/dbname?params.
func mysqlDSNWithDatabase(t *testing.T, dsn, database string) string {
	t.Helper()
	slash := strings.LastIndex(dsn, "/")
	require.Greater(t, slash, -1, "DSN %q must contain a / before the database name", dsn)
	params := ""
	if question := strings.Index(dsn[slash:], "?"); question > -1 {
		params = dsn[slash+question:]
	}
	return dsn[:slash+1] + database + params
}

// splitSQLStatements splits a migration file on statement terminators. Comments
// are dropped first: a `;` inside a comment would otherwise cut a CREATE TABLE
// in half and report the remainder as a syntax error. The baseline declares no
// stored programs, so a plain `;` split is sufficient once comments are gone.
func splitSQLStatements(script string) []string {
	var statements []string
	for _, chunk := range strings.Split(stripSQLComments(script), ";") {
		if trimmed := strings.TrimSpace(chunk); trimmed != "" {
			statements = append(statements, trimmed)
		}
	}
	return statements
}

func stripSQLComments(script string) string {
	var kept []string
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func insertMySQLKnowledge(t *testing.T, db *gorm.DB, tenantID uint64, kbID, externalID string) string {
	t.Helper()
	id := uuid.New().String()
	require.NoError(t, db.Exec(`
		INSERT INTO knowledges
		  (id, tenant_id, knowledge_base_id, type, title, source, parse_status, metadata)
		VALUES (?, ?, ?, 'document', ?, 'feishu', 'completed', ?)
	`, id, tenantID, kbID, externalID, fmt.Sprintf(`{"external_id":%q}`, externalID)).Error)
	return id
}

// insertMySQLTenant seeds a tenant so rows with a tenant_id foreign key can be
// inserted. tenants.id is AUTO_INCREMENT, so the assigned id is read back.
func insertMySQLTenant(t *testing.T, db *gorm.DB) uint64 {
	t.Helper()
	name := "it-" + uuid.New().String()
	require.NoError(t, db.Exec(
		`INSERT INTO tenants (name, business) VALUES (?, 'integration-test')`, name,
	).Error)
	var id uint64
	require.NoError(t, db.Raw(`SELECT id FROM tenants WHERE name = ?`, name).Scan(&id).Error)
	return id
}

func insertMySQLChunk(t *testing.T, db *gorm.DB, tenantID uint64, kbID, knowledgeID, content string) string {
	t.Helper()
	id := uuid.New().String()
	require.NoError(t, db.Exec(`
		INSERT INTO chunks
		  (id, tenant_id, knowledge_id, knowledge_base_id, content,
		   chunk_index, start_at, end_at, is_enabled, chunk_type, flags, status)
		VALUES (?, ?, ?, ?, ?, 0, 0, ?, TRUE, 'text', 0, 0)
	`, id, tenantID, knowledgeID, kbID, content, len(content)).Error)
	return id
}

// TestMySQLFindByMetadataKeyPrefix is the regression test for the bare-key
// `metadata->>'external_id'` form. Its only caller logs and returns on error,
// so on MySQL the failure was silent: re-syncing a datasource stopped sweeping
// sub-items that had disappeared upstream, leaking orphan knowledge rows.
func TestMySQLFindByMetadataKeyPrefix(t *testing.T) {
	db := setupMySQLTestDB(t)
	repo := NewKnowledgeRepository(db).(*knowledgeRepository)
	ctx := context.Background()

	const tenantID uint64 = 4242
	kbID := uuid.New().String()
	otherKBID := uuid.New().String()

	_ = insertMySQLKnowledge(t, db, tenantID, kbID, "nodeA")
	childID := insertMySQLKnowledge(t, db, tenantID, kbID, "nodeA#file#x")
	_ = insertMySQLKnowledge(t, db, tenantID, kbID, "nodeB")
	_ = insertMySQLKnowledge(t, db, tenantID, otherKBID, "nodeA#file#y")

	results, err := repo.FindByMetadataKeyPrefix(ctx, tenantID, kbID, "external_id", "nodeA#")
	require.NoError(t, err)
	require.Len(t, results, 1, "only the attachment child should match the prefix 'nodeA#'")
	assert.Equal(t, childID, results[0].ID)
}

func TestMySQLFindByMetadataKey(t *testing.T) {
	db := setupMySQLTestDB(t)
	repo := NewKnowledgeRepository(db)
	ctx := context.Background()

	const tenantID uint64 = 4243
	kbID := uuid.New().String()
	wanted := insertMySQLKnowledge(t, db, tenantID, kbID, "node-exact")
	_ = insertMySQLKnowledge(t, db, tenantID, kbID, "node-other")

	found, err := repo.FindByMetadataKey(ctx, tenantID, kbID, "external_id", "node-exact")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, wanted, found.ID)

	missing, err := repo.FindByMetadataKey(ctx, tenantID, kbID, "external_id", "absent")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

// TestMySQLMetadataExternalIDUsesIndex guards the generated column that
// materializes metadata->>'$.external_id'. Without it both lookups above are
// full scans, and a datasource sync runs one per item — invisible on the small
// datasets these tests use, painful on a real knowledge base.
func TestMySQLMetadataExternalIDUsesIndex(t *testing.T) {
	db := setupMySQLTestDB(t)

	// possible_keys rather than key: which index the optimizer finally picks
	// depends on table statistics, but possible_keys answers the question this
	// test exists for — whether MySQL can match the predicate to the index at
	// all. For the equality form that means generated-column substitution
	// recognised metadata ->> '$.external_id'; MySQL never substitutes for LIKE,
	// so the prefix form has to name the generated column itself.
	for name, query := range map[string]string{
		"equality": "SELECT id FROM knowledges WHERE knowledge_base_id = 'kb' " +
			"AND metadata ->> '$.external_id' = 'node'",
		"prefix": "SELECT id FROM knowledges WHERE knowledge_base_id = 'kb' " +
			"AND metadata_external_id LIKE 'node#%'",
	} {
		t.Run(name, func(t *testing.T) {
			var plan struct {
				PossibleKeys *string `gorm:"column:possible_keys"`
			}
			require.NoError(t, db.Raw("EXPLAIN "+query).Scan(&plan).Error)
			require.NotNil(t, plan.PossibleKeys, "predicate must be indexable, got a full scan")
			assert.Contains(t, *plan.PossibleKeys, "idx_knowledges_kb_metadata_external_id")
		})
	}
}

// TestMySQLUpdateChunks covers the batched CASE update. PostgreSQL needs the
// `(CASE ... END)::boolean` cast fed by string arguments, MySQL needs native
// bool/int under STRICT_TRANS_TABLES, and the two are easy to conflate: binding
// "true" to a tinyint raises error 1292, while casting on the PostgreSQL side
// with a native bool makes pgx refuse to encode the argument at all.
func TestMySQLUpdateChunks(t *testing.T) {
	db := setupMySQLTestDB(t)
	repo := NewChunkRepository(db)
	ctx := context.Background()

	tenantID := insertMySQLTenant(t, db)
	kbID := uuid.New().String()
	knowledgeID := insertMySQLKnowledge(t, db, tenantID, kbID, "chunk-owner")
	chunkID := insertMySQLChunk(t, db, tenantID, kbID, knowledgeID, "original content")

	updated := &types.Chunk{
		ID:              chunkID,
		TenantID:        tenantID,
		KnowledgeID:     knowledgeID,
		KnowledgeBaseID: kbID,
		Content:         "rewritten content",
		IsEnabled:       false,
		Flags:           3,
		Status:          2,
	}
	require.NoError(t, repo.UpdateChunks(ctx, []*types.Chunk{updated}))

	var stored struct {
		Content   string
		IsEnabled bool
		Flags     int
		Status    int
	}
	require.NoError(t, db.Raw(
		"SELECT content, is_enabled, flags, status FROM chunks WHERE id = ?", chunkID,
	).Scan(&stored).Error)
	assert.Equal(t, "rewritten content", stored.Content)
	assert.False(t, stored.IsEnabled)
	assert.Equal(t, 3, stored.Flags)
	assert.Equal(t, 2, stored.Status)
}

// TestMySQLSeqIDStartsAboveReservedRange pins the AUTO_INCREMENT start values
// to the PostgreSQL sequence start values. FAQ import lets a caller choose a
// seq_id below the start value (types.FAQImportEntry.ID documents the rule), so
// generated values entering that range collide with imported ones.
func TestMySQLSeqIDStartsAboveReservedRange(t *testing.T) {
	db := setupMySQLTestDB(t)

	tenantID := insertMySQLTenant(t, db)
	kbID := uuid.New().String()
	knowledgeID := insertMySQLKnowledge(t, db, tenantID, kbID, "seq-owner")
	chunkID := insertMySQLChunk(t, db, tenantID, kbID, knowledgeID, "seq probe")

	var chunkSeqID int64
	require.NoError(t, db.Raw("SELECT seq_id FROM chunks WHERE id = ?", chunkID).Scan(&chunkSeqID).Error)
	assert.GreaterOrEqual(t, chunkSeqID, int64(100000000),
		"generated chunk seq_id must stay out of the range reserved for FAQ import")

	tagID := uuid.New().String()
	require.NoError(t, db.Exec(`
		INSERT INTO knowledge_tags (id, tenant_id, knowledge_base_id, name)
		VALUES (?, ?, ?, 'seq probe tag')
	`, tagID, tenantID, kbID).Error)

	var tagSeqID int64
	require.NoError(t, db.Raw("SELECT seq_id FROM knowledge_tags WHERE id = ?", tagID).Scan(&tagSeqID).Error)
	assert.GreaterOrEqual(t, tagSeqID, int64(10000000))
}

// TestMySQLSessionDefaultsMatchPostgres pins the seeded fallback answer. Its
// value is non-ASCII, so it also catches the schema being applied over a
// connection that is not utf8mb4: MySQL records the DDL-time character set in
// an expression default, and a latin1 connection stores mojibake that no query
// ever complains about.
func TestMySQLSessionDefaultsMatchPostgres(t *testing.T) {
	db := setupMySQLTestDB(t)

	tenantID := insertMySQLTenant(t, db)
	sessionID := uuid.New().String()
	require.NoError(t, db.Exec(
		`INSERT INTO sessions (id, tenant_id) VALUES (?, ?)`, sessionID, tenantID,
	).Error)

	var fallback string
	require.NoError(t, db.Raw(
		`SELECT fallback_response FROM sessions WHERE id = ?`, sessionID,
	).Scan(&fallback).Error)
	assert.Equal(t, "很抱歉，我暂时无法回答这个问题。", fallback)
}

// TestMySQLCaseInsensitiveSearches covers the ILIKE replacements. Each of these
// is a plain syntax error on MySQL if the dialect helper is bypassed.
func TestMySQLCaseInsensitiveSearches(t *testing.T) {
	db := setupMySQLTestDB(t)
	ctx := context.Background()

	tenantID := insertMySQLTenant(t, db)

	userID := uuid.New().String()
	require.NoError(t, db.Exec(`
		INSERT INTO users (id, username, email, password_hash, tenant_id, is_active)
		VALUES (?, 'AliceExample', 'Alice@Example.COM', 'hash', ?, TRUE)
	`, userID, tenantID).Error)

	users, err := NewUserRepository(db).SearchUsers(ctx, "alice", 10)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, userID, users[0].ID)

	sessionID := uuid.New().String()
	require.NoError(t, db.Exec(`
		INSERT INTO sessions (id, tenant_id, title, knowledge_base_id)
		VALUES (?, ?, 'Quarterly Planning', ?)
	`, sessionID, tenantID, uuid.New().String()).Error)

	messageID := uuid.New().String()
	require.NoError(t, db.Exec(`
		INSERT INTO messages (id, session_id, request_id, role, content)
		VALUES (?, ?, ?, 'user', 'Please Summarize The Roadmap')
	`, messageID, sessionID, uuid.New().String()).Error)

	messages, err := NewMessageRepository(db).
		SearchMessagesByKeyword(ctx, tenantID, "summarize the", nil, 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, messageID, messages[0].ID)
}

// TestMySQLTaskQueueClaimAndFail exercises FOR UPDATE SKIP LOCKED (MySQL 8
// supports it, so the dialect gate must open rather than fall through to the
// single-writer SQLite path) and the UPDATE ... RETURNING replacement.
func TestMySQLTaskQueueClaimAndFail(t *testing.T) {
	db := setupMySQLTestDB(t)
	repo := NewTaskPendingOpsRepository(db)
	ctx := context.Background()

	kbID := uuid.New().String()
	for index := 0; index < 3; index++ {
		require.NoError(t, repo.Enqueue(ctx, &types.TaskPendingOp{
			TenantID: 4247,
			TaskType: types.TypeWikiIngest,
			Scope:    types.TaskScopeKnowledgeBase,
			ScopeID:  kbID,
			Op:       "ingest",
			DedupKey: fmt.Sprintf("knowledge-%d", index),
			Payload:  []byte(`{}`),
		}))
	}

	claimed, err := repo.ClaimBatch(
		ctx, types.TypeWikiIngest, types.TaskScopeKnowledgeBase, kbID, 10, time.Now().UTC(),
	)
	require.NoError(t, err)
	require.Len(t, claimed, 3)

	count, err := repo.IncrFailCount(ctx, claimed[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	missing, err := repo.IncrFailCount(ctx, claimed[0].ID+100000)
	require.NoError(t, err, "a missing row must not be an error, the caller treats 0 as already-gone")
	assert.Equal(t, 0, missing)
}
