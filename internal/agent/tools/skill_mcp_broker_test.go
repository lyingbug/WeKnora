package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent/approval"
	"github.com/Tencent/WeKnora/internal/mcp"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillMCPBrokerCallTool(t *testing.T) {
	ctx := context.Background()
	client := &fakeBrokerMCPClient{
		result: &mcp.CallToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "ok"}},
		},
	}
	broker := NewSkillMCPBroker(
		fakeBrokerMCPServiceGetter{service: &types.MCPService{ID: "svc-1", TenantID: 10, Name: "github", Enabled: true}},
		fakeBrokerMCPClientProvider{client: client},
		nil,
	)
	require.NoError(t, broker.Start(ctx))
	defer broker.Shutdown(ctx)

	registration, err := broker.Register(ctx, SkillMCPBrokerRegistration{
		TenantID: 10,
		Bindings: map[string]string{
			"github": "svc-1",
		},
	})
	require.NoError(t, err)
	defer registration.Cleanup()

	body := bytes.NewBufferString(`{"alias":"github","tool":"search","args":{"q":"weknora"}}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registration.URL+"/v1/tools/call", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+registration.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got mcp.CallToolResult
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "ok", got.Content[0].Text)
	assert.Equal(t, "search", client.toolName)
	assert.Equal(t, "weknora", client.args["q"])
}

func TestSkillMCPBrokerRejectsBadToken(t *testing.T) {
	ctx := context.Background()
	broker := NewSkillMCPBroker(fakeBrokerMCPServiceGetter{}, fakeBrokerMCPClientProvider{}, nil)
	require.NoError(t, broker.Start(ctx))
	defer broker.Shutdown(ctx)

	body := bytes.NewBufferString(`{"alias":"github","tool":"search","args":{}}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, broker.URL()+"/v1/tools/call", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer missing")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSkillMCPBrokerRejectsApprovalRequiredTool(t *testing.T) {
	ctx := context.Background()
	broker := NewSkillMCPBroker(
		fakeBrokerMCPServiceGetter{service: &types.MCPService{ID: "svc-1", TenantID: 10, Name: "github", Enabled: true}},
		fakeBrokerMCPClientProvider{client: &fakeBrokerMCPClient{}},
		fakeBrokerApproval{needs: true},
	)
	require.NoError(t, broker.Start(ctx))
	defer broker.Shutdown(ctx)

	registration, err := broker.Register(ctx, SkillMCPBrokerRegistration{
		TenantID: 10,
		Bindings: map[string]string{"github": "svc-1"},
	})
	require.NoError(t, err)
	defer registration.Cleanup()

	body := bytes.NewBufferString(`{"alias":"github","tool":"delete","args":{}}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registration.URL+"/v1/tools/call", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+registration.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

type fakeBrokerMCPServiceGetter struct {
	service *types.MCPService
	err     error
}

func (f fakeBrokerMCPServiceGetter) GetMCPServiceByID(context.Context, uint64, string) (*types.MCPService, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.service, nil
}

type fakeBrokerMCPClientProvider struct {
	client mcp.MCPClient
	err    error
}

func (f fakeBrokerMCPClientProvider) GetOrCreateClient(*types.MCPService) (mcp.MCPClient, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.client, nil
}

type fakeBrokerApproval struct {
	needs bool
}

func (f fakeBrokerApproval) NeedsApproval(context.Context, uint64, string, string) bool {
	return f.needs
}

func (f fakeBrokerApproval) RequestAndWait(context.Context, approval.PendingRequest) (approval.Decision, error) {
	return approval.Decision{}, nil
}

type fakeBrokerMCPClient struct {
	toolName string
	args     map[string]interface{}
	result   *mcp.CallToolResult
}

func (f *fakeBrokerMCPClient) Connect(context.Context) error { return nil }
func (f *fakeBrokerMCPClient) Disconnect() error             { return nil }
func (f *fakeBrokerMCPClient) Initialize(context.Context) (*mcp.InitializeResult, error) {
	return nil, nil
}
func (f *fakeBrokerMCPClient) ListTools(context.Context) ([]*types.MCPTool, error) {
	return nil, nil
}
func (f *fakeBrokerMCPClient) ListResources(context.Context) ([]*types.MCPResource, error) {
	return nil, nil
}
func (f *fakeBrokerMCPClient) CallTool(_ context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	f.toolName = name
	f.args = args
	if f.result == nil {
		return &mcp.CallToolResult{}, nil
	}
	return f.result, nil
}
func (f *fakeBrokerMCPClient) ReadResource(context.Context, string) (*mcp.ReadResourceResult, error) {
	return nil, nil
}
func (f *fakeBrokerMCPClient) IsConnected() bool { return true }
func (f *fakeBrokerMCPClient) GetServiceID() string {
	return "svc-1"
}
