package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/plugin/protocol"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestMain(m *testing.M) {
	if os.Getenv("WEKNORA_STDIO_HELPER") != "" {
		runStdioHelper(os.Getenv("WEKNORA_STDIO_HELPER"))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runStdioHelper(kind string) {
	sc := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var req protocol.Request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			continue
		}
		if req.Method == protocol.MethodShutdown {
			return
		}
		if kind == "sleep" {
			select {}
		}
		var in protocol.SearchRequest
		_ = json.Unmarshal(req.Params, &in)
		out := protocol.SearchResponse{}
		if q := strings.TrimSpace(in.Query); q != "" {
			title := "stdio-helper"
			if in.Parameters.APIKey != "" {
				title = in.Parameters.APIKey
			}
			out.Results = []*types.WebSearchResult{{
				Title: title, URL: "https://weknora.local/stdio",
				Snippet: q, Source: "stdio-helper",
			}}
		}
		raw, _ := json.Marshal(out)
		_ = enc.Encode(protocol.Response{JSONRPC: "2.0", ID: req.ID, Result: raw})
	}
}

func helperManifest(t *testing.T, kind string, timeoutMS int) plugin.Manifest {
	t.Helper()
	return plugin.Manifest{
		ID:        "websearch.stdio-helper",
		Seam:      plugin.ServiceWebSearch,
		Runtime:   plugin.RuntimeStdio,
		Command:   os.Args[0],
		Args:      []string{"-test.run=^$"},
		Provider:  "stdio-helper",
		TimeoutMS: timeoutMS,
		Env:       map[string]string{"WEKNORA_STDIO_HELPER": kind},
		Dir:       t.TempDir(),
	}
}

func TestStdioMissingCommand(t *testing.T) {
	_, err := newStdioSession(plugin.Manifest{
		ID: "websearch.missing", Seam: plugin.ServiceWebSearch, Runtime: plugin.RuntimeStdio,
		Command: "weknora-no-such-bin-xyz", Dir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected lookpath error")
	}
}

func TestStdioProviderSearch(t *testing.T) {
	session, err := newStdioSession(helperManifest(t, "echo", 3000))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	p := session.withParams(types.WebSearchProviderParameters{APIKey: "keyed"})
	got, err := p.Search(context.Background(), "hello", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "keyed" || got[0].Snippet != "hello" {
		t.Fatalf("got = %+v", got)
	}

	got, err = p.Search(context.Background(), "again", 1, false)
	if err != nil || len(got) != 1 || got[0].Snippet != "again" {
		t.Fatalf("reuse = %+v, %v", got, err)
	}
}

func TestStdioProviderTimeoutRestarts(t *testing.T) {
	session, err := newStdioSession(helperManifest(t, "sleep", 50))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	_, err = session.withParams(types.WebSearchProviderParameters{}).
		Search(context.Background(), "x", 1, false)
	if err == nil {
		t.Fatal("expected timeout")
	}

	session.timeout = clampTimeout(3000)
	session.env = extraEnv(map[string]string{"WEKNORA_STDIO_HELPER": "echo"})
	got, err := session.withParams(types.WebSearchProviderParameters{}).
		Search(context.Background(), "recovered", 1, false)
	if err != nil || len(got) != 1 || got[0].Snippet != "recovered" {
		t.Fatalf("after restart = %+v, %v", got, err)
	}
}

func TestStdioCloseRejectsSearch(t *testing.T) {
	session, err := newStdioSession(helperManifest(t, "echo", 3000))
	if err != nil {
		t.Fatal(err)
	}
	p := session.withParams(types.WebSearchProviderParameters{})
	if _, err := p.Search(context.Background(), "x", 1, false); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Search(context.Background(), "x", 1, false); err == nil {
		t.Fatal("expected closed session")
	}
}

func TestRegisterManifestsMountsStdio(t *testing.T) {
	m := helperManifest(t, "echo", 3000)
	if err := RegisterManifests([]plugin.Manifest{m}); err != nil {
		t.Fatal(err)
	}
	reg := infra_web_search.NewRegistry()
	h := plugin.NewHost()
	h.Context().Provide(plugin.ServiceWebSearch, reg)
	if err := h.Compose(plugin.Profile{Bundles: []string{plugin.ExternalBundle}}, map[string]plugin.Bundle{
		plugin.ExternalBundle: plugin.BundleFromManifests([]plugin.Manifest{m}),
	}, nil); err != nil {
		t.Fatal(err)
	}
	p, err := reg.CreateProvider("stdio-helper", types.WebSearchProviderParameters{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Search(context.Background(), "mounted", 1, false)
	if err != nil || len(got) != 1 || got[0].Snippet != "mounted" {
		t.Fatalf("search = %+v, %v", got, err)
	}
	h.Unload()
	if reg.Has("stdio-helper") {
		t.Fatal("unload should drop stdio provider")
	}
}

func TestStdioPythonSample(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
	dir := filepath.Join(repoRoot(t), "plugins.d", "websearch-stdio-echo")
	ms, err := plugin.Discover([]string{dir})
	if err != nil || len(ms) != 1 {
		t.Fatalf("discover = %v, %v", ms, err)
	}
	session, err := newStdioSession(ms[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	got, err := session.withParams(types.WebSearchProviderParameters{}).
		Search(context.Background(), "from-python", 1, false)
	if err != nil || len(got) != 1 || got[0].Source != "stdio-echo" {
		t.Fatalf("python sample = %+v, %v", got, err)
	}
}

func TestStdioNodeSample(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	dir := filepath.Join(repoRoot(t), "plugins.d", "websearch-node-echo")
	ms, err := plugin.Discover([]string{dir})
	if err != nil || len(ms) != 1 {
		t.Fatalf("discover = %v, %v", ms, err)
	}
	session, err := newStdioSession(ms[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	got, err := session.withParams(types.WebSearchProviderParameters{}).
		Search(context.Background(), "from-node", 1, false)
	if err != nil || len(got) != 1 || got[0].Source != "node-echo" {
		t.Fatalf("node sample = %+v, %v", got, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
