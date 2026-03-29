package httputil

import (
	"net/http"
	"time"

	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// DefaultTransport is a shared HTTP transport with SSRF-safe dialer and connection-level timeouts.
// It does NOT set an overall Timeout — streaming calls are controlled by context cancellation.
var DefaultTransport = &http.Transport{
	DialContext:         secutils.SSRFSafeDialContext,
	TLSHandshakeTimeout: 10 * time.Second,
	IdleConnTimeout:     90 * time.Second,
	MaxIdleConnsPerHost: 10,
	Proxy:               http.ProxyFromEnvironment,
}

// StreamingClient is a shared HTTP client for streaming calls (no overall timeout).
// Uses SSRFSafeDialContext to prevent DNS rebinding attacks.
var StreamingClient = &http.Client{
	Transport: DefaultTransport,
}

// DefaultTimeout is the default timeout for non-streaming API calls (embedding, rerank, etc.).
const DefaultTimeout = 60 * time.Second

// TimedClient returns a new HTTP client with the given timeout, sharing the default transport.
func TimedClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: DefaultTransport,
	}
}
