package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSkillService struct {
	installTenantID    uint64
	installPackagePath string
	installUserID      string
	installPermissions types.JSON
	installEntry       *types.SkillRegistryEntry
	installs           []*types.TenantSkillInstallInfo
	setEnabledTenantID uint64
	setEnabledSkillID  string
	setEnabled         bool
}

type mockSkillExecutionRunRepo struct {
	tenantID uint64
	limit    int
	runs     []*types.SkillExecutionRun
}

func (m *mockSkillExecutionRunRepo) CreateSkillExecutionRun(context.Context, *types.SkillExecutionRun) error {
	return nil
}

func (m *mockSkillExecutionRunRepo) ListSkillExecutionRuns(_ context.Context, tenantID uint64, limit int) ([]*types.SkillExecutionRun, error) {
	m.tenantID = tenantID
	m.limit = limit
	return m.runs, nil
}

func (m *mockSkillService) ListPreloadedSkills(context.Context) ([]*skills.SkillMetadata, error) {
	return nil, nil
}

func (m *mockSkillService) ListTenantSkills(context.Context, uint64) ([]*skills.SkillMetadata, error) {
	return nil, nil
}

func (m *mockSkillService) ListTenantSkillInstalls(context.Context, uint64) ([]*types.TenantSkillInstallInfo, error) {
	return m.installs, nil
}

func (m *mockSkillService) SetTenantSkillEnabled(_ context.Context, tenantID uint64, skillID string, enabled bool) error {
	m.setEnabledTenantID = tenantID
	m.setEnabledSkillID = skillID
	m.setEnabled = enabled
	return nil
}

func (m *mockSkillService) ImportPreloadedSkills(context.Context) error {
	return nil
}

func (m *mockSkillService) EnsureTenantPreloadedSkillInstalls(context.Context, uint64) error {
	return nil
}

func (m *mockSkillService) InstallLocalSkillPackage(
	_ context.Context,
	tenantID uint64,
	packagePath string,
	installedBy string,
) (*types.SkillRegistryEntry, error) {
	return m.InstallLocalSkillPackageWithPermissions(context.Background(), tenantID, packagePath, installedBy, nil)
}

func (m *mockSkillService) InstallLocalSkillPackageWithPermissions(
	_ context.Context,
	tenantID uint64,
	packagePath string,
	installedBy string,
	approvedPermissions types.JSON,
) (*types.SkillRegistryEntry, error) {
	m.installTenantID = tenantID
	m.installPackagePath = packagePath
	m.installUserID = installedBy
	m.installPermissions = approvedPermissions
	return m.installEntry, nil
}

func (m *mockSkillService) SyncAgentSkillBindings(context.Context, uint64, string, string, []string) error {
	return nil
}

func (m *mockSkillService) ResolveAgentSkillAccess(context.Context, uint64, string, string, []string) ([]string, []string, error) {
	return nil, nil, nil
}

func (m *mockSkillService) ResolveAgentSelectedSkills(context.Context, uint64, string, string, []string) ([]string, error) {
	return nil, nil
}

func (m *mockSkillService) GetSkillByName(context.Context, string) (*skills.Skill, error) {
	return nil, nil
}

func TestSkillHandler_InstallLocalSkillPackage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &mockSkillService{
		installEntry: &types.SkillRegistryEntry{
			ID:          "local-sample-skill-1-0-0",
			Name:        "sample-skill",
			Version:     "1.0.0",
			Description: "Sample skill",
			SourceType:  types.SkillSourceTypeLocal,
		},
	}
	h := NewSkillHandler(svc, nil)
	r := gin.New()
	r.POST("/skills/install-local", func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(10))
		c.Set(types.UserIDContextKey.String(), "user-a")
		h.InstallLocalSkillPackage(c)
	})

	body := bytes.NewBufferString(`{"package_path":"sample-skill","approved_permissions":{"network":[]}}`)
	req := httptest.NewRequest(http.MethodPost, "/skills/install-local", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint64(10), svc.installTenantID)
	assert.Equal(t, "sample-skill", svc.installPackagePath)
	assert.Equal(t, "user-a", svc.installUserID)
	assert.JSONEq(t, `{"network":[]}`, svc.installPermissions.ToString())

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, true, got["success"])
	data, ok := got["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "sample-skill", data["name"])
	assert.Equal(t, types.SkillSourceTypeLocal, data["source_type"])
}

