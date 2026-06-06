package sandbox

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

type EgressProxy struct {
	allowedDomains []string
	server         *http.Server
	listener       net.Listener
	url            string
	mu             sync.RWMutex
}

func NewEgressProxy(allowedDomains []string) *EgressProxy {
	return &EgressProxy{allowedDomains: allowedDomains}
}

func (p *EgressProxy) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server != nil {
		return nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to start egress proxy: %w", err)
	}
	p.listener = listener
	p.url = "http://" + listener.Addr().String()
	p.server = &http.Server{Handler: http.HandlerFunc(p.handle)}
	go func() {
		_ = p.server.Serve(listener)
	}()
	go func() {
		<-ctx.Done()
		_ = p.Shutdown(context.Background())
	}()
	return nil
}

func (p *EgressProxy) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server == nil {
		return nil
	}
	err := p.server.Shutdown(ctx)
	p.server = nil
	p.listener = nil
	p.url = ""
	return err
}

func (p *EgressProxy) URL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.url
}

func (p *EgressProxy) handle(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Hostname()
	if host == "" {
		host = hostOnly(r.Host)
	}
	if !egressDomainAllowed(host, p.allowedDomains) {
		http.Error(w, "network domain is not approved", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	outReq.Header.Del("Proxy-Connection")
	resp, err := http.DefaultTransport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, "egress proxy upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *EgressProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	upstream, err := net.Dial("tcp", r.Host)
	if err != nil {
		http.Error(w, "egress proxy upstream connect failed", http.StatusBadGateway)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(w, "hijacking is not supported", http.StatusInternalServerError)
		return
	}
	client, _, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	go proxyCopy(upstream, client)
	go proxyCopy(client, upstream)
}

func proxyCopy(dst net.Conn, src net.Conn) {
	defer dst.Close()
	defer src.Close()
	_, _ = io.Copy(dst, src)
}

func egressDomainAllowed(host string, allowedDomains []string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, domain := range allowedDomains {
		domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
		if domain == "" {
			continue
		}
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func hostOnly(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err == nil {
		return host
	}
	return hostport
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
