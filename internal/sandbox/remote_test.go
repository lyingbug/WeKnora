package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoteSandboxExecute(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "main.py")
	scriptContent := `print("hello")`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	var received remoteExecuteRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}

		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusNoContent)
		case "/v1/execute":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(remoteExecuteResponse{
				Stdout:     "remote stdout",
				Stderr:     "remote stderr",
				ExitCode:   7,
				DurationMS: 123,
				Killed:     true,
				Error:      "remote error",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.Type = SandboxTypeRemote
	config.RemoteEndpoint = server.URL
	config.RemoteToken = "test-token"
	config.DefaultTimeout = 3 * time.Second
	config.MaxMemory = 128
	config.MaxCPU = 0.5

	remoteSandbox := NewRemoteSandbox(config)
	if !remoteSandbox.IsAvailable(context.Background()) {
		t.Fatal("remote sandbox should be available")
	}

	result, err := remoteSandbox.Execute(context.Background(), &ExecuteConfig{
		Script:                scriptPath,
		Args:                  []string{"one", "two"},
		Stdin:                 "input",
		Env:                   map[string]string{"A": "B"},
		WorkDir:               "/workspace",
		AllowNetwork:          true,
		AllowedNetworkDomains: []string{"api.example.com"},
		MemoryLimit:           256,
		CPULimit:              1.25,
		Mounts: []Mount{{
			HostPath:      "/host/session",
			ContainerPath: "/workspace/session",
			ReadOnly:      true,
		}},
		Metadata: map[string]string{"tenant_id": "tenant-1", "session_id": "session-1"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if result.Stdout != "remote stdout" || result.Stderr != "remote stderr" || result.ExitCode != 7 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Duration != 123*time.Millisecond || !result.Killed || result.Error != "remote error" {
		t.Fatalf("unexpected result metadata: %+v", result)
	}

	if received.Script != scriptContent {
		t.Fatalf("unexpected script content: %q", received.Script)
	}
	if received.ScriptName != "main.py" || received.TimeoutMS != 3000 {
		t.Fatalf("unexpected script metadata: %+v", received)
	}
	if len(received.Args) != 2 || received.Args[1] != "two" || received.Stdin != "input" {
		t.Fatalf("unexpected io payload: %+v", received)
	}
	if !received.AllowNetwork || len(received.AllowedNetworkDomains) != 1 || received.AllowedNetworkDomains[0] != "api.example.com" {
		t.Fatalf("unexpected network payload: %+v", received)
	}
	if received.MemoryLimit != 256 || received.CPULimit != 1.25 {
		t.Fatalf("unexpected resource payload: %+v", received)
	}
	if len(received.Mounts) != 1 || received.Mounts[0].ContainerPath != "/workspace/session" {
		t.Fatalf("unexpected mounts payload: %+v", received.Mounts)
	}
	if received.Metadata["tenant_id"] != "tenant-1" || received.Metadata["session_id"] != "session-1" {
		t.Fatalf("unexpected metadata payload: %+v", received.Metadata)
	}
}

func TestRemoteSandboxExecuteNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/execute":
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(remoteExecuteResponse{
				Stderr:   "denied",
				ExitCode: -1,
				Error:    "egress blocked",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.Type = SandboxTypeRemote
	config.RemoteEndpoint = server.URL
	remoteSandbox := NewRemoteSandbox(config)

	result, err := remoteSandbox.Execute(context.Background(), &ExecuteConfig{
		ScriptContent: "print('x')",
		Script:        "main.py",
	})
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("expected ErrExecutionFailed, got %v", err)
	}
	if result == nil || result.Error != "egress blocked" || result.Stderr != "denied" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestValidateConfigRemoteRequiresEndpoint(t *testing.T) {
	config := DefaultConfig()
	config.Type = SandboxTypeRemote

	if err := ValidateConfig(config); err == nil {
		t.Fatal("expected remote endpoint validation error")
	}

	config.RemoteEndpoint = "https://sandbox.example.com"
	if err := ValidateConfig(config); err != nil {
		t.Fatalf("validate remote config: %v", err)
	}
}
