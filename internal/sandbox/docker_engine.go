// Package sandbox: Docker Engine plumbing for the docker backend.
//
// This file owns everything between WeKnora and the Docker Engine API that is
// not sandbox semantics: the narrow interface the adapter talks to (so unit
// tests need no daemon), how a daemon connection is built and shared, and how
// Engine errors are classified into the provider-neutral RemoteErrorKind.
//
// Connections are pooled per daemon endpoint. Managers are rebuilt on every
// request (see tenant_resolver.go), and each moby client owns an HTTP
// transport, so constructing one per request would leak a connection pool per
// request.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// dockerEngineAPI is the slice of the Docker Engine API the sandbox adapter
// uses. It exists so tests can drive the adapter without a daemon; the real
// implementation is *client.Client, which satisfies it as-is.
type dockerEngineAPI interface {
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)

	ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerStart(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerUnpause(ctx context.Context, containerID string, options client.ContainerUnpauseOptions) (client.ContainerUnpauseResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)

	ExecCreate(ctx context.Context, containerID string, options client.ExecCreateOptions) (client.ExecCreateResult, error)
	ExecAttach(ctx context.Context, execID string, options client.ExecAttachOptions) (client.ExecAttachResult, error)
	ExecInspect(ctx context.Context, execID string, options client.ExecInspectOptions) (client.ExecInspectResult, error)

	CopyToContainer(ctx context.Context, containerID string, options client.CopyToContainerOptions) (client.CopyToContainerResult, error)
	CopyFromContainer(ctx context.Context, containerID string, options client.CopyFromContainerOptions) (client.CopyFromContainerResult, error)
	ContainerStatPath(ctx context.Context, containerID string, options client.ContainerStatPathOptions) (client.ContainerStatPathResult, error)

	ImageInspect(ctx context.Context, imageID string, opts ...client.ImageInspectOption) (client.ImageInspectResult, error)
	ImagePull(ctx context.Context, refStr string, options client.ImagePullOptions) (client.ImagePullResponse, error)
	ImageList(ctx context.Context, options client.ImageListOptions) (client.ImageListResult, error)
}

var _ dockerEngineAPI = (*client.Client)(nil)

// DefaultDockerHost is the endpoint used when a config names none. It matches
// the Docker CLI's own default so an operator who can run `docker ps` on the
// application host needs to configure nothing.
const DefaultDockerHost = "unix:///var/run/docker.sock"

// DefaultDockerHTTPTimeout bounds a single Engine API call. Execution has its
// own per-call timeout and is exempt (see dockerRemoteClient.Exec).
const DefaultDockerHTTPTimeout = 30 * time.Second

// DefaultDockerIdleTTL is how long a session container may go without an exec
// before the idle sweep reclaims it. The daemon has no TTL of its own, so this
// is the only thing standing between an abandoned session and a container that
// lives until the host runs out of memory.
const DefaultDockerIdleTTL = 30 * time.Minute

// DefaultDockerMemoryLimit / DefaultDockerCPULimit / DefaultDockerPidsLimit are
// the per-sandbox resource ceilings applied when a config names none. They are
// deliberately larger than the stateless backend's old 256MB/1CPU: a session
// container hosts a whole turn's work (package installs, data processing),
// not one short script.
const (
	DefaultDockerMemoryLimit int64 = 2 * 1024 * 1024 * 1024
	DefaultDockerCPULimit          = 2.0
	DefaultDockerPidsLimit   int64 = 512
)

// dockerSandboxCapabilities are granted back after CapDrop=ALL.
//
// Everything Docker grants by default that a sandbox does not need is left
// dropped (NET_RAW, NET_BIND_SERVICE, MKNOD, SYS_CHROOT, AUDIT_WRITE,
// SETPCAP, SETFCAP). What remains is what a root-run package installer needs:
// CHOWN/DAC_OVERRIDE/FOWNER/FSETID for writing into image-owned directories
// and fixing up ownership, SETUID/SETGID because apt and pip drop privileges
// while unpacking, KILL so a supervisor can stop its own children.
var dockerSandboxCapabilities = []string{
	"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "SETGID", "SETUID", "KILL",
}

// dockerEngineClientPool hands out one shared client per daemon endpoint.
type dockerEngineClientPool struct {
	mu      sync.Mutex
	clients map[string]*client.Client
}

