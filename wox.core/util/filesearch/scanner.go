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
	defaultScanInterval        = 5 * time.Minute
	defaultDirtyDebounceWindow = 750 * time.Millisecond
	progressBatchSize          = 256
	progressUpdateGap          = 250 * time.Millisecond
)

type Scanner struct {
	db                 *FileSearchDB
	localProvider      *LocalIndexProvider
	onStateChange      func(ctx context.Context)
	stopOnce           sync.Once
	stopCh             chan struct{}
	requestCh          chan struct{}
	dirtyCh            chan struct{}
	runningMu          sync.Mutex
	scanRunning        bool
	changeFeed         ChangeFeed
	dirtyQueue         *DirtyQueue
	dirtyQueueConfig   DirtyQueueConfig
	reconciler         *Reconciler
	transientRootMu    sync.RWMutex
	transientRootState *TransientRootState
	transientSyncMu    sync.RWMutex
	transientSyncState *TransientSyncState
}

func NewScanner(db *FileSearchDB, localProvider *LocalIndexProvider) *Scanner {
	dirtyQueueConfig := DirtyQueueConfig{
		DebounceWindow:               defaultDirtyDebounceWindow,
		SiblingMergeThreshold:        8,
		RootEscalationPathThreshold:  512,
		RootEscalationDirectoryRatio: 0.10,
	}

	return &Scanner{
		db:               db,
		localProvider:    localProvider,
		stopCh:           make(chan struct{}),
		requestCh:        make(chan struct{}, 1),
		dirtyCh:          make(chan struct{}, 1),
		changeFeed:       newPlatformChangeFeed(),
		dirtyQueueConfig: dirtyQueueConfig,
		dirtyQueue:       NewDirtyQueue(dirtyQueueConfig),
		reconciler:       NewReconciler(db),
	}
}

func (s *Scanner) SetStateChangeHandler(handler func(ctx context.Context)) {
	s.onStateChange = handler
}

