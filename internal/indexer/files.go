package indexer

import (
	"context"
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type FileIndex struct {
	root    string
	mu      sync.RWMutex
	files   []string
	w       *fsnotify.Watcher
	cancel  context.CancelFunc
	refresh chan struct{}
}

type Match struct {
	Path  string
	Score int
}

func New(root string) *FileIndex { return &FileIndex{root: root, refresh: make(chan struct{}, 1)} }

func (f *FileIndex) Build(ctx context.Context) error {
	files, dirs, err := scan(ctx, f.root)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.files = files
	f.mu.Unlock()
	if f.w == nil {
		w, err := fsnotify.NewWatcher()
		if err == nil {
			f.w = w
			for _, d := range dirs {
				_ = w.Add(d)
			}
			wctx, cancel := context.WithCancel(context.Background())
			f.cancel = cancel
			go f.watch(wctx)
		}
	}
	return nil
}

func (f *FileIndex) Close() error {
	if f.cancel != nil {
		f.cancel()
	}
	if f.w != nil {
		return f.w.Close()
	}
	return nil
}
func (f *FileIndex) Files() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]string(nil), f.files...)
}

func (f *FileIndex) Search(q string, limit int) []Match {
	if limit <= 0 {
		limit = 20
	}
	q = strings.ToLower(strings.TrimSpace(q))
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]Match, 0, limit*2)
	for _, p := range f.files {
		score := fuzzyScore(strings.ToLower(p), q)
		if score < 0 {
			continue
		}
		out = append(out, Match{p, score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return len(out[i].Path) < len(out[j].Path)
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (f *FileIndex) watch(ctx context.Context) {
	var timer *time.Timer
	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case ev, ok := <-f.w.Events:
			if !ok {
				return
			}
			if ev.Name == "" {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(200*time.Millisecond, func() {
				select {
				case f.refresh <- struct{}{}:
				default:
				}
			})
		case <-f.refresh:
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_ = f.rebuild(cctx)
			cancel()
		case <-f.w.Errors:
		}
	}
}
func (f *FileIndex) rebuild(ctx context.Context) error {
	files, dirs, err := scan(ctx, f.root)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.files = files
	f.mu.Unlock()
	if f.w != nil {
		for _, d := range dirs {
			_ = f.w.Add(d)
		}
	}
	return nil
}

func scan(ctx context.Context, root string) ([]string, []string, error) {
	if rg, err := exec.LookPath("rg"); err == nil {
		cmd := exec.CommandContext(ctx, rg, "--files", "--hidden", "--glob", "!.git/**", "--glob", "!node_modules/**", "--glob", "!vendor/**", "--glob", "!build/**", "--glob", "!dist/**")
		cmd.Dir = root
		out, err := cmd.Output()
		if err == nil {
			var files []string
			for _, x := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if x != "" {
					files = append(files, filepath.ToSlash(x))
				}
			}
			sort.Strings(files)
			// fsnotify is not recursive. Build a de-duplicated parent directory set
			// from the rg index so edits in nested packages also invalidate the cache.
			dirSet := map[string]struct{}{root: {}}
			for _, rel := range files {
				d := filepath.Dir(filepath.Join(root, filepath.FromSlash(rel)))
				for {
					dirSet[d] = struct{}{}
					if d == root {
						break
					}
					next := filepath.Dir(d)
					if next == d || !strings.HasPrefix(next, root) {
						break
					}
					d = next
				}
			}
			dirs := make([]string, 0, len(dirSet))
			for d := range dirSet {
				dirs = append(dirs, d)
			}
			sort.Strings(dirs)
			return files, dirs, nil
		}
	}
	var files, dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			if path != root && skip(d.Name()) {
				return filepath.SkipDir
			}
			dirs = append(dirs, path)
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	return files, dirs, err
}
func skip(n string) bool {
	switch n {
	case ".git", "node_modules", "vendor", "build", "dist", ".cache":
		return true
	}
	return strings.HasPrefix(n, ".git")
}
func fuzzyScore(s, q string) int {
	if q == "" {
		return 0
	}
	qi := 0
	score := 0
	last := -2
	for i := 0; i < len(s) && qi < len(q); i++ {
		if s[i] == q[qi] {
			score += 10
			if i == last+1 {
				score += 8
			}
			if i == 0 || s[i-1] == '/' || s[i-1] == '_' || s[i-1] == '-' || s[i-1] == '.' {
				score += 6
			}
			last = i
			qi++
		}
	}
	if qi != len(q) {
		return -1
	}
	score -= len(s) - len(q)
	return score
}
