package runtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/plugin/protocol"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// stdioSession is one host-launched subprocess that speaks the plugin ABI
// on stdin/stdout. The author does not open a port or serve HTTP.
type stdioSession struct {
	name    string
	command string
	args    []string
	dir     string
	env     []string
	timeout time.Duration

	mu        sync.Mutex
	closed    bool
	cmd       *exec.Cmd
	conn      *protocol.Conn
	runCancel context.CancelFunc
	done      <-chan struct{}
}

func newStdioSession(m plugin.Manifest) (*stdioSession, error) {
	name, args := m.Exec()
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: command %q: %w", m.ID, name, err)
	}
	return &stdioSession{
		name:    m.ProviderID(),
		command: path,
		args:    args,
		dir:     m.Dir,
		env:     extraEnv(m.Env),
		timeout: clampTimeout(m.Timeout()),
	}, nil
}

func (s *stdioSession) withParams(params types.WebSearchProviderParameters) *stdioProvider {
	return &stdioProvider{session: s, params: params}
}

func (s *stdioSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.stopLocked()
	return nil
}

func (s *stdioSession) search(
	ctx context.Context, query string, maxResults int, includeDate bool,
	params types.WebSearchProviderParameters,
) ([]*types.WebSearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("plugin %s: stdio session closed", s.name)
	}
	if err := s.ensureLocked(); err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var out protocol.SearchResponse
	err := s.conn.Call(callCtx, protocol.MethodWebSearchSearch, protocol.SearchRequest{
		Query: query, MaxResults: maxResults, IncludeDate: includeDate, Parameters: params,
	}, &out)
	if err != nil {
		s.stopLocked()
		return nil, fmt.Errorf("plugin %s: stdio: %w", s.name, err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("plugin %s: %s", s.name, out.Error)
	}
	return out.Results, nil
}

func (s *stdioSession) ensureLocked() error {
	if s.cmd != nil {
		select {
		case <-s.done:
			s.clearLocked()
		default:
			return nil
		}
	}
	return s.startLocked()
}

func (s *stdioSession) startLocked() error {
	runCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(runCtx, s.command, s.args...)
	cmd.Dir = s.dir
	if len(s.env) > 0 {
		cmd.Env = append(os.Environ(), s.env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("plugin %s: start %s: %w", s.name, s.command, err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	go drainStderr(s.name, stderr)

	s.cmd = cmd
	s.conn = protocol.NewConn(stdout, stdin)
	s.runCancel = cancel
	s.done = done
	return nil
}

func (s *stdioSession) stopLocked() {
	if s.conn != nil {
		_ = s.conn.Notify(protocol.MethodShutdown, nil)
	}
	if s.runCancel != nil {
		s.runCancel()
	}
	if s.done != nil {
		<-s.done
	}
	s.clearLocked()
}

func (s *stdioSession) clearLocked() {
	s.cmd = nil
	s.conn = nil
	s.runCancel = nil
	s.done = nil
}

func extraEnv(kv map[string]string) []string {
	if len(kv) == 0 {
		return nil
	}
	out := make([]string, 0, len(kv))
	for k, v := range kv {
		if k == "" {
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}

func drainStderr(name string, r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		logger.Debugf(context.Background(), "[plugin %s] %s", name, sc.Text())
	}
}

type stdioProvider struct {
	session *stdioSession
	params  types.WebSearchProviderParameters
}

func (p *stdioProvider) Name() string { return p.session.name }

func (p *stdioProvider) Search(
	ctx context.Context, query string, maxResults int, includeDate bool,
) ([]*types.WebSearchResult, error) {
	return p.session.search(ctx, query, maxResults, includeDate, p.params)
}

var _ interfaces.WebSearchProvider = (*stdioProvider)(nil)