func (s *Scanner) Start(ctx context.Context) {
	util.Go(ctx, "filesearch change feed loop", func() {
		s.changeFeedLoop(ctx)
	})

	util.Go(ctx, "filesearch scan loop", func() {
		util.GetLogger().Info(ctx, "filesearch scanner started")
		s.scanAllRoots(ctx)
		s.refreshChangeFeed(ctx)

		fullScanTimer := time.NewTimer(defaultScanInterval)
		defer fullScanTimer.Stop()

		dirtyTimer := time.NewTimer(time.Hour)
		if !dirtyTimer.Stop() {
			<-dirtyTimer.C
		}
		defer dirtyTimer.Stop()

		for {
			select {
			case <-fullScanTimer.C:
				s.enqueueAllRootsDirty(util.NewTraceContext())
				s.resetDirtyTimer(dirtyTimer)
				fullScanTimer.Reset(defaultScanInterval)
			case <-s.requestCh:
				s.resetDirtyQueue()
				s.scanAllRoots(util.NewTraceContext())
				s.refreshChangeFeed(util.NewTraceContext())
				if !fullScanTimer.Stop() {
					select {
					case <-fullScanTimer.C:
					default:
					}
				}
				fullScanTimer.Reset(defaultScanInterval)
			case <-s.dirtyCh:
				s.resetDirtyTimer(dirtyTimer)
			case <-dirtyTimer.C:
				if err := s.processDirtyQueue(util.NewTraceContext(), time.Now()); err != nil {
					util.GetLogger().Warn(ctx, "filesearch failed to process dirty queue: "+err.Error())
				}
			case <-s.stopCh:
				s.closeChangeFeed()
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

	for index, root := range roots {
		s.scanRoot(ctx, root, index+1, len(roots))
	}

	entries, err := s.db.ListEntries(ctx)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to reload entries: "+err.Error())
		return
	}
	s.localProvider.ReplaceEntries(entries)
	util.GetLogger().Info(ctx, fmt.Sprintf("filesearch scan cycle completed: entries=%d", len(entries)))
}

func (s *Scanner) refreshChangeFeed(ctx context.Context) {
	if s.changeFeed == nil {
		return
	}

	roots, err := s.db.ListRoots(ctx)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to refresh change feed roots: "+err.Error())
		return
	}

	if err := s.changeFeed.Refresh(ctx, roots); err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to refresh change feed: "+err.Error())
		return
	}

	util.GetLogger().Info(ctx, fmt.Sprintf("filesearch change feed refreshed: roots=%d mode=%s", len(roots), s.changeFeed.Mode()))
}

func (s *Scanner) changeFeedLoop(ctx context.Context) {
	if s.changeFeed == nil {
		return
	}

	for {
		select {
		case <-s.stopCh:
			return
		case signal, ok := <-s.changeFeed.Signals():
			if !ok {
				return
			}
			s.handleChangeSignal(util.NewTraceContext(), signal)
		}
	}
}

func (s *Scanner) closeChangeFeed() {
	if s.changeFeed == nil {
		return
	}
	if err := s.changeFeed.Close(); err != nil {
		util.GetLogger().Warn(context.Background(), "filesearch failed to close change feed: "+err.Error())
	}
}

func (s *Scanner) GetTransientRootState() (TransientRootState, bool) {
	s.transientRootMu.RLock()
	defer s.transientRootMu.RUnlock()
	if s.transientRootState == nil {
		return TransientRootState{}, false
	}

	return *s.transientRootState, true
}

func (s *Scanner) GetTransientSyncState() (TransientSyncState, bool) {
	s.transientSyncMu.RLock()
	defer s.transientSyncMu.RUnlock()
	if s.transientSyncState == nil {
		return TransientSyncState{}, false
	}

	return *s.transientSyncState, true
}

func (s *Scanner) setTransientRootState(state TransientRootState) {
	stateCopy := state
	s.transientRootMu.Lock()
	s.transientRootState = &stateCopy
	s.transientRootMu.Unlock()
}

func (s *Scanner) clearTransientRootState(rootID string) {
	s.transientRootMu.Lock()
	defer s.transientRootMu.Unlock()
	if s.transientRootState == nil {
		return
	}
	if rootID == "" || s.transientRootState.Root.ID == rootID {
		s.transientRootState = nil
	}
}

func (s *Scanner) setTransientSyncState(state TransientSyncState) {
	stateCopy := state
	s.transientSyncMu.Lock()
	s.transientSyncState = &stateCopy
	s.transientSyncMu.Unlock()
}

func (s *Scanner) clearTransientSyncState(rootID string) {
	s.transientSyncMu.Lock()
	defer s.transientSyncMu.Unlock()
	if s.transientSyncState == nil {
		return
	}
	if rootID == "" || s.transientSyncState.Root.ID == rootID {
		s.transientSyncState = nil
	}
}

func (s *Scanner) scanRoot(ctx context.Context, root RootRecord, rootIndex int, rootTotal int) {
	startTime := util.GetSystemTimestamp()
	util.GetLogger().Info(ctx, "filesearch scanning root: "+root.Path)
	s.clearTransientRootState(root.ID)
	if root.FeedType == "" {
		root.FeedType = RootFeedTypeFallback
	}
	if root.FeedState == "" {
		root.FeedState = RootFeedStateReady
	}
	root.Status = RootStatusPreparing
	root.ProgressCurrent = 0
	root.ProgressTotal = 0
	root.LastError = nil
	root.UpdatedAt = util.GetSystemTimestamp()
	_ = s.db.UpdateRootState(ctx, root)
	s.setTransientRootState(TransientRootState{
		Root:            root,
		RootIndex:       rootIndex,
		RootTotal:       rootTotal,
		DiscoveredCount: 1,
		DirectoryIndex:  0,
		DirectoryTotal:  1,
		ItemCurrent:     0,
		ItemTotal:       0,
	})
	s.emitStateChange(ctx)

	plan, err := s.buildScanPlan(ctx, root, rootIndex, rootTotal)
	if err != nil {
		root.Status = RootStatusError
		errMessage := err.Error()
		root.LastError = &errMessage
		root.UpdatedAt = util.GetSystemTimestamp()
		_ = s.db.UpdateRootState(ctx, root)
		s.emitStateChange(ctx)
		util.GetLogger().Warn(ctx, "filesearch failed to scan root "+root.Path+": "+err.Error())
		return
	}

	root.Status = RootStatusScanning
	root.ProgressCurrent = 0
	root.ProgressTotal = plan.TotalItems
	root.UpdatedAt = util.GetSystemTimestamp()
	_ = s.db.UpdateRootState(ctx, root)
	s.setTransientRootState(TransientRootState{
		Root:            root,
		RootIndex:       rootIndex,
		RootTotal:       rootTotal,
		DiscoveredCount: 1,
		DirectoryIndex:  0,
		DirectoryTotal:  plan.DirectoryTotal,
		ItemCurrent:     0,
		ItemTotal:       plan.TotalItems,
	})
	s.emitStateChange(ctx)

	entries, err := s.collectEntries(ctx, root, plan, rootIndex, rootTotal)
	if err != nil {
		root.Status = RootStatusError
		errMessage := err.Error()
		root.LastError = &errMessage
		root.UpdatedAt = util.GetSystemTimestamp()
		_ = s.db.UpdateRootState(ctx, root)
		s.clearTransientRootState(root.ID)
		s.emitStateChange(ctx)
		util.GetLogger().Warn(ctx, "filesearch failed to collect entries for root "+root.Path+": "+err.Error())
		return
	}

	s.setTransientRootState(TransientRootState{
		Root: RootRecord{
			ID:              root.ID,
			Path:            root.Path,
			Kind:            root.Kind,
			Status:          RootStatusFinalizing,
			FeedType:        root.FeedType,
			FeedCursor:      root.FeedCursor,
			FeedState:       root.FeedState,
			LastReconcileAt: root.LastReconcileAt,
			LastFullScanAt:  root.LastFullScanAt,
			ProgressCurrent: 0,
			ProgressTotal:   0,
			LastError:       nil,
			CreatedAt:       root.CreatedAt,
			UpdatedAt:       util.GetSystemTimestamp(),
		},
		RootIndex:       rootIndex,
		RootTotal:       rootTotal,
		DiscoveredCount: int64(len(entries)),
		DirectoryIndex:  plan.DirectoryTotal,
		DirectoryTotal:  plan.DirectoryTotal,
		ItemCurrent:     plan.TotalItems,
		ItemTotal:       plan.TotalItems,
	})
	s.emitStateChange(ctx)

	scanTimestamp := util.GetSystemTimestamp()
	directories := buildDirectorySnapshotRecords(root, plan, scanTimestamp)
	if err := s.db.ReplaceRootSnapshot(ctx, root, directories, entries, func(progress ReplaceEntriesProgress) {
		nextRoot := RootRecord{
			ID:              root.ID,
			Path:            root.Path,
			Kind:            root.Kind,
			Status:          RootStatusFinalizing,
			FeedType:        root.FeedType,
			FeedCursor:      root.FeedCursor,
			FeedState:       root.FeedState,
			LastReconcileAt: root.LastReconcileAt,
			LastFullScanAt:  root.LastFullScanAt,
			ProgressCurrent: 0,
			ProgressTotal:   0,
			LastError:       nil,
			CreatedAt:       root.CreatedAt,
			UpdatedAt:       util.GetSystemTimestamp(),
		}

		if progress.Stage == ReplaceEntriesStageWriting {
			nextRoot.Status = RootStatusWriting
			nextRoot.ProgressCurrent = progress.Current
			nextRoot.ProgressTotal = progress.Total
		}

		s.setTransientRootState(TransientRootState{
			Root:            nextRoot,
			RootIndex:       rootIndex,
			RootTotal:       rootTotal,
			DiscoveredCount: int64(len(entries)),
			DirectoryIndex:  plan.DirectoryTotal,
			DirectoryTotal:  plan.DirectoryTotal,
			ItemCurrent:     plan.TotalItems,
			ItemTotal:       plan.TotalItems,
		})
		s.emitStateChange(ctx)
	}); err != nil {
		root.Status = RootStatusError
		errMessage := err.Error()
		root.LastError = &errMessage
		root.UpdatedAt = util.GetSystemTimestamp()
		_ = s.db.UpdateRootState(ctx, root)
		s.clearTransientRootState(root.ID)
		s.emitStateChange(ctx)
		util.GetLogger().Warn(ctx, "filesearch failed to replace entries for root "+root.Path+": "+err.Error())
		return
	}

	root.LastReconcileAt = scanTimestamp
	root.LastFullScanAt = scanTimestamp
	root.Status = RootStatusFinalizing
	root.ProgressCurrent = RootProgressScale
	root.ProgressTotal = RootProgressScale
	root.LastError = nil
	root.FeedState = RootFeedStateReady
	root = s.captureRootFeedSnapshot(ctx, root)
	root.UpdatedAt = util.GetSystemTimestamp()
	_ = s.db.UpdateRootState(ctx, root)
	s.refreshChangeFeed(ctx)
	root.Status = RootStatusIdle
	root.UpdatedAt = util.GetSystemTimestamp()
	_ = s.db.UpdateRootState(ctx, root)
	s.clearTransientRootState(root.ID)
	s.emitStateChange(ctx)
	util.GetLogger().Info(ctx, fmt.Sprintf(
		"filesearch scanned root: path=%s entries=%d cost=%dms",
		root.Path,
		len(entries),
		util.GetSystemTimestamp()-startTime,
	))
}

func (s *Scanner) buildScanPlan(ctx context.Context, root RootRecord, rootIndex int, rootTotal int) (scanPlan, error) {
	rootPath := filepath.Clean(root.Path)
	if _, err := os.Stat(rootPath); err != nil {
		return scanPlan{}, err
	}

	queue := []scanState{{
		path:     rootPath,
		patterns: nil,
	}}
	plannedDirectories := make([]plannedDirectory, 0, 64)
	discoveredDirectories := 1
	processedDirectories := 0
	totalItems := int64(1)
	lastProgressUpdateAt := time.Now()

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return scanPlan{}, ctx.Err()
		default:
		}

		state := queue[0]
		queue = queue[1:]

		dirEntries, readErr := os.ReadDir(state.path)
		if readErr != nil {
			processedDirectories++
			s.updatePlanningProgress(ctx, root, rootIndex, rootTotal, processedDirectories, discoveredDirectories)
			if state.path == rootPath {
				return scanPlan{}, fmt.Errorf("failed to read root directory %s: %w", state.path, readErr)
			}
			util.GetLogger().Warn(ctx, "filesearch skipped unreadable directory "+state.path+": "+readErr.Error())
			continue
		}

		localPatterns := append([]gitIgnorePattern(nil), state.patterns...)
		localPatterns = append(localPatterns, loadGitIgnorePatterns(state.path)...)
		plannedDirectories = append(plannedDirectories, plannedDirectory{
			path:       state.path,
			patterns:   localPatterns,
			childCount: len(dirEntries),
		})
		totalItems += int64(len(dirEntries))
		processedDirectories++

		for _, dirEntry := range dirEntries {
			fullPath := filepath.Join(state.path, dirEntry.Name())
			isDir := dirEntry.IsDir()
			if shouldSkipSystemPath(fullPath, isDir) {
				continue
			}
			if shouldIgnorePath(localPatterns, fullPath, isDir) {
				continue
			}

			if isDir {
				queue = append(queue, scanState{
					path:     fullPath,
					patterns: localPatterns,
				})
				discoveredDirectories++
			}
		}

		if processedDirectories%progressBatchSize == 0 || time.Since(lastProgressUpdateAt) >= progressUpdateGap {
			s.updatePlanningProgress(ctx, root, rootIndex, rootTotal, processedDirectories, discoveredDirectories)
			lastProgressUpdateAt = time.Now()
		}
	}

	s.updatePlanningProgress(ctx, root, rootIndex, rootTotal, processedDirectories, discoveredDirectories)

	return scanPlan{
		directories:    plannedDirectories,
		DirectoryTotal: len(plannedDirectories),
		TotalItems:     totalItems,
	}, nil
}

