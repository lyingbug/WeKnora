package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/tidwall/gjson"
)

// The HTTP runtime executes a declarative manifest: build a request from the
// manifest plus configuration plus input, send it, and read the reply into the
// records the domain expects.
//
// It is the reason most plugins need no code. What used to be a Go file per
// vendor — build a struct, set two headers, unmarshal into another struct,
// loop over results — is the same four steps every time, so it is written once
// here and described per vendor in YAML.

// maxResponseBytes bounds a reply. A plugin endpoint is operator-configured
// and may be a third-party service having a bad day; an unbounded read turns
// that into this process running out of memory.
const maxResponseBytes = 8 << 20

// Record is one result read out of a response: the fields the manifest mapped,
// keyed by the names the domain asked for.
type Record map[string]string

// Get reports a field, or "" when the manifest did not map it.
func (r Record) Get(field string) string { return r[field] }

// HTTPInvoker executes the HTTP runtime of one plugin.
type HTTPInvoker struct {
	manifest *Manifest
	client   *http.Client
}

// NewHTTPInvoker builds an invoker for a manifest.
//
// The client is SSRF-safe because a plugin's endpoint comes from a file an
// operator dropped in, and in the case of a self-hosted integration the URL
// itself is operator configuration. Neither is a reason to let the server be
// pointed at an internal address.
func NewHTTPInvoker(m *Manifest) (*HTTPInvoker, error) {
	if m.Runtime.Type != RuntimeHTTP {
		return nil, fmt.Errorf("%s: not an http plugin", m.Ref())
	}
	clientConfig := secutils.DefaultSSRFSafeHTTPClientConfig()
	clientConfig.Timeout = m.Runtime.Request.ResolvedTimeout()
	return &HTTPInvoker{
		manifest: m,
		client:   secutils.NewSSRFSafeHTTPClient(clientConfig),
	}, nil
}

// Invoke performs the call described by the manifest and returns the mapped
// records.
func (h *HTTPInvoker) Invoke(ctx context.Context, cfg Config, input map[string]any) ([]Record, error) {
	scope := NewScope().
		With("config", cfg.values).
		With("input", input)

	req, err := h.buildRequest(ctx, scope)
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", h.manifest.Ref(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", h.manifest.Ref(), err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %s returned %d: %s",
			h.manifest.Ref(), req.URL.Host, resp.StatusCode, truncate(string(body), 512))
	}
	return h.mapResponse(body)
}

// buildRequest assembles the outbound call from the manifest.
func (h *HTTPInvoker) buildRequest(ctx context.Context, scope Scope) (*http.Request, error) {
	spec := h.manifest.Runtime.Request

	endpoint, err := scope.ResolveString(spec.URL)
	if err != nil {
		return nil, fmt.Errorf("%s: request.url: %w", h.manifest.Ref(), err)
	}
	if endpoint == "" {
		return nil, fmt.Errorf("%s: request.url resolved to empty", h.manifest.Ref())
	}
	if err := secutils.ValidateURLForSSRF(endpoint); err != nil {
		return nil, fmt.Errorf("%s: request.url: %w", h.manifest.Ref(), err)
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%s: request.url %q: %w", h.manifest.Ref(), endpoint, err)
	}
	if len(spec.Query) > 0 {
		values := parsed.Query()
		for name, template := range spec.Query {
			resolved, err := scope.ResolveString(template)
			if err != nil {
				return nil, fmt.Errorf("%s: request.query.%s: %w", h.manifest.Ref(), name, err)
			}
			// An unset optional parameter is omitted rather than sent empty,
			// so a manifest never needs a conditional.
			if resolved == "" {
				continue
			}
			values.Set(name, resolved)
		}
		parsed.RawQuery = values.Encode()
	}

	var payload io.Reader
	if spec.Body != nil {
		resolved, err := scope.ResolveTree(spec.Body)
		if err != nil {
			return nil, fmt.Errorf("%s: request.body: %w", h.manifest.Ref(), err)
		}
		encoded, err := json.Marshal(resolved)
		if err != nil {
			return nil, fmt.Errorf("%s: request.body: %w", h.manifest.Ref(), err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, spec.ResolvedMethod(), parsed.String(), payload)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", h.manifest.Ref(), err)
	}
	if spec.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for name, template := range spec.Headers {
		resolved, err := scope.ResolveString(template)
		if err != nil {
			return nil, fmt.Errorf("%s: request.headers.%s: %w", h.manifest.Ref(), name, err)
		}
		if resolved == "" {
			continue
		}
		req.Header.Set(name, resolved)
	}
	return req, nil
}

// mapResponse reads the reply into records using the manifest's paths.
func (h *HTTPInvoker) mapResponse(body []byte) ([]Record, error) {
	spec := h.manifest.Runtime.Response

	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("%s: response is not valid JSON", h.manifest.Ref())
	}
	parsed := gjson.ParseBytes(body)

	// A vendor answering 200 with an error object is common enough that the
	// manifest can point at it instead of every plugin inventing a check.
	if spec.ErrorPath != "" {
		if message := parsed.Get(spec.ErrorPath); message.Exists() && message.String() != "" {
			return nil, fmt.Errorf("%s: %s", h.manifest.Ref(), truncate(message.String(), 512))
		}
	}

	items := parsed
	if spec.Items != "" {
		items = parsed.Get(spec.Items)
		if !items.Exists() {
			// An endpoint that legitimately found nothing returns an empty
			// list, not a missing key, so a missing key means the manifest's
			// path is wrong and saying so beats returning zero results.
			return nil, fmt.Errorf("%s: response has no %q; check runtime.response.items",
				h.manifest.Ref(), spec.Items)
		}
	}

	var records []Record
	appendItem := func(item gjson.Result) {
		record := make(Record, len(spec.Fields))
		for name, path := range spec.Fields {
			if value := item.Get(path); value.Exists() {
				record[name] = value.String()
			}
		}
		records = append(records, record)
	}

	if items.IsArray() {
		for _, item := range items.Array() {
			appendItem(item)
		}
		return records, nil
	}
	appendItem(items)
	return records, nil
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}
