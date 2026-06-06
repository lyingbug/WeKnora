package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	installEntry       *types.SkillRegistryEntry
}

func (m *mockSkillService) ListPreloadedSkills(context.Context) ([]*skills.SkillMetadata, error) {
	return nil, nil
}

func (m *mockSkillService) ListTenantSkills(context.Context, uint64) ([]*skills.SkillMetadata, error) {
	return nil, nil
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
	m.installTenantID = tenantID
	m.installPackagePath = packagePath
	m.installUserID = installedBy
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
	h := NewSkillHandler(svc)
	r := gin.New()
	r.POST("/skills/install-local", func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(10))
		c.Set(types.UserIDContextKey.String(), "user-a")
		h.InstallLocalSkillPackage(c)
	})

	body := bytes.NewBufferString(`{"package_path":"sample-skill"}`)
	req := httptest.NewRequest(http.MethodPost, "/skills/install-local", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint64(10), svc.installTenantID)
	assert.Equal(t, "sample-skill", svc.installPackagePath)
	assert.Equal(t, "user-a", svc.installUserID)

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

	h := NewSkillHandler(&mockSkillService{})
	r := gin.New()
	r.POST("/skills/install-local", h.InstallLocalSkillPackage)

	req := httptest.NewRequest(http.MethodPost, "/skills/install-local", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "package_path is required")
}