func (s *Scanner) collectEntries(ctx context.Context, root RootRecord, plan scanPlan, rootIndex int, rootTotal int) ([]EntryRecord, error) {
	rootPath := filepath.Clean(root.Path)
	rootInfo, err := os.Stat(rootPath)
	if err != nil {
		return nil, err
	}

	entries := []EntryRecord{newEntryRecord(root, rootPath, rootInfo)}
	processedItems := int64(1)
	lastReportedItems := int64(0)
	lastProgressUpdateAt := time.Now()

	if len(plan.directories) == 0 {
		s.updateScanProgress(ctx, root, rootIndex, rootTotal, 0, 0, int64(len(entries)), processedItems, plan.TotalItems, &lastReportedItems, true)
		return entries, nil
	}

	for directoryIndex, plannedDirectory := range plan.directories {
		select {
		case <-ctx.Done():
			return entries, ctx.Err()
		default:
		}

		dirEntries, readErr := os.ReadDir(plannedDirectory.path)
		if readErr != nil {
			processedItems += int64(plannedDirectory.childCount)
			s.updateScanProgress(ctx, root, rootIndex, rootTotal, directoryIndex+1, plan.DirectoryTotal, int64(len(entries)), processedItems, plan.TotalItems, &lastReportedItems, true)
			if plannedDirectory.path == rootPath {
				return nil, fmt.Errorf("failed to read root directory %s: %w", plannedDirectory.path, readErr)
			}
			util.GetLogger().Warn(ctx, "filesearch skipped unreadable directory "+plannedDirectory.path+": "+readErr.Error())
			continue
		}

		count := 0
		for _, dirEntry := range dirEntries {
			fullPath := filepath.Join(plannedDirectory.path, dirEntry.Name())
			info, infoErr := dirEntry.Info()
			if infoErr != nil {
				processedItems++
				count++
				if count%progressBatchSize == 0 || time.Since(lastProgressUpdateAt) >= progressUpdateGap {
					s.updateScanProgress(ctx, root, rootIndex, rootTotal, directoryIndex+1, plan.DirectoryTotal, int64(len(entries)), processedItems, plan.TotalItems, &lastReportedItems, false)
					lastProgressUpdateAt = time.Now()
				}
				continue
			}

			isDir := info.IsDir()
			if shouldSkipSystemPath(fullPath, isDir) {
				processedItems++
				count++
				if count%progressBatchSize == 0 || time.Since(lastProgressUpdateAt) >= progressUpdateGap {
					s.updateScanProgress(ctx, root, rootIndex, rootTotal, directoryIndex+1, plan.DirectoryTotal, int64(len(entries)), processedItems, plan.TotalItems, &lastReportedItems, false)
					lastProgressUpdateAt = time.Now()
				}
				continue
			}
			if shouldIgnorePath(plannedDirectory.patterns, fullPath, isDir) {
				processedItems++
				count++
				if count%progressBatchSize == 0 || time.Since(lastProgressUpdateAt) >= progressUpdateGap {
					s.updateScanProgress(ctx, root, rootIndex, rootTotal, directoryIndex+1, plan.DirectoryTotal, int64(len(entries)), processedItems, plan.TotalItems, &lastReportedItems, false)
					lastProgressUpdateAt = time.Now()
				}
				continue
			}

			entries = append(entries, newEntryRecord(root, fullPath, info))
			count++
			processedItems++
			if count%progressBatchSize == 0 || time.Since(lastProgressUpdateAt) >= progressUpdateGap {
				s.updateScanProgress(ctx, root, rootIndex, rootTotal, directoryIndex+1, plan.DirectoryTotal, int64(len(entries)), processedItems, plan.TotalItems, &lastReportedItems, false)
				lastProgressUpdateAt = time.Now()
				time.Sleep(2 * time.Millisecond)
			}
		}

		s.updateScanProgress(ctx, root, rootIndex, rootTotal, directoryIndex+1, plan.DirectoryTotal, int64(len(entries)), processedItems, plan.TotalItems, &lastReportedItems, true)
	}

	return entries, nil
}

