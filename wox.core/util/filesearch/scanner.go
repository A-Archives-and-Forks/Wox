package filesearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"wox/util"

	"github.com/fsnotify/fsnotify"
)

const (
	defaultScanInterval = 10 * time.Second
	progressBatchSize   = 256
)

type Scanner struct {
	db            *FileSearchDB
	localProvider *LocalIndexProvider
	stopOnce      sync.Once
	stopCh        chan struct{}
	requestCh     chan struct{}
	runningMu     sync.Mutex
	scanRunning   bool
	watcher       *fsnotify.Watcher
	watcherMu     sync.Mutex
}

func NewScanner(db *FileSearchDB, localProvider *LocalIndexProvider) *Scanner {
	return &Scanner{
		db:            db,
		localProvider: localProvider,
		stopCh:        make(chan struct{}),
		requestCh:     make(chan struct{}, 1),
	}
}

func (s *Scanner) Start(ctx context.Context) {
	util.Go(ctx, "filesearch scan loop", func() {
		util.GetLogger().Info(ctx, "filesearch scanner started")
		s.scanAllRoots(ctx)
		s.refreshWatcher(ctx)

		ticker := time.NewTicker(defaultScanInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.scanAllRoots(util.NewTraceContext())
				s.refreshWatcher(util.NewTraceContext())
			case <-s.requestCh:
				s.scanAllRoots(util.NewTraceContext())
				s.refreshWatcher(util.NewTraceContext())
			case <-s.stopCh:
				s.closeWatcher()
				return
			}
		}
	})
}

func (s *Scanner) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *Scanner) RequestRescan() {
	select {
	case s.requestCh <- struct{}{}:
		util.GetLogger().Debug(context.Background(), "filesearch rescan requested")
	default:
	}
}

func (s *Scanner) scanAllRoots(ctx context.Context) {
	s.runningMu.Lock()
	if s.scanRunning {
		s.runningMu.Unlock()
		return
	}
	s.scanRunning = true
	s.runningMu.Unlock()

	defer func() {
		s.runningMu.Lock()
		s.scanRunning = false
		s.runningMu.Unlock()
	}()

	roots, err := s.db.ListRoots(ctx)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to load roots: "+err.Error())
		return
	}
	util.GetLogger().Info(ctx, fmt.Sprintf("filesearch scan cycle started: roots=%d", len(roots)))

	for _, root := range roots {
		s.scanRoot(ctx, root)
	}

	entries, err := s.db.ListEntries(ctx)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to reload entries: "+err.Error())
		return
	}
	s.localProvider.ReplaceEntries(entries)
	util.GetLogger().Info(ctx, fmt.Sprintf("filesearch scan cycle completed: entries=%d", len(entries)))
}

func (s *Scanner) refreshWatcher(ctx context.Context) {
	roots, err := s.db.ListRoots(ctx)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to refresh watcher roots: "+err.Error())
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to create watcher: "+err.Error())
		return
	}

	for _, root := range roots {
		_ = addWatchRecursive(watcher, root.Path)
	}
	util.GetLogger().Info(ctx, fmt.Sprintf("filesearch watcher refreshed: roots=%d", len(roots)))

	s.watcherMu.Lock()
	oldWatcher := s.watcher
	s.watcher = watcher
	s.watcherMu.Unlock()

	if oldWatcher != nil {
		_ = oldWatcher.Close()
	}

	go s.watchLoop(ctx, watcher)
}

func (s *Scanner) watchLoop(ctx context.Context, watcher *fsnotify.Watcher) {
	for {
		select {
		case <-s.stopCh:
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = addWatchRecursive(watcher, event.Name)
				}
			}
			s.RequestRescan()
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			util.GetLogger().Warn(ctx, "filesearch watcher error: "+err.Error())
			s.RequestRescan()
			return
		}
	}
}

func (s *Scanner) closeWatcher() {
	s.watcherMu.Lock()
	defer s.watcherMu.Unlock()
	if s.watcher != nil {
		_ = s.watcher.Close()
		s.watcher = nil
	}
}

func (s *Scanner) scanRoot(ctx context.Context, root RootRecord) {
	startTime := util.GetSystemTimestamp()
	util.GetLogger().Info(ctx, "filesearch scanning root: "+root.Path)
	root.Status = RootStatusScanning
	root.ProgressCurrent = 0
	root.ProgressTotal = 0
	root.LastError = nil
	root.UpdatedAt = util.GetSystemTimestamp()
	_ = s.db.UpdateRootState(ctx, root)

	entries, err := s.collectEntries(ctx, root)
	if err != nil {
		root.Status = RootStatusError
		errMessage := err.Error()
		root.LastError = &errMessage
		root.UpdatedAt = util.GetSystemTimestamp()
		_ = s.db.UpdateRootState(ctx, root)
		util.GetLogger().Warn(ctx, "filesearch failed to scan root "+root.Path+": "+err.Error())
		return
	}

	if err := s.db.ReplaceRootEntries(ctx, root, entries); err != nil {
		root.Status = RootStatusError
		errMessage := err.Error()
		root.LastError = &errMessage
		root.UpdatedAt = util.GetSystemTimestamp()
		_ = s.db.UpdateRootState(ctx, root)
		util.GetLogger().Warn(ctx, "filesearch failed to replace entries for root "+root.Path+": "+err.Error())
		return
	}

	root.Status = RootStatusIdle
	root.ProgressCurrent = int64(len(entries))
	root.ProgressTotal = int64(len(entries))
	root.LastError = nil
	root.UpdatedAt = util.GetSystemTimestamp()
	_ = s.db.UpdateRootState(ctx, root)
	util.GetLogger().Info(ctx, fmt.Sprintf(
		"filesearch scanned root: path=%s entries=%d cost=%dms",
		root.Path,
		len(entries),
		util.GetSystemTimestamp()-startTime,
	))
}

