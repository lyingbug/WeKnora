package handler

import (
	"bytes"
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const feedbackReasonTestTenantID = uint64(1)

// setupFeedbackReasonHTTPStack wires the real feedback routes on top of a SQLite
// database migrated with migrations/sqlite/000079_chunk_feedback.up.sql, so the
// test exercises handler, service, repositories and schema together.
func setupFeedbackReasonHTTPStack(t *testing.T) (*gin.Engine, *service.ChunkFeedbackService) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.Exec(`
		CREATE TABLE sessions (
			id varchar(36) PRIMARY KEY,
			tenant_id integer NOT NULL,
			user_id varchar(512),
			deleted_at datetime
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE messages (
			id varchar(36) PRIMARY KEY,
			session_id varchar(36),
			role text,
			is_completed boolean NOT NULL DEFAULT false,
			knowledge_references text,
			updated_at datetime,
			deleted_at datetime
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE chunks (
			id varchar(36) PRIMARY KEY,
			tenant_id integer NOT NULL,
			knowledge_base_id varchar(36),
			knowledge_id varchar(36),
			content text,
			updated_at datetime,
			deleted_at datetime
		)
	`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO sessions (id, tenant_id, user_id) VALUES ('session-1', 1, 'user-1')`,
	).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO messages (id, session_id, role, is_completed, knowledge_references)
		VALUES ('message-1', 'session-1', 'assistant', true, '[{"id":"chunk-1","knowledge_base_id":"kb-1"}]')
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO chunks (id, tenant_id, knowledge_base_id, knowledge_id, content, updated_at)
		VALUES ('chunk-1', 1, 'kb-1', 'knowledge-1', 'chunk content', CURRENT_TIMESTAMP)
	`).Error)

	migrationSQL, err := os.ReadFile("../../migrations/sqlite/000079_chunk_feedback.up.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(migrationSQL)).Error)

	feedbackService := service.NewChunkFeedbackServiceWithUnitOfWork(
		repository.NewQAReplyChunkRefRepository(db),
		repository.NewChunkFeedbackRepository(db),
		repository.NewMessageRepository(db),
		repository.NewChunkRepository(db),
		repository.NewChunkWeightLogRepository(db),
		repository.NewChunkFeedbackUnitOfWork(db),
	)
	handler := NewChunkFeedbackHandler(feedbackService)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), feedbackReasonTestTenantID)
		c.Set(types.UserIDContextKey.String(), "user-1")
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		status := http.StatusInternalServerError
		var appErr *errors.AppError
		if stdErrors.As(c.Errors.Last().Err, &appErr) {
			status = appErr.HTTPCode
		}
		c.JSON(status, gin.H{"success": false, "error": c.Errors.Last().Err.Error()})
	})
	engine.POST("/feedback", handler.SubmitFeedback)
	engine.GET("/feedback/user-feedback", handler.GetUserFeedback)
	engine.GET("/feedback/dislike-reasons", handler.GetDislikeReasons)

	return engine, feedbackService
}

func postFeedback(t *testing.T, engine *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/feedback", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestFeedbackHTTPRejectsFreeFormDislikeReasonCode(t *testing.T) {
	engine, _ := setupFeedbackReasonHTTPStack(t)

	w := postFeedback(t, engine, map[string]any{
		"message_id":     "message-1",
		"is_positive":    false,
		"dislike_reason": "这段引用完全对不上我的问题",
	})

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "predefined reason codes")
}

func TestFeedbackHTTPStoresReasonCodeAndDetailSeparately(t *testing.T) {
	engine, feedbackService := setupFeedbackReasonHTTPStack(t)

	w := postFeedback(t, engine, map[string]any{
		"message_id":            "message-1",
		"is_positive":           false,
		"dislike_reason":        "other",
		"dislike_reason_detail": "引用的片段是旧版本文档",
	})
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(
		http.MethodGet, "/feedback/user-feedback?message_id=message-1", nil,
	))
	require.Equal(t, http.StatusOK, w.Code)

	var stored struct {
		Data types.UserFeedbackResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stored))
	require.Equal(t, string(types.DislikeReasonOther), stored.Data.DislikeReason)
	require.Equal(t, "引用的片段是旧版本文档", stored.Data.DislikeReasonDetail)

	// The free-form note must stay out of the chunk-level aggregate.
	stats, err := feedbackService.GetChunkStats(
		t.Context(), feedbackReasonTestTenantID, "chunk-1",
	)
	require.NoError(t, err)
	require.Equal(t, []string{string(types.DislikeReasonOther)}, stats.DislikeReasons)
	require.Equal(t,
		[]types.DislikeReasonStat{{Reason: string(types.DislikeReasonOther), Count: 1}},
		stats.DislikeReasonStats,
	)
}

func TestFeedbackHTTPNormalizesLegacyLabelFromOlderClients(t *testing.T) {
	engine, _ := setupFeedbackReasonHTTPStack(t)

	w := postFeedback(t, engine, map[string]any{
		"message_id":     "message-1",
		"is_positive":    false,
		"dislike_reason": "与问题不相关",
	})
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(
		http.MethodGet, "/feedback/user-feedback?message_id=message-1", nil,
	))
	var stored struct {
		Data types.UserFeedbackResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stored))
	require.Equal(t, string(types.DislikeReasonIrrelevant), stored.Data.DislikeReason)
}

func TestFeedbackHTTPDislikeReasonsExposeCodesForLocalization(t *testing.T) {
	engine, _ := setupFeedbackReasonHTTPStack(t)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/feedback/dislike-reasons", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []types.DislikeReasonOption `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, len(types.AllDislikeReasons()))
	for i, reason := range types.AllDislikeReasons() {
		require.Equal(t, reason, response.Data[i].Code)
		require.NotEmpty(t, response.Data[i].Label)
	}
}