func (s *Scanner) updatePlanningProgress(
	ctx context.Context,
	root RootRecord,
	rootIndex int,
	rootTotal int,
	processedDirectories int,
	discoveredDirectories int,
) {
	s.setTransientRootState(TransientRootState{
		Root:            root,
		RootIndex:       rootIndex,
		RootTotal:       rootTotal,
		DiscoveredCount: int64(discoveredDirectories),
		DirectoryIndex:  processedDirectories,
		DirectoryTotal:  discoveredDirectories,
		ItemCurrent:     0,
		ItemTotal:       0,
	})
	s.emitStateChange(ctx)
}

func (s *Scanner) updateScanProgress(
	ctx context.Context,
	root RootRecord,
	rootIndex int,
	rootTotal int,
	directoryIndex int,
	directoryTotal int,
	discoveredCount int64,
	currentItems int64,
	totalItems int64,
	lastReportedProgress *int64,
	force bool,
) {
	if totalItems <= 0 {
		totalItems = 1
	}
	if currentItems < 0 {
		currentItems = 0
	}
	if currentItems > totalItems {
		currentItems = totalItems
	}
	if !force && currentItems <= *lastReportedProgress {
		return
	}

	*lastReportedProgress = currentItems
	root.ProgressCurrent = currentItems
	root.ProgressTotal = totalItems
	root.UpdatedAt = util.GetSystemTimestamp()
	s.setTransientRootState(TransientRootState{
		Root:            root,
		RootIndex:       rootIndex,
		RootTotal:       rootTotal,
		DiscoveredCount: discoveredCount,
		DirectoryIndex:  directoryIndex,
		DirectoryTotal:  directoryTotal,
		ItemCurrent:     currentItems,
		ItemTotal:       totalItems,
	})
	s.emitStateChange(ctx)
}

