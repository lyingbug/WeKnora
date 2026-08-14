package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/llm/protocol"
	_ "github.com/Tencent/WeKnora/internal/models/llm/protocol/all" // register the standard protocol drivers
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	_ "github.com/Tencent/WeKnora/internal/models/llm/vendors" // register the built-in vendor plugins
	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// PluginChat drives a model through the plugin seam: a vendor descriptor
// supplies the parameter dispositions, a protocol driver supplies the wire.
//
// It owns only what neither of those does — the HTTP round trip, credentials,
// and timeouts — which is why it stays short while covering three protocols
// and every registered vendor.
type PluginChat struct {
	descriptor spi.Descriptor
	driver     protocol.Driver
	endpoint   string

	modelName string
	modelID   string
	creds     spi.Credentials

	customHeaders map[string]string
}

// endpointer is implemented by drivers that derive a full URL from a base URL,
// which is how a protocol absorbs the base-URL spellings users actually paste.
type endpointer interface {
	Endpoint(baseURL string) string
}

// NewPluginChat builds a plugin-driven chat client, reporting whether the
// configuration resolves to a registered plugin. A caller that gets false
// should fall back to the legacy path rather than fail: an unregistered vendor
// is a gap in the catalog, not a broken model.
func NewPluginChat(config *ChatConfig) (*PluginChat, bool, error) {
	vendor := resolveVendorName(config)
	desc, ok := spi.Resolve(spi.Query{
		Vendor:   string(vendor),
		Kind:     spi.KindChat,
		Model:    config.ModelName,
		Protocol: configuredProtocol(config),
	})
	if !ok {
		return nil, false, nil
	}

	driver, err := protocol.MustGet(desc.Protocol)
	if err != nil {
		return nil, false, err
	}

	baseURL := strings.TrimRight(config.BaseURL, "/")
	if baseURL == "" {
		baseURL = desc.DefaultBaseURL
	}
	if baseURL == "" {
		return nil, false, fmt.Errorf("%s: base URL is required", desc.Vendor)
	}
	if err := secutils.ValidateURLForSSRF(baseURL); err != nil {
		return nil, false, fmt.Errorf("baseURL SSRF check failed: %w", err)
	}

	endpoint := buildEndpoint(driver, desc, baseURL)
	if err := secutils.ValidateURLForSSRF(endpoint); err != nil {
		return nil, false, fmt.Errorf("endpoint SSRF check failed: %w", err)
	}

	if desc.Auth.EffectiveKind() == spi.AuthSigned {
		if config.AppID == "" || config.AppSecret == "" {
			return nil, false, fmt.Errorf("%s: application credentials are required", desc.Vendor)
		}
	}

	return &PluginChat{
		descriptor: desc,
		driver:     driver,
		endpoint:   endpoint,
		modelName:  remoteModelName(config),
		modelID:    config.ModelID,
		creds: spi.Credentials{
			APIKey:    config.APIKey,
			AppID:     config.AppID,
			AppSecret: config.AppSecret,
		},
		customHeaders: config.CustomHeaders,
	}, true, nil
}

// buildEndpoint resolves the request URL, letting a descriptor override the
// protocol's standard path for vendors serving a compatible protocol elsewhere.
func buildEndpoint(driver protocol.Driver, desc spi.Descriptor, baseURL string) string {
	if desc.EndpointPath != "" {
		return baseURL + desc.EndpointPath
	}
	if e, ok := driver.(endpointer); ok {
		return e.Endpoint(baseURL)
	}
	return baseURL + driver.EndpointPath()
}

// resolveVendorName reports the provider identity, falling back to detection
// from the base URL exactly as the rest of the model layer does.
func resolveVendorName(config *ChatConfig) provider.ProviderName {
	name := provider.ProviderName(strings.TrimSpace(config.Provider))
	if name == "" {
		name = provider.DetectProvider(config.BaseURL)
	}
	return name
}

// configuredProtocol reports the protocol pinned in the model configuration.
// It is empty for the great majority of models, where the vendor offers one.
func configuredProtocol(config *ChatConfig) spi.ProtocolID {
	if config.ExtraConfig == nil {
		return ""
	}
	return spi.ProtocolID(strings.TrimSpace(config.ExtraConfig[ExtraConfigProtocol]))
}

// remoteModelName reports the identifier to send, honoring the override some
// deployments need when the stored name is a local label.
func remoteModelName(config *ChatConfig) string {
	if config.ExtraConfig != nil {
		if override := strings.TrimSpace(config.ExtraConfig["remote_model_name"]); override != "" {
			return override
		}
	}
	return config.ModelName
}