func (s *Scanner) collectEntries(ctx context.Context, root RootRecord) ([]EntryRecord, error) {
	rootPath := filepath.Clean(root.Path)
	rootInfo, err := os.Stat(rootPath)
	if err != nil {
		return nil, err
	}

	entries := []EntryRecord{newEntryRecord(root, rootPath, rootInfo)}
	queue := []scanState{{
		path:     rootPath,
		patterns: nil,
	}}

	count := 0
	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return entries, ctx.Err()
		default:
		}

		state := queue[0]
		queue = queue[1:]

		localPatterns := append([]gitIgnorePattern(nil), state.patterns...)
		localPatterns = append(localPatterns, loadGitIgnorePatterns(state.path)...)

		dirEntries, readErr := os.ReadDir(state.path)
		if readErr != nil {
			if state.path == rootPath {
				return nil, fmt.Errorf("failed to read root directory %s: %w", state.path, readErr)
			}
			util.GetLogger().Warn(ctx, "filesearch skipped unreadable directory "+state.path+": "+readErr.Error())
			continue
		}

		for _, dirEntry := range dirEntries {
			fullPath := filepath.Join(state.path, dirEntry.Name())
			info, infoErr := dirEntry.Info()
			if infoErr != nil {
				continue
			}

			isDir := info.IsDir()
			if shouldSkipSystemPath(fullPath, isDir) {
				continue
			}
			if shouldIgnorePath(localPatterns, fullPath, isDir) {
				continue
			}

			entry := newEntryRecord(root, fullPath, info)
			entries = append(entries, entry)

			if isDir {
				queue = append(queue, scanState{
					path:     fullPath,
					patterns: localPatterns,
				})
			}

			count++
			if count%progressBatchSize == 0 {
				root.ProgressCurrent = int64(len(entries))
				root.UpdatedAt = util.GetSystemTimestamp()
				_ = s.db.UpdateRootState(ctx, root)
				time.Sleep(2 * time.Millisecond)
			}
		}
	}

	return entries, nil
}

type scanState struct {
	path     string
	patterns []gitIgnorePattern
}

type gitIgnorePattern struct {
	baseDir  string
	pattern  string
	negate   bool
	dirOnly  bool
	rooted   bool
	hasSlash bool
}

func loadGitIgnorePatterns(directory string) []gitIgnorePattern {
	gitIgnorePath := filepath.Join(directory, ".gitignore")
	data, err := os.ReadFile(gitIgnorePath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	patterns := make([]gitIgnorePattern, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		pattern := gitIgnorePattern{baseDir: directory}
		if strings.HasPrefix(line, "!") {
			pattern.negate = true
			line = strings.TrimPrefix(line, "!")
		}
		if strings.HasPrefix(line, "/") {
			pattern.rooted = true
			line = strings.TrimPrefix(line, "/")
		}
		if strings.HasSuffix(line, "/") {
			pattern.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		pattern.pattern = line
		pattern.hasSlash = strings.Contains(line, "/")
		if pattern.pattern != "" {
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}

func shouldIgnorePath(patterns []gitIgnorePattern, fullPath string, isDir bool) bool {
	ignored := false
	for _, pattern := range patterns {
		if pattern.matches(fullPath, isDir) {
			ignored = !pattern.negate
		}
	}
	return ignored
}

func (p gitIgnorePattern) matches(fullPath string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}

	relPath, err := filepath.Rel(p.baseDir, fullPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	pattern := filepath.ToSlash(p.pattern)

	if p.rooted || p.hasSlash {
		if ok, _ := filepath.Match(pattern, relPath); ok {
			return true
		}
		if strings.HasPrefix(relPath, pattern+"/") {
			return true
		}
		return false
	}

	for _, segment := range strings.Split(relPath, "/") {
		if ok, _ := filepath.Match(pattern, segment); ok {
			return true
		}
	}

	return false
}

func newEntryRecord(root RootRecord, fullPath string, info os.FileInfo) EntryRecord {
	pinyinFull, pinyinInitials := buildPinyinFields(info.Name())
	return EntryRecord{
		Path:           fullPath,
		RootID:         root.ID,
		ParentPath:     filepath.Dir(fullPath),
		Name:           info.Name(),
		NormalizedName: strings.ToLower(info.Name()),
		NormalizedPath: normalizePath(fullPath),
		PinyinFull:     pinyinFull,
		PinyinInitials: pinyinInitials,
		IsDir:          info.IsDir(),
		Mtime:          info.ModTime().UnixMilli(),
		Size:           info.Size(),
		UpdatedAt:      util.GetSystemTimestamp(),
	}
}

func addWatchRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if shouldSkipSystemPath(path, true) {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}

func shouldSkipSystemPath(fullPath string, isDir bool) bool {
	if !isDir || !util.IsMacOS() {
		return false
	}

	base := strings.ToLower(filepath.Base(fullPath))
	return strings.HasSuffix(base, ".photoslibrary") ||
		strings.HasSuffix(base, ".lrlibrary") ||
		strings.HasSuffix(base, ".lrdata")
}