// sharedDockerEngineClients is process-wide on purpose: two tenant configs
// pointing at the same daemon should share one connection pool, and the check
// endpoint builds throwaway configs that must not each open their own.
var sharedDockerEngineClients = &dockerEngineClientPool{
	clients: make(map[string]*client.Client),
}

// dockerEndpoint is the identity of a daemon connection. Two configs with
// equal endpoints may share a client.
type dockerEndpoint struct {
	Host string

	// TLSCertPath is a directory holding ca.pem / cert.pem / key.pem, the
	// layout Docker's own DOCKER_CERT_PATH uses. Empty means plain HTTP,
	// which is only acceptable for a local unix socket.
	TLSCertPath string

	Timeout time.Duration
}

func (e dockerEndpoint) key() string {
	return fmt.Sprintf("%s|%s|%s", e.Host, e.TLSCertPath, e.Timeout)
}

// get returns the shared client for endpoint, building it on first use.
func (p *dockerEngineClientPool) get(endpoint dockerEndpoint) (*client.Client, error) {
	key := endpoint.key()
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.clients[key]; ok {
		return existing, nil
	}
	built, err := newDockerEngineClient(endpoint)
	if err != nil {
		return nil, err
	}
	p.clients[key] = built
	return built, nil
}

// newDockerEngineClient builds a moby client for one endpoint.
//
// API version negotiation is deliberately not performed here: it costs a
// round-trip against a daemon that may be down, and it would happen inside
// whatever request first touches the pool. The client's default version is
// negotiated lazily by the moby client itself on the first call.
func newDockerEngineClient(endpoint dockerEndpoint) (*client.Client, error) {
	host := strings.TrimSpace(endpoint.Host)
	if host == "" {
		host = DefaultDockerHost
	}
	timeout := endpoint.Timeout
	if timeout <= 0 {
		timeout = DefaultDockerHTTPTimeout
	}

	opts := []client.Opt{
		client.WithHost(host),
		client.WithTimeout(timeout),
		client.WithAPIVersionNegotiation(),
	}
	if endpoint.TLSCertPath != "" {
		// Certificates stay on the application host rather than in the
		// workspace config: they are deployment infrastructure, and keeping
		// them out of the database keeps them out of backups and API
		// responses. The daemon certificate is always verified — a remote
		// daemon accepts container creation, so an unauthenticated peer on
		// that socket is a root shell on the sandbox host.
		certPath := endpoint.TLSCertPath
		opts = append(opts, client.WithTLSClientConfig(
			filepath.Join(certPath, "ca.pem"),
			filepath.Join(certPath, "cert.pem"),
			filepath.Join(certPath, "key.pem"),
		))
	}

	built, err := client.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("sandbox: build docker client for %s: %w", host, err)
	}
	return built, nil
}

// ValidateDockerHost checks a daemon endpoint before it is stored or dialled.
//
// A TCP endpoint gets the same outbound treatment as any other workspace-
// supplied URL: a daemon socket accepts container creation, so an admin who
// can point it anywhere can make WeKnora talk to an arbitrary internal
// service. Unix sockets are local by definition and only have to be absolute.
func ValidateDockerHost(host string, allowPrivate bool) error {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return nil
	}
	scheme, address, found := strings.Cut(trimmed, "://")
	if !found {
		return fmt.Errorf(
			"sandbox: docker host %q must include a scheme (unix:// or tcp://)", host)
	}
	switch strings.ToLower(scheme) {
	case "unix":
		if !strings.HasPrefix(address, "/") {
			return fmt.Errorf("sandbox: docker unix socket path %q must be absolute", address)
		}
		return nil
	case "tcp", "http", "https":
		// The guard speaks HTTP; the daemon's TCP endpoint is an HTTP
		// endpoint, so the check is the same one every other backend gets.
		return ValidateOutboundURLWithPolicy(
			"http://"+address, OutboundURLPolicy{AllowPrivate: allowPrivate},
		)
	default:
		return fmt.Errorf("sandbox: unsupported docker host scheme %q", scheme)
	}
}