func (s *Scanner) emitStateChange(ctx context.Context) {
	if s.onStateChange != nil {
		s.onStateChange(ctx)
	}
}

func (s *Scanner) enqueueDirty(signal DirtySignal) {
	normalized, ok := normalizeDirtySignal(signal)
	if !ok {
		return
	}

	if s.dirtyQueue != nil {
		s.dirtyQueue.Push(normalized)
	}
	s.refreshTransientSyncPendingCounts()
	s.emitStateChange(context.Background())

	select {
	case s.dirtyCh <- struct{}{}:
	default:
	}
}

func (s *Scanner) enqueueAllRootsDirty(ctx context.Context) {
	roots, err := s.db.ListRoots(ctx)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to enqueue dirty roots: "+err.Error())
		return
	}

	for _, root := range roots {
		s.enqueueDirty(DirtySignal{
			Kind:          DirtySignalKindRoot,
			RootID:        root.ID,
			Path:          root.Path,
			PathIsDir:     true,
			PathTypeKnown: true,
		})
	}
}

func (s *Scanner) enqueueDirtyForPath(ctx context.Context, path string) bool {
	root, ok := s.findRootForPath(ctx, path)
	if !ok {
		return false
	}

	cleanPath := filepath.Clean(path)
	cleanRootPath := filepath.Clean(root.Path)
	kind := DirtySignalKindPath
	if cleanPath == cleanRootPath || filepath.Dir(cleanPath) == cleanRootPath {
		kind = DirtySignalKindRoot
	}

	pathIsDir := false
	pathTypeKnown := false
	if info, err := os.Stat(cleanPath); err == nil {
		pathIsDir = info.IsDir()
		pathTypeKnown = true
	}

	s.enqueueDirty(DirtySignal{
		Kind:          kind,
		RootID:        root.ID,
		Path:          cleanPath,
		PathIsDir:     pathIsDir,
		PathTypeKnown: pathTypeKnown,
	})
	return true
}

