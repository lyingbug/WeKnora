package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Batch reparse can be driven either by an explicit ID list or by the same
// filter the document list uses. The filter path is what makes "rebuild the 100
// failed files out of 1000" a single click, so these tests pin down how IDs are
// resolved, which rows are skipped, and how the work is split into async tasks.

type stubBatchReparseKBService struct {
	interfaces.KnowledgeBaseService
}

func (s *stubBatchReparseKBService) GetKnowledgeBaseByID(
	_ context.Context, id string,
) (*types.KnowledgeBase, error) {
	return &types.KnowledgeBase{ID: id, TenantID: 1, Type: "document"}, nil
}

type stubBatchReparseKGService struct {
	interfaces.KnowledgeService
	rows      []*types.Knowledge
	pageSizes []int
	listErr   error
}

func (s *stubBatchReparseKGService) ListPagedKnowledgeByKnowledgeBaseID(
	_ context.Context, _ string, page *types.Pagination, _ types.KnowledgeListFilter,
) (*types.PageResult, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	s.pageSizes = append(s.pageSizes, page.PageSize)
	start := (page.Page - 1) * page.PageSize
	if start >= len(s.rows) {
		return types.NewPageResult(int64(len(s.rows)), page, []*types.Knowledge{}), nil
	}
	end := start + page.PageSize
	if end > len(s.rows) {
		end = len(s.rows)
	}
	return types.NewPageResult(int64(len(s.rows)), page, s.rows[start:end]), nil
}

func (s *stubBatchReparseKGService) GetKnowledgeBatch(
	_ context.Context, _ uint64, ids []string,
) ([]*types.Knowledge, error) {
	byID := make(map[string]*types.Knowledge, len(s.rows))
	for _, k := range s.rows {
		byID[k.ID] = k
	}
	out := make([]*types.Knowledge, 0, len(ids))
	for _, id := range ids {
		if k, ok := byID[id]; ok {
			out = append(out, k)
		}
	}
	return out, nil
}

type recordingTaskEnqueuer struct {
	payloads [][]byte
}

func (e *recordingTaskEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	e.payloads = append(e.payloads, task.Payload())
	return &asynq.TaskInfo{ID: fmt.Sprintf("task-%d", len(e.payloads))}, nil
}

func newBatchReparseRouter(
	kg interfaces.KnowledgeService, enqueuer interfaces.TaskEnqueuer,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Next()
	})
	h := &KnowledgeHandler{
		kbService:   &stubBatchReparseKBService{},
		kgService:   kg,
		asynqClient: enqueuer,
	}
	r.POST("/batch-reparse", h.BatchReparseKnowledge)
	return r
}

func postBatchReparse(t *testing.T, router *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/batch-reparse", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeBatchReparseData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	if !resp.Success {
		t.Fatalf("expected success response, got %q", w.Body.String())
	}
	return resp.Data
}

func knowledgeRow(id, status string) *types.Knowledge {
	return &types.Knowledge{ID: id, TenantID: 1, KnowledgeBaseID: "kb-1", ParseStatus: status}
}

func TestNormalizeBatchReparseIDs(t *testing.T) {
	got := normalizeBatchReparseIDs([]string{" a ", "", "b", "a", "  ", "c"})
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("normalizeBatchReparseIDs = %v, want %v", got, want)
	}
	if len(normalizeBatchReparseIDs(nil)) != 0 {
		t.Fatal("normalizeBatchReparseIDs(nil) should be empty")
	}
}

