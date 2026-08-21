package plugin_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/utils"
)

// allowLoopback lets a test reach an httptest server past the SSRF guard. The
// parsed whitelist is process-cached, so it is reset on both sides.
func allowLoopback(t *testing.T) {
	t.Helper()
	utils.ResetSSRFWhitelistForTest()
	t.Setenv("SSRF_WHITELIST", "127.0.0.1,localhost")
	t.Cleanup(utils.ResetSSRFWhitelistForTest)
}

// The claim this package has to earn is that a plugin is a file: writing one
// installs a capability into a running process and deleting it removes the
// capability, with no rebuild. These tests exercise that literally — they
// write YAML to a temporary directory and watch the registry change.

const echoPlugin = `
apiVersion: weknora/v1
kind: demo
id: echo
name: Echo
config:
  - id: api_key
    type: string
    required: true
    secret: true
  - id: flavor
    type: enum
    default: plain
    options:
      - value: plain
      - value: fancy
runtime:
  type: http
  request:
    method: POST
    url: %s
    headers:
      Authorization: Bearer ${config.api_key}
    body:
      query: ${input.query}
      count: ${input.max_results}
      flavor: ${config.flavor}
  response:
    items: results
    fields:
      title: title
      url: link
`

func writePlugin(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// A plugin that arrives while the process is running becomes usable, and one
// that is deleted stops being usable. No recompilation, no restart.
func TestPluginsAppearAndDisappearWithTheirFiles(t *testing.T) {
	dir := t.TempDir()
	reg := plugin.NewRegistry()

	if count := plugin.LoadDir(reg, "user", dir); count != 0 {
		t.Fatalf("an empty directory should install nothing, got %d", count)
	}
	if _, ok := reg.Lookup("demo", "echo"); ok {
		t.Fatal("nothing should be installed yet")
	}

	writePlugin(t, dir, "echo.yaml", strings.Replace(echoPlugin, "%s", "https://example.test/search", 1))
	if count := plugin.LoadDir(reg, "user", dir); count != 1 {
		t.Fatalf("writing a file should install one plugin, got %d", count)
	}
	if _, ok := reg.Lookup("demo", "echo"); !ok {
		t.Fatal("the plugin should be installed")
	}

	if err := os.Remove(filepath.Join(dir, "echo.yaml")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	plugin.LoadDir(reg, "user", dir)
	if _, ok := reg.Lookup("demo", "echo"); ok {
		t.Fatal("deleting the file should uninstall the plugin")
	}
}

// The watcher is what makes that automatic: no reload call, just a file
// appearing on disk.
func TestWatcherInstallsWithoutAnyoneAskingItTo(t *testing.T) {
	dir := t.TempDir()
	reg := plugin.NewRegistry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher := plugin.NewWatcher(reg, "user", dir)
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("start watcher: %v", err)
	}

	writePlugin(t, dir, "echo.yaml", strings.Replace(echoPlugin, "%s", "https://example.test/search", 1))

	if !eventually(t, 3*time.Second, func() bool {
		_, ok := reg.Lookup("demo", "echo")
		return ok
	}) {
		t.Fatal("the watcher should have installed the plugin after the file appeared")
	}

	if err := os.Remove(filepath.Join(dir, "echo.yaml")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !eventually(t, 3*time.Second, func() bool {
		_, ok := reg.Lookup("demo", "echo")
		return !ok
	}) {
		t.Fatal("the watcher should have uninstalled the plugin after the file was deleted")
	}
}

// Hand-written files are wrong sometimes. One bad file must cost one plugin,
// not the whole directory, and the reason must be reportable to whoever wrote
// it rather than buried in a log.
func TestABrokenFileDoesNotTakeDownTheRest(t *testing.T) {
	dir := t.TempDir()
	reg := plugin.NewRegistry()

	writePlugin(t, dir, "good.yaml", strings.Replace(echoPlugin, "%s", "https://example.test/search", 1))
	writePlugin(t, dir, "broken.yaml", "apiVersion: weknora/v1\nkind: demo\nid: broken\n")

	count := plugin.LoadDir(reg, "user", dir)
	if count != 1 {
		t.Fatalf("the good plugin should still install, got %d", count)
	}
	if _, ok := reg.Lookup("demo", "echo"); !ok {
		t.Fatal("the good plugin should be usable")
	}

	failures := reg.Failures()
	if len(failures) != 1 {
		t.Fatalf("expected one reported failure, got %+v", failures)
	}
	if !strings.Contains(failures[0].Source, "broken.yaml") {
		t.Errorf("the failure should name the file, got %q", failures[0].Source)
	}
	if failures[0].Err == "" {
		t.Error("the failure should explain itself")
	}
}

// Fixing a broken file while the server runs must recover, and a file that
// breaks must not leave a stale version in effect under a false name.
func TestEditingAFileReplacesWhatIsInEffect(t *testing.T) {
	dir := t.TempDir()
	reg := plugin.NewRegistry()

	writePlugin(t, dir, "echo.yaml", strings.Replace(echoPlugin, "%s", "https://first.test/search", 1))
	plugin.LoadDir(reg, "user", dir)

	entry, ok := reg.Lookup("demo", "echo")
	if !ok {
		t.Fatal("the plugin should be installed")
	}
	if got := entry.Manifest.Runtime.Request.URL; got != "https://first.test/search" {
		t.Fatalf("url = %q", got)
	}

	writePlugin(t, dir, "echo.yaml", strings.Replace(echoPlugin, "%s", "https://second.test/search", 1))
	plugin.LoadDir(reg, "user", dir)

	entry, _ = reg.Lookup("demo", "echo")
	if got := entry.Manifest.Runtime.Request.URL; got != "https://second.test/search" {
		t.Errorf("the edit should be in effect, url = %q", got)
	}
}

// Sources are independent, which is what lets a deployment drop in its own
// files without risking the built-in ones.
func TestOneSourceDoesNotDisturbAnother(t *testing.T) {
	reg := plugin.NewRegistry()
	builtinDir := t.TempDir()
	userDir := t.TempDir()

	writePlugin(t, builtinDir, "echo.yaml", strings.Replace(echoPlugin, "%s", "https://builtin.test/s", 1))
	writePlugin(t, userDir, "other.yaml",
		strings.NewReplacer("%s", "https://user.test/s", "id: echo", "id: other").Replace(echoPlugin))

	plugin.LoadDir(reg, "builtin", builtinDir)
	plugin.LoadDir(reg, "user", userDir)

	if len(reg.List("demo")) != 2 {
		t.Fatalf("both sources should contribute, got %d", len(reg.List("demo")))
	}

	// Emptying the user directory must not remove the built-in plugin.
	_ = os.Remove(filepath.Join(userDir, "other.yaml"))
	plugin.LoadDir(reg, "user", userDir)

	if _, ok := reg.Lookup("demo", "echo"); !ok {
		t.Error("the built-in plugin should survive a change to the user directory")
	}
	if _, ok := reg.Lookup("demo", "other"); ok {
		t.Error("the user plugin should be gone")
	}
}

// Configuration is validated against the manifest before anything runs, so a
// plugin author writes no validation code and every plugin reports a missing
// credential the same way.
func TestConfigurationIsValidatedAgainstTheManifest(t *testing.T) {
	dir := t.TempDir()
	reg := plugin.NewRegistry()
	writePlugin(t, dir, "echo.yaml", strings.Replace(echoPlugin, "%s", "https://example.test/s", 1))
	plugin.LoadDir(reg, "user", dir)

	if _, _, err := reg.Configure("demo", "echo", map[string]any{}); err == nil {
		t.Error("a missing required credential should be refused")
	}
	if _, _, err := reg.Configure("demo", "echo", map[string]any{
		"api_key": "k", "flavor": "spicy",
	}); err == nil {
		t.Error("a value outside the declared options should be refused")
	}

	cfg, _, err := reg.Configure("demo", "echo", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("a complete configuration should pass: %v", err)
	}
	if cfg.String("flavor") != "plain" {
		t.Errorf("the declared default should apply, got %q", cfg.String("flavor"))
	}
	if _, leaked := cfg.Redacted()["api_key"]; leaked {
		t.Error("a secret must not survive redaction")
	}
}

// The declarative runtime has to produce the request the manifest describes,
// including typed values in the JSON body.
func TestDeclarativeRuntimeBuildsAndMapsTheCall(t *testing.T) {
	allowLoopback(t)

	var gotBody map[string]any
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"results":[
			{"title":"First","link":"https://example.test/1"},
			{"title":"Second","link":"https://example.test/2"}
		]}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	reg := plugin.NewRegistry()
	writePlugin(t, dir, "echo.yaml", strings.Replace(echoPlugin, "%s", server.URL, 1))
	plugin.LoadDir(reg, "user", dir)

	cfg, entry, err := reg.Configure("demo", "echo", map[string]any{"api_key": "secret", "flavor": "fancy"})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}

	records, err := entry.Invoke(context.Background(), cfg, map[string]any{
		"query":       `quotes "and" braces {}`,
		"max_results": 5,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	// The query goes through as a JSON string, quotes and all. A text template
	// would have produced broken JSON here, which is why the body is
	// structured instead.
	if gotBody["query"] != `quotes "and" braces {}` {
		t.Errorf("query = %#v", gotBody["query"])
	}
	// A lone expression keeps its type, so this is a number rather than "5".
	if count, ok := gotBody["count"].(float64); !ok || count != 5 {
		t.Errorf("count = %#v, want the number 5", gotBody["count"])
	}
	if gotBody["flavor"] != "fancy" {
		t.Errorf("flavor = %#v", gotBody["flavor"])
	}

	if len(records) != 2 {
		t.Fatalf("expected two records, got %d", len(records))
	}
	if records[0].Get("title") != "First" || records[0].Get("url") != "https://example.test/1" {
		t.Errorf("record = %+v", records[0])
	}
}

// A vendor that answers 200 with an error object is common enough that the
// manifest points at it rather than every plugin inventing a check.
func TestDeclaredErrorPathIsReported(t *testing.T) {
	allowLoopback(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error":"quota exceeded"}`))
	}))
	defer server.Close()

	manifest := strings.Replace(echoPlugin, "%s", server.URL, 1) + "    errorPath: error\n"
	dir := t.TempDir()
	reg := plugin.NewRegistry()
	writePlugin(t, dir, "echo.yaml", manifest)
	plugin.LoadDir(reg, "user", dir)

	cfg, entry, err := reg.Configure("demo", "echo", map[string]any{"api_key": "k"})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	_, err = entry.Invoke(context.Background(), cfg, map[string]any{"query": "x", "max_results": 1})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("the declared error should surface, got %v", err)
	}
}

