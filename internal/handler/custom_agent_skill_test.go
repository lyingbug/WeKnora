package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCustomAgentSkillService struct {
	gotAgentID string
	gotMode    string
	gotSkills  []string
	config     *types.AgentSkillConfig
	updateErr  error
}

func (m *mockCustomAgentSkillService) CreateAgent(context.Context, *types.CustomAgent) (*types.CustomAgent, error) {
	return nil, nil
}

func (m *mockCustomAgentSkillService) GetAgentByID(context.Context, string) (*types.CustomAgent, error) {
	return nil, nil
}

func (m *mockCustomAgentSkillService) GetAgentByIDAndTenant(context.Context, string, uint64) (*types.CustomAgent, error) {
	return nil, nil
}

func (m *mockCustomAgentSkillService) ListAgents(context.Context) ([]*types.CustomAgent, error) {
	return nil, nil
}

func (m *mockCustomAgentSkillService) UpdateAgent(context.Context, *types.CustomAgent) (*types.CustomAgent, error) {
	return nil, nil
}

func (m *mockCustomAgentSkillService) GetAgentSkillConfig(_ context.Context, agentID string) (*types.AgentSkillConfig, error) {
	m.gotAgentID = agentID
	return m.config, nil
}

func (m *mockCustomAgentSkillService) UpdateAgentSkillConfig(
	_ context.Context,
	agentID string,
	mode string,
	selectedSkills []string,
) (*types.AgentSkillConfig, error) {
	m.gotAgentID = agentID
	m.gotMode = mode
	m.gotSkills = selectedSkills
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return m.config, nil
}

func (m *mockCustomAgentSkillService) DeleteAgent(context.Context, string) error {
	return nil
}

func (m *mockCustomAgentSkillService) CopyAgent(context.Context, string) (*types.CustomAgent, error) {
	return nil, nil
}

func (m *mockCustomAgentSkillService) GetSuggestedQuestions(context.Context, string, []string, []string, int) ([]types.SuggestedQuestion, error) {
	return nil, nil
}

func TestCustomAgentHandler_GetAgentSkills(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &mockCustomAgentSkillService{
		config: &types.AgentSkillConfig{
			AgentID:        "agent-a",
			Mode:           "selected",
			SelectedSkills: []string{"alpha"},
		},
	}
	h := NewCustomAgentHandler(svc, nil, nil)
	r := gin.New()
	r.GET("/agents/:id/skills", h.GetAgentSkills)

	req := httptest.NewRequest(http.MethodGet, "/agents/agent-a/skills", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "agent-a", svc.gotAgentID)
	assert.Contains(t, w.Body.String(), `"mode":"selected"`)
	assert.Contains(t, w.Body.String(), `"selected_skills":["alpha"]`)
}

func TestCustomAgentHandler_UpdateAgentSkills(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &mockCustomAgentSkillService{
		config: &types.AgentSkillConfig{
			AgentID:        "agent-a",
			Mode:           "selected",
			SelectedSkills: []string{"alpha", "beta"},
		},
	}
	h := NewCustomAgentHandler(svc, nil, nil)
	r := gin.New()
	r.PUT("/agents/:id/skills", h.UpdateAgentSkills)

	body := bytes.NewBufferString(`{"mode":"selected","selected_skills":["alpha","beta"]}`)
	req := httptest.NewRequest(http.MethodPut, "/agents/agent-a/skills", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "agent-a", svc.gotAgentID)
	assert.Equal(t, "selected", svc.gotMode)
	assert.Equal(t, []string{"alpha", "beta"}, svc.gotSkills)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, true, got["success"])
}

func TestCustomAgentHandler_UpdateAgentSkills_InvalidMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &mockCustomAgentSkillService{updateErr: service.ErrInvalidSkillMode}
	h := NewCustomAgentHandler(svc, nil, nil)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.PUT("/agents/:id/skills", h.UpdateAgentSkills)

	body := bytes.NewBufferString(`{"mode":"bad-mode"}`)
	req := httptest.NewRequest(http.MethodPut, "/agents/agent-a/skills", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid skill selection mode")
}