// GetModelName reports the model identifier.
func (c *PluginChat) GetModelName() string { return c.modelName }

// GetModelID reports the stored model id.
func (c *PluginChat) GetModelID() string { return c.modelID }

// Descriptor reports the resolved plugin, for diagnostics.
func (c *PluginChat) Descriptor() spi.Descriptor { return c.descriptor }

// Plan resolves what a call would send without sending it. The debug endpoint
// uses it to report the real request shape rather than re-deriving it.
func (c *PluginChat) Plan(opts *ChatOptions, stream bool) (*spi.Plan, error) {
	return c.descriptor.Plan(spi.Request{
		Model:  c.modelName,
		Stream: stream,
		Values: opts.ParamValues(),
	})
}

// build renders one call into its outbound bytes, running the protocol driver
// first and the vendor plan over the result.
func (c *PluginChat) build(messages []Message, opts *ChatOptions, stream bool) ([]byte, *spi.Plan, error) {
	draft, err := c.driver.BuildDraft(protocol.Call{
		Model:    c.modelName,
		Stream:   stream,
		Messages: messages,
		Options:  opts,
		Replay:   c.descriptor.EffectiveReplay(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}

	plan, err := c.Plan(opts, stream)
	if err != nil {
		return nil, nil, err
	}
	if err := plan.Apply(draft); err != nil {
		return nil, nil, err
	}

	body, err := json.Marshal(draft.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}
	return body, plan, nil
}

// newRequest builds the authenticated HTTP request for a body.
func (c *PluginChat) newRequest(ctx context.Context, body []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	auth := c.descriptor.Auth
	for key, value := range auth.Static {
		req.Header.Set(key, value)
	}
	switch auth.EffectiveKind() {
	case spi.AuthHeader:
		req.Header.Set(auth.Header, c.creds.APIKey)
	case spi.AuthSigned:
		headers, err := auth.Signer.Sign(body, c.creds)
		if err != nil {
			return nil, fmt.Errorf("sign request: %w", err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	default:
		req.Header.Set("Authorization", "Bearer "+c.creds.APIKey)
	}

	// User headers come last but never displace the ones above; the helper
	// skips reserved names.
	secutils.ApplyCustomHeaders(req, c.customHeaders)
	return req, nil
}

// logPlan records the adjustments a vendor's rules forced, so a request that
// came back without reasoning can be explained from the logs alone.
func (c *PluginChat) logPlan(ctx context.Context, plan *spi.Plan, body []byte) {
	logger.Infof(ctx, "[LLM Request] vendor=%s protocol=%s model=%s endpoint=%s request:\n%s",
		plan.Vendor, plan.Protocol, c.modelName, c.endpoint,
		secutils.CompactImageDataURLForLog(string(body)))
	for _, note := range plan.Notes {
		logger.Infof(ctx, "[LLM Plan] %s %s: %s", note.Param, note.Reason, note.Detail)
	}
}

// Chat performs a non-streaming call.
func (c *PluginChat) Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*types.ChatResponse, error) {
	ctx, cancel := withLLMTimeout(ctx, defaultChatTimeout)
	defer cancel()

	body, plan, err := c.build(messages, opts, false)
	if err != nil {
		return nil, err
	}
	c.logPlan(ctx, plan, body)

	req, err := c.newRequest(ctx, body, false)
	if err != nil {
		return nil, err
	}
	resp, err := rawHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(payload))
	}

	result, err := c.driver.ParseResponse(payload)
	if err != nil {
		return nil, err
	}
	logUsage(ctx, c.modelName, &result.Usage)
	return result, nil
}

// ChatStream performs a streaming call.
func (c *PluginChat) ChatStream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan types.StreamResponse, error) {
	streamCtx, cancel := withLLMTimeout(ctx, defaultStreamTimeout)

	body, plan, err := c.build(messages, opts, true)
	if err != nil {
		cancel()
		return nil, err
	}
	c.logPlan(streamCtx, plan, body)

	req, err := c.newRequest(streamCtx, body, true)
	if err != nil {
		cancel()
		return nil, err
	}
	resp, err := rawHTTPClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("send request: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(payload))
	}

	out := make(chan types.StreamResponse)
	go func() {
		defer cancel()
		defer close(out)
		defer resp.Body.Close()
		c.driver.DecodeStream(streamCtx, resp.Body, out)
	}()
	return out, nil
}