// A manifest is written by hand, so its mistakes are reported when the file
// loads rather than on the first request that happens to use the broken part.
func TestManifestMistakesAreCaughtAtLoadTime(t *testing.T) {
	cases := map[string]string{
		"unknown apiVersion": `
apiVersion: weknora/v99
kind: demo
id: x
name: X
runtime: {type: http, request: {url: https://a.test}}`,

		"expression referring to an undeclared field": `
apiVersion: weknora/v1
kind: demo
id: x
name: X
runtime:
  type: http
  request:
    url: https://a.test
    headers:
      Authorization: Bearer ${config.nope}`,

		"expression with an unknown namespace": `
apiVersion: weknora/v1
kind: demo
id: x
name: X
runtime:
  type: http
  request:
    url: https://a.test/${secrets.leak}`,

		"native runtime naming nothing": `
apiVersion: weknora/v1
kind: demo
id: x
name: X
runtime: {type: native}`,

		"misspelled field": `
apiVersion: weknora/v1
kind: demo
id: x
nmae: X
runtime: {type: http, request: {url: https://a.test}}`,

		"enum without options": `
apiVersion: weknora/v1
kind: demo
id: x
name: X
config:
  - id: mode
    type: enum
runtime: {type: http, request: {url: https://a.test}}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := plugin.ParseManifest([]byte(body)); err == nil {
				t.Fatal("expected the manifest to be rejected")
			}
		})
	}
}

// A native manifest is inert unless the binary actually carries the
// implementation it names, and saying so beats a confusing runtime failure.
func TestNativeManifestNeedsItsImplementation(t *testing.T) {
	dir := t.TempDir()
	reg := plugin.NewRegistry()
	writePlugin(t, dir, "n.yaml", `
apiVersion: weknora/v1
kind: demo
id: scraper
name: Scraper
runtime:
  type: native
  native: scraper
`)
	plugin.LoadDir(reg, "user", dir)

	if _, ok := reg.Lookup("demo", "scraper"); ok {
		t.Fatal("a native plugin with no implementation should not install")
	}
	failures := reg.Failures()
	if len(failures) != 1 || !strings.Contains(failures[0].Err, "scraper") {
		t.Fatalf("the failure should name the missing implementation, got %+v", failures)
	}

	reg.RegisterNative("demo", "scraper", func(_ *plugin.Manifest, _ plugin.Config) (any, error) {
		return "instance", nil
	})
	plugin.LoadDir(reg, "user", dir)

	entry, ok := reg.Lookup("demo", "scraper")
	if !ok {
		t.Fatal("the plugin should install once the implementation exists")
	}
	if !entry.IsNative() {
		t.Error("the entry should be native")
	}
}

func eventually(t *testing.T, limit time.Duration, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
