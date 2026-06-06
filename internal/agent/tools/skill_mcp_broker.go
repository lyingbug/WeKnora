package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/agent/approval"
	"github.com/Tencent/WeKnora/internal/mcp"
	"github.com/Tencent/WeKnora/internal/types"
)

type SkillMCPServiceGetter interface {
	GetMCPServiceByID(ctx context.Context, tenantID uint64, id string) (*types.MCPService, error)
}

type SkillMCPClientProvider interface {
	GetOrCreateClient(service *types.MCPService) (mcp.MCPClient, error)
}

type SkillMCPBroker struct {
	serviceGetter  SkillMCPServiceGetter
	clientProvider SkillMCPClientProvider
	approval       approval.MCPApproval

	mu       sync.RWMutex
	sessions map[string]skillMCPBrokerSession
	server   *http.Server
	listener net.Listener
	url      string
}

type SkillMCPBrokerRegistration struct {
	TenantID uint64
	Bindings map[string]string
}

type SkillMCPBrokerSession struct {
	URL     string
	Token   string
	Cleanup func()
}

type skillMCPBrokerSession struct {
	tenantID uint64
	bindings map[string]string
}

type skillMCPCallRequest struct {
	Alias string                 `json:"alias"`
	Tool  string                 `json:"tool"`
	Args  map[string]interface{} `json:"args"`
}

func NewSkillMCPBroker(
	serviceGetter SkillMCPServiceGetter,
	clientProvider SkillMCPClientProvider,
	approvalGate approval.MCPApproval,
) *SkillMCPBroker {
	return &SkillMCPBroker{
		serviceGetter:  serviceGetter,
		clientProvider: clientProvider,
		approval:       approvalGate,
		sessions:       make(map[string]skillMCPBrokerSession),
	}
}

func (b *SkillMCPBroker) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.server != nil {
		return nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to start skill mcp broker: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tools/call", b.handleCallTool)
	b.server = &http.Server{Handler: mux}
	b.listener = listener
	b.url = "http://" + listener.Addr().String()
	go func() {
		_ = b.server.Serve(listener)
	}()
	go func() {
		<-ctx.Done()
		_ = b.Shutdown(context.Background())
	}()
	return nil
}

func (b *SkillMCPBroker) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.server == nil {
		return nil
	}
	err := b.server.Shutdown(ctx)
	b.server = nil
	b.listener = nil
	b.url = ""
	b.sessions = make(map[string]skillMCPBrokerSession)
	return err
}

func (b *SkillMCPBroker) URL() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.url
}

func (b *SkillMCPBroker) Register(ctx context.Context, registration SkillMCPBrokerRegistration) (*SkillMCPBrokerSession, error) {
	if registration.TenantID == 0 {
		return nil, fmt.Errorf("tenant ID is required")
	}
	if len(registration.Bindings) == 0 {
		return nil, fmt.Errorf("mcp bindings are required")
	}
	if err := b.Start(ctx); err != nil {
		return nil, err
	}
	token, err := randomBrokerToken()
	if err != nil {
		return nil, err
	}
	bindings := make(map[string]string, len(registration.Bindings))
	for alias, serviceID := range registration.Bindings {
		alias = strings.TrimSpace(alias)
		serviceID = strings.TrimSpace(serviceID)
		if alias == "" || serviceID == "" {
			return nil, fmt.Errorf("mcp bindings must not contain empty alias or service id")
		}
		bindings[alias] = serviceID
	}

	b.mu.Lock()
	b.sessions[token] = skillMCPBrokerSession{tenantID: registration.TenantID, bindings: bindings}
	url := b.url
	b.mu.Unlock()

	cleanup := func() {
		b.mu.Lock()
		delete(b.sessions, token)
		b.mu.Unlock()
	}
	return &SkillMCPBrokerSession{URL: url, Token: token, Cleanup: cleanup}, nil
}

func (b *SkillMCPBroker) handleCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	session, ok := b.session(token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req skillMCPCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	serviceID, ok := session.bindings[strings.TrimSpace(req.Alias)]
	if !ok {
		http.Error(w, "mcp alias is not bound", http.StatusForbidden)
		return
	}
	req.Tool = strings.TrimSpace(req.Tool)
	if req.Tool == "" {
		http.Error(w, "tool is required", http.StatusBadRequest)
		return
	}

	service, err := b.serviceGetter.GetMCPServiceByID(r.Context(), session.tenantID, serviceID)
	if err != nil {
		http.Error(w, "mcp service not found", http.StatusNotFound)
		return
	}
	if service == nil || !service.Enabled {
		http.Error(w, "mcp service is not enabled", http.StatusForbidden)
		return
	}
	if b.approval != nil && b.approval.NeedsApproval(r.Context(), session.tenantID, service.ID, req.Tool) {
		http.Error(w, "mcp tool requires user approval and cannot be called from skill broker", http.StatusForbidden)
		return
	}
	client, err := b.clientProvider.GetOrCreateClient(service)
	if err != nil {
		http.Error(w, "failed to connect to mcp service", http.StatusBadGateway)
		return
	}
	if req.Args == nil {
		req.Args = map[string]interface{}{}
	}
	result, err := client.CallTool(r.Context(), req.Tool, req.Args)
	if err != nil {
		http.Error(w, "mcp tool call failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (b *SkillMCPBroker) session(token string) (skillMCPBrokerSession, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	session, ok := b.sessions[token]
	return session, ok
}

func randomBrokerToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
