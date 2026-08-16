package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/plugin/protocol"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// httpProvider is a fallback for a search API that already exists as a
// remote service. New plugins should use stdio (JSON-RPC), not start HTTP.
// The endpoint comes from plugin.yaml (trusted); user query is JSON only.
type httpProvider struct {
	name     string
	endpoint string
	client   *http.Client
	params   types.WebSearchProviderParameters
}

func newHTTPProvider(name, endpoint string, timeout time.Duration) (*httpProvider, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("plugin: invalid http endpoint %q", endpoint)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("plugin: http endpoint must be http(s)")
	}
	return &httpProvider{
		name:     name,
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   &http.Client{Timeout: timeout},
	}, nil
}

func (p *httpProvider) withParams(params types.WebSearchProviderParameters) *httpProvider {
	cp := *p
	cp.params = params
	return &cp
}

func (p *httpProvider) Name() string { return p.name }

func (p *httpProvider) Search(
	ctx context.Context, query string, maxResults int, includeDate bool,
) ([]*types.WebSearchResult, error) {
	return p.search(ctx, query, maxResults, includeDate, p.params)
}

func (p *httpProvider) search(
	ctx context.Context, query string, maxResults int, includeDate bool,
	params types.WebSearchProviderParameters,
) ([]*types.WebSearchResult, error) {
	body, err := json.Marshal(protocol.SearchRequest{
		Query: query, MaxResults: maxResults, IncludeDate: includeDate, Parameters: params,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: http: %w", p.name, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("plugin %s: http status %d: %s", p.name, resp.StatusCode, truncate(raw, 200))
	}
	var out SearchResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("plugin %s: decode: %w", p.name, err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("plugin %s: %s", p.name, out.Error)
	}
	return out.Results, nil
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

var _ interfaces.WebSearchProvider = (*httpProvider)(nil)
