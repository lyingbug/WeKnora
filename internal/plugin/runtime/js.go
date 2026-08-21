package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// jsProvider evaluates a dropped-in search.js. Network goes through
// WeKnora's SSRF-safe client via the host function httpRequest.
type jsProvider struct {
	name    string
	source  string
	timeout time.Duration
	client  *http.Client
	params  types.WebSearchProviderParameters
	mu      sync.Mutex
}

func newJSProvider(name, source string, timeout time.Duration) (*jsProvider, error) {
	client, err := infra_web_search.NewSearchHTTPClient(timeout, "")
	if err != nil {
		return nil, err
	}
	p := &jsProvider{name: name, source: source, timeout: timeout, client: client}
	if err := p.ensureSearchFn(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *jsProvider) withParams(params types.WebSearchProviderParameters) *jsProvider {
	cp := *p
	cp.params = params
	return &cp
}

func (p *jsProvider) Name() string { return p.name }

func (p *jsProvider) Search(
	ctx context.Context, query string, maxResults int, includeDate bool,
) ([]*types.WebSearchResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, err := p.newVM()
	if err != nil {
		return nil, err
	}
	fn, ok := goja.AssertFunction(vm.Get("search"))
	if !ok {
		return nil, fmt.Errorf("plugin %s: search.js must define function search(...)", p.name)
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			vm.Interrupt(ctx.Err().Error())
		case <-time.After(p.timeout):
			vm.Interrupt("js timeout")
		case <-done:
		}
	}()
	defer close(done)

	val, err := fn(
		goja.Undefined(),
		vm.ToValue(query),
		vm.ToValue(maxResults),
		vm.ToValue(includeDate),
		vm.ToValue(paramsToMap(p.params)),
	)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", p.name, err)
	}
	return decodeJSResults(val.Export())
}

func (p *jsProvider) ensureSearchFn() error {
	vm, err := p.newVM()
	if err != nil {
		return err
	}
	if _, ok := goja.AssertFunction(vm.Get("search")); !ok {
		return fmt.Errorf("plugin %s: search.js must define function search(...)", p.name)
	}
	return nil
}

func (p *jsProvider) newVM() (*goja.Runtime, error) {
	vm := goja.New()
	if err := vm.Set("httpRequest", func(call goja.FunctionCall) goja.Value {
		return p.doHTTP(vm, call)
	}); err != nil {
		return nil, err
	}
	if _, err := vm.RunString(p.source); err != nil {
		return nil, fmt.Errorf("plugin %s: load js: %w", p.name, err)
	}
	return vm, nil
}

func (p *jsProvider) doHTTP(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	opts, _ := call.Argument(0).Export().(map[string]any)
	if opts == nil {
		panic(vm.NewTypeError("httpRequest expects an object"))
	}
	method := strings.ToUpper(stringFrom(opts["method"]))
	if method == "" {
		method = http.MethodGet
	}
	rawURL := stringFrom(opts["url"])
	if rawURL == "" {
		panic(vm.NewTypeError("httpRequest.url is required"))
	}
	var body io.Reader
	if b := stringFrom(opts["body"]); b != "" {
		body = strings.NewReader(b)
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return httpResult(vm, 0, err.Error())
	}
	if headers, ok := opts["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprint(v))
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return httpResult(vm, 0, err.Error())
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return httpResult(vm, 0, err.Error())
	}
	return httpResult(vm, resp.StatusCode, string(raw))
}

func httpResult(vm *goja.Runtime, status int, body string) goja.Value {
	o := vm.NewObject()
	_ = o.Set("status", status)
	_ = o.Set("body", body)
	return o
}

func stringFrom(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func paramsToMap(p types.WebSearchProviderParameters) map[string]any {
	return map[string]any{
		"api_key":      p.APIKey,
		"engine_id":    p.EngineID,
		"base_url":     p.BaseURL,
		"proxy_url":    p.ProxyURL,
		"extra_config": p.ExtraConfig,
	}
}

func decodeJSResults(v any) ([]*types.WebSearchResult, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out []*types.WebSearchResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("search() must return an array of results: %w", err)
	}
	return out, nil
}

var _ interfaces.WebSearchProvider = (*jsProvider)(nil)
