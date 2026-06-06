package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RemoteSandbox delegates execution to an external sandbox scheduler.
type RemoteSandbox struct {
	config *Config
	client *http.Client
}

// NewRemoteSandbox creates a remote scheduler backed sandbox.
func NewRemoteSandbox(config *Config) *RemoteSandbox {
	if config == nil {
		config = DefaultConfig()
	}
	return &RemoteSandbox{
		config: config,
		client: &http.Client{},
	}
}

func (s *RemoteSandbox) Type() SandboxType {
	return SandboxTypeRemote
}

func (s *RemoteSandbox) IsAvailable(ctx context.Context) bool {
	if s.config == nil || s.config.RemoteEndpoint == "" {
		return false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.remoteURL("/health"), nil)
	if err != nil {
		return false
	}
	s.authorize(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (s *RemoteSandbox) Execute(ctx context.Context, config *ExecuteConfig) (*ExecuteResult, error) {
	if config == nil {
		return nil, ErrInvalidScript
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = s.config.DefaultTimeout
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	scriptContent := config.ScriptContent
	if scriptContent == "" {
		content, err := os.ReadFile(config.Script)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrScriptNotFound, err)
		}
		scriptContent = string(content)
	}

	payload := remoteExecuteRequest{
		Script:                scriptContent,
		ScriptName:            filepath.Base(config.Script),
		Args:                  config.Args,
		Stdin:                 config.Stdin,
		Env:                   config.Env,
		TimeoutMS:             timeout.Milliseconds(),
		AllowNetwork:          config.AllowNetwork,
		AllowedNetworkDomains: config.AllowedNetworkDomains,
		MemoryLimit:           firstNonZeroInt64(config.MemoryLimit, s.config.MaxMemory),
		CPULimit:              firstNonZeroFloat64(config.CPULimit, s.config.MaxCPU),
		Mounts:                config.Mounts,
		Metadata:              config.Metadata,
		WorkDir:               config.WorkDir,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(execCtx, http.MethodPost, s.remoteURL("/v1/execute"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	s.authorize(req)

	start := time.Now()
	resp, err := s.client.Do(req)
	duration := time.Since(start)
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return &ExecuteResult{
				ExitCode: -1,
				Duration: duration,
				Killed:   true,
				Error:    ErrTimeout.Error(),
			}, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	var remoteResult remoteExecuteResponse
	if err := json.NewDecoder(resp.Body).Decode(&remoteResult); err != nil {
		return nil, fmt.Errorf("decode remote sandbox response: %w", err)
	}

	result := &ExecuteResult{
		Stdout:   remoteResult.Stdout,
		Stderr:   remoteResult.Stderr,
		ExitCode: remoteResult.ExitCode,
		Duration: durationFromRemote(remoteResult.DurationMS, duration),
		Killed:   remoteResult.Killed,
		Error:    remoteResult.Error,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if result.Error == "" {
			result.Error = fmt.Sprintf("remote sandbox returned status %d", resp.StatusCode)
		}
		if result.ExitCode == 0 {
			result.ExitCode = -1
		}
		return result, ErrExecutionFailed
	}

	return result, nil
}

func (s *RemoteSandbox) Cleanup(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.remoteURL("/v1/cleanup"), nil)
	if err != nil {
		return err
	}
	s.authorize(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("remote sandbox cleanup returned status %d", resp.StatusCode)
}

func (s *RemoteSandbox) authorize(req *http.Request) {
	if s.config.RemoteToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.config.RemoteToken)
	}
}

func (s *RemoteSandbox) remoteURL(path string) string {
	base := strings.TrimRight(s.config.RemoteEndpoint, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonZeroFloat64(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func durationFromRemote(durationMS int64, fallback time.Duration) time.Duration {
	if durationMS > 0 {
		return time.Duration(durationMS) * time.Millisecond
	}
	return fallback
}

type remoteExecuteRequest struct {
	Script                string            `json:"script"`
	ScriptName            string            `json:"script_name"`
	Args                  []string          `json:"args,omitempty"`
	Stdin                 string            `json:"stdin,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	TimeoutMS             int64             `json:"timeout_ms"`
	AllowNetwork          bool              `json:"allow_network"`
	AllowedNetworkDomains []string          `json:"allowed_network_domains,omitempty"`
	MemoryLimit           int64             `json:"memory_limit,omitempty"`
	CPULimit              float64           `json:"cpu_limit,omitempty"`
	Mounts                []Mount           `json:"mounts,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	WorkDir               string            `json:"work_dir,omitempty"`
}

type remoteExecuteResponse struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	Killed     bool   `json:"killed"`
	Error      string `json:"error"`
}