func (s *Scanner) processDirtyQueue(ctx context.Context, now time.Time) error {
	if s.dirtyQueue == nil || s.reconciler == nil {
		return nil
	}

	rootDirectoryCounts, rootsByID, rootIndexByID, err := s.loadDirtyQueueContext(ctx)
	if err != nil {
		return err
	}

	batches := s.dirtyQueue.FlushReady(now, rootDirectoryCounts)
	if len(batches) == 0 {
		return nil
	}

	rootTotal := len(rootsByID)
	for batchIndex, batch := range batches {
		root, ok := rootsByID[batch.RootID]
		if !ok {
			s.refreshTransientSyncPendingCounts()
			continue
		}
		batch = forceReconcileBatchForFeedState(root, batch)

		pendingRootCount, pendingPathCount := s.pendingDirtyCounts()
		s.setTransientSyncState(newTransientSyncState(root, rootIndexByID[batch.RootID], rootTotal, batch, pendingRootCount, pendingPathCount))
		s.emitStateChange(ctx)

		result, err := s.reconciler.Reconcile(ctx, batch)
		if err != nil {
			s.clearTransientSyncState(batch.RootID)
			s.handleDirtyQueueFailure(ctx, root, batch, batches[batchIndex+1:])
			return err
		}

		if result.ReloadNeeded {
			if err := s.reloadLocalProviderFromDB(ctx); err != nil {
				s.clearTransientSyncState(batch.RootID)
				s.handleDirtyQueueFailure(ctx, root, batch, batches[batchIndex+1:])
				return err
			}
		}
		if result.Mode == ReconcileModeRoot {
			s.refreshRootFeedSnapshot(ctx, batch.RootID)
		}

		s.clearTransientSyncState(batch.RootID)
		s.refreshTransientSyncPendingCounts()
		s.emitStateChange(ctx)
	}

	return nil
}

func (s *Scanner) handleChangeSignal(ctx context.Context, signal ChangeSignal) {
	s.updateRootFeedMetadata(ctx, signal.RootID, signal.FeedType, signal.Cursor)

	switch signal.Kind {
	case ChangeSignalKindDirtyRoot:
		s.enqueueDirty(DirtySignal{
			Kind:          DirtySignalKindRoot,
			RootID:        signal.RootID,
			Path:          cleanDirtyQueuePath(signal.Path),
			PathIsDir:     true,
			PathTypeKnown: true,
			At:            signal.At,
		})
	case ChangeSignalKindDirtyPath:
		root, ok := s.findRootByID(ctx, signal.RootID)
		if !ok || root.FeedState == RootFeedStateReady {
			s.enqueueDirty(DirtySignal{
				Kind:          DirtySignalKindPath,
				RootID:        signal.RootID,
				Path:          signal.Path,
				PathIsDir:     signal.PathIsDir,
				PathTypeKnown: signal.PathTypeKnown,
				At:            signal.At,
			})
			return
		}
		s.enqueueDirty(DirtySignal{
			Kind:          DirtySignalKindRoot,
			RootID:        signal.RootID,
			Path:          root.Path,
			PathIsDir:     true,
			PathTypeKnown: true,
			At:            signal.At,
		})
	case ChangeSignalKindRequiresRootReconcile:
		s.updateRootFeedState(ctx, signal.RootID, RootFeedStateDegraded)
		s.enqueueDirty(DirtySignal{
			Kind:          DirtySignalKindRoot,
			RootID:        signal.RootID,
			Path:          signal.Path,
			PathIsDir:     true,
			PathTypeKnown: true,
			At:            signal.At,
		})
	case ChangeSignalKindFeedUnavailable:
		s.updateRootFeedState(ctx, signal.RootID, RootFeedStateUnavailable)
		s.enqueueDirty(DirtySignal{
			Kind:          DirtySignalKindRoot,
			RootID:        signal.RootID,
			Path:          signal.Path,
			PathIsDir:     true,
			PathTypeKnown: true,
			At:            signal.At,
		})
	}
}

func (s *Scanner) handleDirtyQueueFailure(ctx context.Context, root RootRecord, batch ReconcileBatch, remaining []ReconcileBatch) {
	s.updateRootFeedState(ctx, root.ID, RootFeedStateDegraded)
	s.enqueueDirty(DirtySignal{
		Kind:          DirtySignalKindRoot,
		RootID:        batch.RootID,
		Path:          root.Path,
		PathIsDir:     true,
		PathTypeKnown: true,
		At:            time.Now(),
	})
	s.requeueDirtyBatches(remaining)
	s.refreshTransientSyncPendingCounts()
	s.emitStateChange(ctx)
}

func (s *Scanner) updateRootFeedMetadata(ctx context.Context, rootID string, feedType RootFeedType, cursor string) {
	if rootID == "" || (feedType == "" && cursor == "") {
		return
	}

	root, ok := s.findRootByID(ctx, rootID)
	if !ok {
		return
	}

	changed := false
	if feedType != "" && root.FeedType != feedType {
		root.FeedType = feedType
		changed = true
	}
	if cursor != "" && root.FeedCursor != cursor {
		root.FeedCursor = cursor
		changed = true
	}
	if !changed {
		return
	}

	root.UpdatedAt = util.GetSystemTimestamp()
	if err := s.db.UpdateRootState(ctx, root); err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to update root feed metadata: "+err.Error())
	}
}

