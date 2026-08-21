package plugin

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/fsnotify/fsnotify"
)

// Loading is where "hot" actually happens: a directory is read into the
// registry, and then watched, so writing a file installs a plugin and deleting
// one uninstalls it while the server keeps serving.
//
// The loader is written on the assumption that it is reading files a human
// just edited, possibly badly, possibly halfway through saving. It never
// aborts a scan on one bad file, it debounces so a save that arrives as three
// filesystem events is one reload, and it keeps the previous set in effect
// until a new one is fully prepared.

// SourceBuiltin names the plugins compiled into the binary.
const SourceBuiltin = "builtin"

// LoadFS reads every manifest under a filesystem and installs them into the
// registry as one source.
//
// It returns the number loaded. Failures do not stop the scan: they are
// recorded against the source and reported through Registry.Failures, because
// one malformed file should cost one plugin, not all of them.
func LoadFS(reg *Registry, source string, fsys fs.FS, root string) int {
	var manifests []*Manifest
	var failures []LoadError

	walkErr := fs.WalkDir(fsys, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			failures = append(failures, LoadError{Source: path, Err: err.Error()})
			return nil
		}
		if entry.IsDir() || !isManifestFile(path) {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			failures = append(failures, LoadError{Source: path, Err: err.Error()})
			return nil
		}
		manifest, err := ParseManifest(data)
		if err != nil {
			failures = append(failures, LoadError{Source: path, Err: err.Error()})
			return nil
		}
		manifest.Source = path
		manifests = append(manifests, manifest)
		return nil
	})
	if walkErr != nil {
		failures = append(failures, LoadError{Source: root, Err: walkErr.Error()})
	}

	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Source < manifests[j].Source })
	reg.Replace(source, manifests, failures)
	return len(manifests)
}

func isManifestFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

// watchDebounce collapses the burst of events an editor produces when saving.
// Writing a file commonly emits create, write, and chmod in quick succession,
// and reloading three times would be wasted work and noisy logs.
const watchDebounce = 300 * time.Millisecond

// Watcher reloads a directory into the registry whenever its contents change.
type Watcher struct {
	registry *Registry
	source   string
	dir      string

	mu      sync.Mutex
	watcher *fsnotify.Watcher
}

// NewWatcher prepares a watcher for a plugin directory.
func NewWatcher(reg *Registry, source, dir string) *Watcher {
	return &Watcher{registry: reg, source: source, dir: dir}
}

// Start loads the directory once and then watches it until the context ends.
//
// A missing directory is not an error: a deployment that installs no plugins
// simply has none, and creating the directory later starts working without a
// restart because the parent is watched too.
func (w *Watcher) Start(ctx context.Context) error {
	count := LoadDir(w.registry, w.source, w.dir)
	logger.Infof(ctx, "[Plugin] loaded %d plugin(s) from %s", count, w.dir)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.watcher = watcher
	w.mu.Unlock()

	if err := watcher.Add(w.dir); err != nil {
		// Watching the parent means a directory created after startup is still
		// picked up, which is the difference between "install plugins before
		// you boot" and "install plugins whenever".
		parent := filepath.Dir(w.dir)
		if parentErr := watcher.Add(parent); parentErr != nil {
			watcher.Close()
			return err
		}
		logger.Infof(ctx, "[Plugin] %s does not exist yet; watching %s for it", w.dir, parent)
	}

	go w.run(ctx, watcher)
	return nil
}

func (w *Watcher) run(ctx context.Context, watcher *fsnotify.Watcher) {
	defer watcher.Close()

	var timer *time.Timer
	var pending <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if !isManifestFile(event.Name) && event.Op&fsnotify.Create == 0 {
				continue
			}
			// A newly created directory needs watching too, so plugins can be
			// organized into subdirectories.
			if event.Op&fsnotify.Create != 0 {
				_ = watcher.Add(event.Name)
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(watchDebounce)
			pending = timer.C

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logger.Warnf(ctx, "[Plugin] watch error on %s: %v", w.dir, err)

		case <-pending:
			pending = nil
			count := LoadDir(w.registry, w.source, w.dir)
			failures := w.registry.Failures()
			if len(failures) > 0 {
				for _, failure := range failures {
					logger.Warnf(ctx, "[Plugin] %s was not loaded: %s", failure.Source, failure.Err)
				}
			}
			logger.Infof(ctx, "[Plugin] reloaded %s: %d plugin(s) in effect", w.dir, count)
		}
	}
}

// LoadDir reads a directory from the real filesystem into the registry.
func LoadDir(reg *Registry, source, dir string) int {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		absolute = dir
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		// The directory may simply not exist yet. Install nothing from this
		// source rather than failing, and leave the other sources untouched.
		reg.Replace(source, nil, nil)
		return 0
	}
	defer root.Close()
	return LoadFS(reg, source, root.FS(), ".")
}