func TestSplitBatchReparseIDs(t *testing.T) {
	ids := make([]string, 450)
	for i := range ids {
		ids[i] = fmt.Sprintf("k-%d", i)
	}
	chunks := splitBatchReparseIDs(ids, batchReparseChunkSize)
	if len(chunks) != 3 {
		t.Fatalf("chunk count = %d, want 3", len(chunks))
	}
	if len(chunks[0]) != 200 || len(chunks[1]) != 200 || len(chunks[2]) != 50 {
		t.Fatalf("chunk sizes = %d/%d/%d, want 200/200/50",
			len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}
	if splitBatchReparseIDs(ids, 0) != nil {
		t.Fatal("non-positive chunk size should yield no chunks")
	}
}

func TestBuildKnowledgeListFilterRejectsEmptySelection(t *testing.T) {
	if _, err := buildKnowledgeListFilter(&batchReparseKnowledgeFilter{}); err == nil {
		t.Fatal("empty filter must be rejected so a stray request cannot rebuild a whole KB")
	}
	if _, err := buildKnowledgeListFilter(&batchReparseKnowledgeFilter{
		ParseStatus: types.ParseStatusFailed,
		StartTime:   "not-a-time",
	}); err == nil {
		t.Fatal("unparsable start_time must be rejected")
	}
	filter, err := buildKnowledgeListFilter(&batchReparseKnowledgeFilter{
		ParseStatus: " failed ",
		TagIDs:      []string{"t1", " t2 "},
		EndTime:     "2026-01-02 03:04:05",
	})
	if err != nil {
		t.Fatalf("valid filter rejected: %v", err)
	}
	if filter.ParseStatus != types.ParseStatusFailed {
		t.Fatalf("parse_status = %q, want %q", filter.ParseStatus, types.ParseStatusFailed)
	}
	if len(filter.TagIDs) != 2 || filter.TagIDs[1] != "t2" {
		t.Fatalf("tag_ids = %v, want [t1 t2]", filter.TagIDs)
	}
	if filter.UpdatedTo.IsZero() {
		t.Fatal("end_time should have been parsed")
	}
}

func TestBuildKnowledgeListFilterFolderScope(t *testing.T) {
	root := ""
	if _, err := buildKnowledgeListFilter(&batchReparseKnowledgeFilter{
		FolderPath: &root, FolderRecursive: true,
	}); err == nil {
		t.Fatal("a recursive filter rooted at the KB covers everything and must be rejected")
	}
	filter, err := buildKnowledgeListFilter(&batchReparseKnowledgeFilter{FolderPath: &root})
	if err != nil {
		t.Fatalf("root-only (non-recursive) filter rejected: %v", err)
	}
	if filter.FolderScope != types.FolderScopeExact {
		t.Fatalf("folder scope = %q, want %q", filter.FolderScope, types.FolderScopeExact)
	}
	nested := "/reports/2026"
	filter, err = buildKnowledgeListFilter(&batchReparseKnowledgeFilter{
		FolderPath: &nested, FolderRecursive: true,
	})
	if err != nil {
		t.Fatalf("nested folder filter rejected: %v", err)
	}
	if filter.FolderScope != types.FolderScopeSubtree || filter.FolderPath == "" {
		t.Fatalf("folder filter = %q/%q, want subtree scope on a non-empty path",
			filter.FolderPath, filter.FolderScope)
	}
}

func TestIsKnowledgeReparseEligible(t *testing.T) {
	cases := []struct {
		knowledge *types.Knowledge
		want      bool
	}{
		{knowledgeRow("k1", types.ParseStatusFailed), true},
		{knowledgeRow("k2", types.ParseStatusCompleted), true},
		{knowledgeRow("k3", types.ParseStatusCancelled), true},
		{knowledgeRow("k4", types.ParseStatusPending), false},
		{knowledgeRow("k5", types.ParseStatusProcessing), false},
		{knowledgeRow("k6", types.ParseStatusFinalizing), false},
		{knowledgeRow("k7", types.ParseStatusDeleting), false},
		{
			&types.Knowledge{
				ID:          "k8",
				Type:        types.KnowledgeTypeManual,
				ParseStatus: types.ManualKnowledgeStatusDraft,
			},
			false,
		},
		{nil, false},
	}
	for _, tc := range cases {
		if got := isKnowledgeReparseEligible(tc.knowledge); got != tc.want {
			t.Fatalf("isKnowledgeReparseEligible(%+v) = %v, want %v", tc.knowledge, got, tc.want)
		}
	}
}

// The headline scenario from the feature request: a KB with far more than one
// page of failed documents is rebuilt from a single filtered request, without
// the browser ever enumerating the IDs.
func TestBatchReparseKnowledgeResolvesFilterAcrossPages(t *testing.T) {
	rows := make([]*types.Knowledge, 0, 250)
	for i := 0; i < 250; i++ {
		status := types.ParseStatusFailed
		if i%50 == 0 {
			status = types.ParseStatusProcessing
		}
		rows = append(rows, knowledgeRow(fmt.Sprintf("k-%d", i), status))
	}
	kg := &stubBatchReparseKGService{rows: rows}
	enqueuer := &recordingTaskEnqueuer{}
	router := newBatchReparseRouter(kg, enqueuer)

	w := postBatchReparse(t, router, map[string]any{
		"kb_id":  "kb-1",
		"filter": map[string]any{"parse_status": types.ParseStatusFailed},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	data := decodeBatchReparseData(t, w)
	if got := data["submitted_count"].(float64); got != 245 {
		t.Fatalf("submitted_count = %v, want 245", got)
	}
	if got := data["skipped_in_flight_count"].(float64); got != 5 {
		t.Fatalf("skipped_in_flight_count = %v, want 5", got)
	}
	if got := len(data["task_ids"].([]any)); got != 2 {
		t.Fatalf("task count = %d, want 2 (245 ids split by %d)", got, batchReparseChunkSize)
	}

	total := 0
	for _, raw := range enqueuer.payloads {
		var payload types.KnowledgeListReparsePayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(payload.KnowledgeIDs) > batchReparseChunkSize {
			t.Fatalf("task carries %d ids, exceeding the %d cap",
				len(payload.KnowledgeIDs), batchReparseChunkSize)
		}
		for _, id := range payload.KnowledgeIDs {
			if strings.HasSuffix(id, "-0") || id == "k-50" || id == "k-100" {
				t.Fatalf("in-flight knowledge %s must not be resubmitted", id)
			}
		}
		total += len(payload.KnowledgeIDs)
	}
	if total != 245 {
		t.Fatalf("enqueued %d ids in total, want 245", total)
	}
}

func TestBatchReparseKnowledgeFilterRequestValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]any
		rows       []*types.Knowledge
		wantStatus int
		wantBody   string
	}{
		{
			name: "ids and filter are mutually exclusive",
			body: map[string]any{
				"kb_id": "kb-1", "ids": []string{"k-1"},
				"filter": map[string]any{"parse_status": types.ParseStatusFailed},
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "mutually exclusive",
		},
		{
			name:       "neither ids nor filter",
			body:       map[string]any{"kb_id": "kb-1"},
			wantStatus: http.StatusBadRequest,
			wantBody:   "no knowledge IDs provided",
		},
		{
			name:       "filter without any condition",
			body:       map[string]any{"kb_id": "kb-1", "filter": map[string]any{}},
			wantStatus: http.StatusBadRequest,
			wantBody:   "at least one condition",
		},
		{
			name: "filter matching nothing",
			body: map[string]any{
				"kb_id":  "kb-1",
				"filter": map[string]any{"parse_status": types.ParseStatusFailed},
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "no knowledge matches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newBatchReparseRouter(
				&stubBatchReparseKGService{rows: tt.rows}, &recordingTaskEnqueuer{})
			w := postBatchReparse(t, router, tt.body)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Fatalf("body %q does not contain %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

// Every match still parsing means there is nothing safe to submit; the request
// succeeds with a zero count instead of clearing content mid-pipeline.
func TestBatchReparseKnowledgeSkipsWhenAllMatchesInFlight(t *testing.T) {
	kg := &stubBatchReparseKGService{rows: []*types.Knowledge{
		knowledgeRow("k-1", types.ParseStatusProcessing),
		knowledgeRow("k-2", types.ParseStatusPending),
	}}
	enqueuer := &recordingTaskEnqueuer{}
	router := newBatchReparseRouter(kg, enqueuer)

	w := postBatchReparse(t, router, map[string]any{
		"kb_id":  "kb-1",
		"filter": map[string]any{"keyword": "report"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	data := decodeBatchReparseData(t, w)
	if got := data["submitted_count"].(float64); got != 0 {
		t.Fatalf("submitted_count = %v, want 0", got)
	}
	if got := data["skipped_in_flight_count"].(float64); got != 2 {
		t.Fatalf("skipped_in_flight_count = %v, want 2", got)
	}
	if len(enqueuer.payloads) != 0 {
		t.Fatalf("enqueued %d tasks, want 0", len(enqueuer.payloads))
	}
}

// The pre-existing ID-list contract must keep working unchanged.
func TestBatchReparseKnowledgeByExplicitIDs(t *testing.T) {
	kg := &stubBatchReparseKGService{rows: []*types.Knowledge{
		knowledgeRow("k-1", types.ParseStatusFailed),
		knowledgeRow("k-2", types.ParseStatusCompleted),
	}}
	enqueuer := &recordingTaskEnqueuer{}
	router := newBatchReparseRouter(kg, enqueuer)

	w := postBatchReparse(t, router, map[string]any{
		"kb_id": "kb-1",
		"ids":   []string{"k-1", "k-2", "k-1"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	data := decodeBatchReparseData(t, w)
	if got := data["reparse_count"].(float64); got != 2 {
		t.Fatalf("reparse_count = %v, want 2 (duplicates removed)", got)
	}
	if data["task_id"].(string) == "" {
		t.Fatal("task_id must stay populated for existing clients")
	}
	if len(enqueuer.payloads) != 1 {
		t.Fatalf("enqueued %d tasks, want 1", len(enqueuer.payloads))
	}
}

func TestBatchReparseKnowledgeRejectsUnknownIDs(t *testing.T) {
	kg := &stubBatchReparseKGService{rows: []*types.Knowledge{
		knowledgeRow("k-1", types.ParseStatusFailed),
	}}
	router := newBatchReparseRouter(kg, &recordingTaskEnqueuer{})
	w := postBatchReparse(t, router, map[string]any{
		"kb_id": "kb-1",
		"ids":   []string{"k-1", "missing"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}