func (s *Scanner) captureRootFeedSnapshot(ctx context.Context, root RootRecord) RootRecord {
	snapshotter, ok := s.changeFeed.(RootFeedSnapshotter)
	if !ok {
		if root.FeedType == "" {
			root.FeedType = RootFeedTypeFallback
		}
		if root.FeedState == "" {
			root.FeedState = RootFeedStateReady
		}
		return root
	}

	snapshot, err := snapshotter.SnapshotRootFeed(ctx, root)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to capture root feed snapshot: "+err.Error())
		if root.FeedType == "" {
			root.FeedType = RootFeedTypeFallback
		}
		if root.FeedState == "" {
			root.FeedState = RootFeedStateReady
		}
		return root
	}

	if snapshot.FeedType != "" {
		root.FeedType = snapshot.FeedType
	}
	root.FeedCursor = snapshot.FeedCursor
	if snapshot.FeedState != "" {
		root.FeedState = snapshot.FeedState
	}

	return root
}

func (s *Scanner) refreshRootFeedSnapshot(ctx context.Context, rootID string) {
	root, ok := s.findRootByID(ctx, rootID)
	if !ok {
		return
	}

	root = s.captureRootFeedSnapshot(ctx, root)
	root.UpdatedAt = util.GetSystemTimestamp()
	if err := s.db.UpdateRootState(ctx, root); err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to persist refreshed root feed snapshot: "+err.Error())
		return
	}
	s.emitStateChange(ctx)
}

func (s *Scanner) resetDirtyTimer(timer *time.Timer) {
	if timer == nil {
		return
	}

	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(s.dirtyDebounceWindow())
}

func (s *Scanner) dirtyDebounceWindow() time.Duration {
	if s.dirtyQueue != nil && s.dirtyQueue.debounceWindow() > 0 {
		return s.dirtyQueue.debounceWindow()
	}
	return 0
}

func (s *Scanner) resetDirtyQueue() {
	s.clearTransientSyncState("")
	s.dirtyQueue = NewDirtyQueue(s.dirtyQueueConfig)
	s.refreshTransientSyncPendingCounts()
	s.emitStateChange(context.Background())
}

func (s *Scanner) loadDirtyQueueContext(ctx context.Context) (map[string]int, map[string]RootRecord, map[string]int, error) {
	roots, err := s.db.ListRoots(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	rootDirectoryCounts := make(map[string]int, len(roots))
	rootsByID := make(map[string]RootRecord, len(roots))
	rootIndexByID := make(map[string]int, len(roots))
	for index, root := range roots {
		rootsByID[root.ID] = root
		rootIndexByID[root.ID] = index + 1

		directoryCount, err := s.db.CountDirectoriesByRoot(ctx, root.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		rootDirectoryCounts[root.ID] = directoryCount
	}

	return rootDirectoryCounts, rootsByID, rootIndexByID, nil
}

func (s *Scanner) reloadLocalProviderFromDB(ctx context.Context) error {
	entries, err := s.db.ListEntries(ctx)
	if err != nil {
		return err
	}
	s.localProvider.ReplaceEntries(entries)
	return nil
}

func (s *Scanner) findRootByID(ctx context.Context, rootID string) (RootRecord, bool) {
	root, err := s.db.FindRootByID(ctx, rootID)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to resolve root by id: "+err.Error())
		return RootRecord{}, false
	}
	if root == nil {
		return RootRecord{}, false
	}
	return *root, true
}

func (s *Scanner) updateRootFeedState(ctx context.Context, rootID string, state RootFeedState) {
	root, ok := s.findRootByID(ctx, rootID)
	if !ok {
		return
	}
	if root.FeedState == state {
		return
	}
	if root.FeedType == "" {
		root.FeedType = RootFeedTypeFallback
	}
	root.FeedState = state
	root.UpdatedAt = util.GetSystemTimestamp()
	if err := s.db.UpdateRootState(ctx, root); err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to update root feed state: "+err.Error())
		return
	}
	s.emitStateChange(ctx)
}

func forceReconcileBatchForFeedState(root RootRecord, batch ReconcileBatch) ReconcileBatch {
	if batch.Mode == ReconcileModeRoot {
		return batch
	}
	if root.FeedState != RootFeedStateDegraded && root.FeedState != RootFeedStateUnavailable {
		return batch
	}

	batch.Mode = ReconcileModeRoot
	batch.Paths = nil
	return batch
}

func (s *Scanner) pendingDirtyCounts() (int, int) {
	if s.dirtyQueue == nil {
		return 0, 0
	}

	s.dirtyQueue.mu.Lock()
	defer s.dirtyQueue.mu.Unlock()

	pendingRootCount := 0
	pendingPathSet := map[string]struct{}{}
	for _, signals := range s.dirtyQueue.pending {
		if len(signals) == 0 {
			continue
		}

		pendingRootCount++
		for _, signal := range signals {
			if signal.Kind != DirtySignalKindPath {
				continue
			}
			if signal.Path == "" {
				continue
			}
			pendingPathSet[signal.Path] = struct{}{}
		}
	}

	return pendingRootCount, len(pendingPathSet)
}