// dockerErrorKind classifies an Engine API error. The moby client tags its
// errors with containerd's errdefs, which is a far more reliable signal than
// the message text.
func dockerErrorKind(op string, err error) RemoteErrorKind {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded), cerrdefs.IsDeadlineExceeded(err):
		return RemoteErrorKindTimeout
	case cerrdefs.IsNotFound(err):
		// A missing image on create is a bad template, not a vanished sandbox:
		// classifying it as NotFound would tell the lifecycle it may rebind.
		if op == "Create" {
			return RemoteErrorKindInvalidRequest
		}
		return RemoteErrorKindNotFound
	case cerrdefs.IsUnauthorized(err), cerrdefs.IsPermissionDenied(err):
		return RemoteErrorKindAuthentication
	case cerrdefs.IsInvalidArgument(err):
		return RemoteErrorKindInvalidRequest
	case cerrdefs.IsNotImplemented(err):
		return RemoteErrorKindUnsupported
	case cerrdefs.IsConflict(err), cerrdefs.IsAlreadyExists(err):
		return RemoteErrorKindConflict
	case cerrdefs.IsResourceExhausted(err):
		return RemoteErrorKindCapacity
	case cerrdefs.IsUnavailable(err), client.IsErrConnectionFailed(err):
		return RemoteErrorKindUnavailable
	default:
		return RemoteErrorKindInternal
	}
}

// dockerError wraps an Engine API error as a RemoteError.
func dockerError(op string, err error) error {
	if err == nil {
		return nil
	}
	var existing *RemoteError
	if errors.As(err, &existing) {
		return err
	}
	return &RemoteError{
		Kind:     dockerErrorKind(op, err),
		Provider: SandboxTypeDocker,
		Op:       op,
		Message:  err.Error(),
		Cause:    err,
	}
}

// dockerInvalidRequest reports a caller-side mistake that never reached the
// daemon (an unusable path, an unsupported request shape).
func dockerInvalidRequest(op, message string) error {
	return &RemoteError{
		Kind:     RemoteErrorKindInvalidRequest,
		Provider: SandboxTypeDocker,
		Op:       op,
		Message:  message,
	}
}

// awaitImagePull waits for a pull to finish. The daemon only performs the
// transfer while its progress stream is being consumed, so a caller that
// closes the body early aborts the pull.
func awaitImagePull(ctx context.Context, body client.ImagePullResponse) error {
	if body == nil {
		return nil
	}
	defer func() { _ = body.Close() }()
	return body.Wait(ctx)
}

// dockerStateOf normalizes a container state string. "exited" is deliberately
// NOT terminal: a stopped container keeps its filesystem and Connect restarts
// it, which is the closest Docker gets to E2B's pause + auto-resume.
func dockerStateOf(status container.ContainerState) RemoteSandboxState {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case "running":
		return RemoteStateRunning
	case "paused", "exited", "created":
		return RemoteStatePaused
	case "restarting", "removing":
		return RemoteStateTransitioning
	case "dead":
		return RemoteStateTerminal
	case "":
		return RemoteStateUnknown
	default:
		return RemoteStateUnknown
	}
}

// dockerContainerLabels projects sandbox metadata onto container labels and
// stamps the ownership marker every sweep relies on.
func dockerContainerLabels(metadata map[string]string) map[string]string {
	labels := make(map[string]string, len(metadata)+1)
	for key, value := range metadata {
		labels[key] = value
	}
	labels[dockerManagedLabel] = "true"
	return labels
}

// dockerSandboxMetadata is the inverse of dockerContainerLabels: it strips the
// ownership marker so callers see exactly the metadata they supplied.
func dockerSandboxMetadata(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	metadata := make(map[string]string, len(labels))
	for key, value := range labels {
		if key == dockerManagedLabel {
			continue
		}
		metadata[key] = value
	}
	return metadata
}

// dockerManagedLabel marks every container this backend creates. Sweeps filter
// on it so a WeKnora deployment sharing a daemon with other workloads can
// never delete a container it does not own.
const dockerManagedLabel = "com.weknora.sandbox.managed"

// dockerContainerStartedAt parses the daemon's RFC3339Nano timestamps, which
// are the zero value string "0001-01-01T00:00:00Z" when unset.
func dockerContainerStartedAt(state *container.State) time.Time {
	if state == nil {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, state.StartedAt)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}
