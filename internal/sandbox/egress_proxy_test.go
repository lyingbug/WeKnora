package sandbox

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEgressProxyAllowsApprovedHTTPHost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	proxy := NewEgressProxy([]string{upstreamURL.Hostname()})
	require.NoError(t, proxy.Start(context.Background()))
	defer proxy.Shutdown(context.Background())

	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(mustParseURL(t, proxy.URL()))}}
	resp, err := client.Get(upstream.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", string(body))
}

func TestEgressProxyRejectsUnapprovedHTTPHost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("blocked?"))
	}))
	defer upstream.Close()

	proxy := NewEgressProxy([]string{"api.example.com"})
	require.NoError(t, proxy.Start(context.Background()))
	defer proxy.Shutdown(context.Background())

	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(mustParseURL(t, proxy.URL()))}}
	resp, err := client.Get(upstream.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestEgressProxyAllowsApprovedHTTPSHost(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("secure"))
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	proxy := NewEgressProxy([]string{upstreamURL.Hostname()})
	require.NoError(t, proxy.Start(context.Background()))
	defer proxy.Shutdown(context.Background())

	client := &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(mustParseURL(t, proxy.URL())),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Get(upstream.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "secure", string(body))
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}