func (s *Scanner) refreshTransientSyncPendingCounts() {
	pendingRootCount, pendingPathCount := s.pendingDirtyCounts()

	s.transientSyncMu.Lock()
	defer s.transientSyncMu.Unlock()

	if pendingRootCount == 0 && pendingPathCount == 0 {
		if s.transientSyncState == nil || s.transientSyncState.Root.ID == "" {
			s.transientSyncState = nil
		}
		return
	}

	if s.transientSyncState == nil {
		s.transientSyncState = &TransientSyncState{}
	}

	s.transientSyncState.PendingRootCount = pendingRootCount
	s.transientSyncState.PendingPathCount = pendingPathCount
}

func (s *Scanner) requeueDirtyBatches(batches []ReconcileBatch) {
	requeuedAt := time.Now()
	for _, batch := range batches {
		switch batch.Mode {
		case ReconcileModeRoot:
			s.enqueueDirty(DirtySignal{
				Kind:          DirtySignalKindRoot,
				RootID:        batch.RootID,
				PathIsDir:     true,
				PathTypeKnown: true,
				At:            requeuedAt,
			})
		case ReconcileModeSubtree:
			for _, path := range batch.Paths {
				s.enqueueDirty(DirtySignal{
					Kind:          DirtySignalKindPath,
					RootID:        batch.RootID,
					Path:          path,
					PathIsDir:     true,
					PathTypeKnown: true,
					At:            requeuedAt,
				})
			}
		}
	}
}

func (s *Scanner) findRootForPath(ctx context.Context, path string) (RootRecord, bool) {
	roots, err := s.db.ListRoots(ctx)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to resolve dirty root: "+err.Error())
		return RootRecord{}, false
	}

	cleanPath := filepath.Clean(path)
	bestIndex := -1
	bestLength := -1
	for index, root := range roots {
		if !pathWithinScope(root.Path, cleanPath) {
			continue
		}
		if len(root.Path) <= bestLength {
			continue
		}
		bestIndex = index
		bestLength = len(root.Path)
	}

	if bestIndex < 0 {
		return RootRecord{}, false
	}

	return roots[bestIndex], true
}

func newTransientSyncState(root RootRecord, rootIndex int, rootTotal int, batch ReconcileBatch, pendingRootCount int, pendingPathCount int) TransientSyncState {
	progressTotal := int64(batch.DirtyPathCount)
	if progressTotal <= 0 {
		progressTotal = int64(len(batch.Paths))
	}
	if progressTotal <= 0 {
		progressTotal = 1
	}

	root.Status = RootStatusSyncing
	root.ProgressCurrent = 0
	root.ProgressTotal = progressTotal
	root.LastError = nil
	root.UpdatedAt = util.GetSystemTimestamp()

	return TransientSyncState{
		Root:             root,
		RootIndex:        rootIndex,
		RootTotal:        rootTotal,
		Mode:             batch.Mode,
		ScopeCount:       len(batch.Paths),
		DirtyPathCount:   batch.DirtyPathCount,
		PendingRootCount: pendingRootCount,
		PendingPathCount: pendingPathCount,
	}
}

type scanState struct {
	path     string
	patterns []gitIgnorePattern
}

type plannedDirectory struct {
	path       string
	patterns   []gitIgnorePattern
	childCount int
}

type scanPlan struct {
	directories    []plannedDirectory
	DirectoryTotal int
	TotalItems     int64
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

func addRootOnlyWatches(watcher *fsnotify.Watcher, roots []RootRecord) error {
	var watchErrs []string

	for _, root := range roots {
		if err := watcher.Add(root.Path); err != nil {
			watchErrs = append(watchErrs, fmt.Sprintf("%s: %s", root.Path, err.Error()))
		}
	}

	if len(watchErrs) == len(roots) && len(watchErrs) > 0 {
		return fmt.Errorf("%s", strings.Join(watchErrs, "; "))
	}

	for _, watchErr := range watchErrs {
		util.GetLogger().Warn(context.Background(), "filesearch skipped root watcher: "+watchErr)
	}

	return nil
}

func (s *Scanner) addWatchForNewDirectory(watcher *fsnotify.Watcher, directory string) error {
	return nil
}

func buildDirectorySnapshotRecords(root RootRecord, plan scanPlan, scanTimestamp int64) []DirectoryRecord {
	directories := make([]DirectoryRecord, 0, len(plan.directories))
	for _, plannedDirectory := range plan.directories {
		directories = append(directories, DirectoryRecord{
			Path:         plannedDirectory.path,
			RootID:       root.ID,
			ParentPath:   filepath.Dir(plannedDirectory.path),
			LastScanTime: scanTimestamp,
			Exists:       true,
		})
	}
	return directories
}

func shouldSkipSystemPath(fullPath string, isDir bool) bool {
	if !isDir {
		return false
	}

	base := strings.ToLower(filepath.Base(fullPath))

	if base == ".git" || base == ".hg" || base == ".svn" {
		return true
	}

	if !util.IsMacOS() {
		return false
	}

	return strings.HasSuffix(base, ".photoslibrary") ||
		strings.HasSuffix(base, ".lrlibrary") ||
		strings.HasSuffix(base, ".lrdata")
}