func TestSkillHandler_InstallLocalSkillPackage_RequiresPackagePath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewSkillHandler(&mockSkillService{}, nil)
	r := gin.New()
	r.POST("/skills/install-local", h.InstallLocalSkillPackage)

	req := httptest.NewRequest(http.MethodPost, "/skills/install-local", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "package_path is required")
}

func TestSkillHandler_ListInstalledSkills(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &mockSkillService{
		installs: []*types.TenantSkillInstallInfo{
			{
				SkillID:             "preloaded-alpha-0-0-0",
				Name:                "alpha",
				Version:             "0.0.0",
				Description:         "Alpha skill",
				SourceType:          types.SkillSourceTypePreloaded,
				Enabled:             false,
				InstalledBy:         "user-a",
				ApprovedPermissions: types.JSON(`{"network":[]}`),
				IsBuiltin:           true,
			},
		},
	}
	h := NewSkillHandler(svc, nil)
	r := gin.New()
	r.GET("/skills/installed", func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(10))
		h.ListInstalledSkills(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/skills/installed", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"enabled":false`)
	assert.Contains(t, w.Body.String(), `"name":"alpha"`)
}

func TestSkillHandler_UpdateTenantSkillInstall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &mockSkillService{}
	h := NewSkillHandler(svc, nil)
	r := gin.New()
	r.PATCH("/skills/:skill_id", func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(10))
		h.UpdateTenantSkillInstall(c)
	})

	req := httptest.NewRequest(http.MethodPatch, "/skills/preloaded-alpha-0-0-0", bytes.NewBufferString(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint64(10), svc.setEnabledTenantID)
	assert.Equal(t, "preloaded-alpha-0-0-0", svc.setEnabledSkillID)
	assert.False(t, svc.setEnabled)
}

func TestSkillHandler_UpdateTenantSkillInstall_RequiresEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewSkillHandler(&mockSkillService{}, nil)
	r := gin.New()
	r.PATCH("/skills/:skill_id", h.UpdateTenantSkillInstall)

	req := httptest.NewRequest(http.MethodPatch, "/skills/preloaded-alpha-0-0-0", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "enabled is required")
}

func TestSkillHandler_ListSkillExecutionRuns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runRepo := &mockSkillExecutionRunRepo{
		runs: []*types.SkillExecutionRun{
			{
				ID:            "run-1",
				TenantID:      10,
				UserID:        "user-a",
				AgentID:       "agent-a",
				SessionID:     "session-a",
				MessageID:     "message-a",
				ToolCallID:    "call-a",
				SkillID:       "alpha",
				ScriptPath:    "scripts/run.py",
				Status:        "failed",
				DurationMS:    42,
				ResourceUsage: types.JSON(`{"exit_code":1}`),
				ErrorSummary:  "boom",
				CreatedAt:     time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC),
			},
		},
	}
	h := NewSkillHandler(&mockSkillService{}, runRepo)
	r := gin.New()
	r.GET("/skills/runs", func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(10))
		h.ListSkillExecutionRuns(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/skills/runs?limit=20", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint64(10), runRepo.tenantID)
	assert.Equal(t, 20, runRepo.limit)
	assert.Contains(t, w.Body.String(), `"skill_id":"alpha"`)
	assert.Contains(t, w.Body.String(), `"resource_usage":{"exit_code":1}`)
	assert.Contains(t, w.Body.String(), `"created_at":"2026-06-06T01:02:03Z"`)
}

func TestSkillHandler_ListSkillExecutionRuns_RejectsBadLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewSkillHandler(&mockSkillService{}, &mockSkillExecutionRunRepo{})
	r := gin.New()
	r.GET("/skills/runs", h.ListSkillExecutionRuns)

	req := httptest.NewRequest(http.MethodGet, "/skills/runs?limit=bad", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "limit must be a positive integer")
}
