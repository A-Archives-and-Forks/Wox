package filesearch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func makeTestEntryRecord(root RootRecord, fullPath string, isDir bool, size int64, mtime time.Time) EntryRecord {
	name := filepath.Base(fullPath)
	pinyinFull, pinyinInitials := buildPinyinFields(name)

	return EntryRecord{
		Path:           fullPath,
		RootID:         root.ID,
		ParentPath:     filepath.Dir(fullPath),
		Name:           name,
		NormalizedName: normalizeIndexText(name),
		NormalizedPath: normalizePath(fullPath),
		PinyinFull:     pinyinFull,
		PinyinInitials: pinyinInitials,
		IsDir:          isDir,
		Mtime:          mtime.UnixMilli(),
		Size:           size,
		UpdatedAt:      time.Now().UnixMilli(),
	}
}

func TestScannerReloadLocalProviderFromDBRootOnlyRefreshesTargetRoot(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now()
	rootAPath := filepath.Join(t.TempDir(), "root-a-refresh")
	rootBPath := filepath.Join(t.TempDir(), "root-b-refresh")
	rootAInitialFilePath := filepath.Join(rootAPath, "initial-a.txt")
	rootANewFilePath := filepath.Join(rootAPath, "new-a.txt")
	rootBInitialFilePath := filepath.Join(rootBPath, "initial-b.txt")

	mustMkdirAll(t, rootAPath)
	mustMkdirAll(t, rootBPath)
	mustWriteTestFile(t, rootAInitialFilePath, "initial-a")
	mustWriteTestFile(t, rootBInitialFilePath, "initial-b")

	rootA := RootRecord{
		ID:        "root-a-refresh",
		Path:      rootAPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now.UnixMilli(),
		UpdatedAt: now.UnixMilli(),
	}
	rootB := RootRecord{
		ID:        "root-b-refresh",
		Path:      rootBPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now.UnixMilli(),
		UpdatedAt: now.UnixMilli(),
	}
	mustInsertRoot(t, ctx, db, rootA)
	mustInsertRoot(t, ctx, db, rootB)

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.scanAllRoots(ctx)

	mustWriteTestFile(t, rootANewFilePath, "new-a")

	rootANewInfo, err := os.Stat(rootANewFilePath)
	if err != nil {
		t.Fatalf("stat new root-a file: %v", err)
	}

	if err := db.ReplaceRootEntries(ctx, rootA, []EntryRecord{
		makeTestEntryRecord(rootA, rootANewFilePath, rootANewInfo.IsDir(), rootANewInfo.Size(), rootANewInfo.ModTime()),
	}, nil); err != nil {
		t.Fatalf("replace root-a entries: %v", err)
	}

	if _, err := scanner.reloadLocalProviderRootFromDB(ctx, rootA.ID); err != nil {
		t.Fatalf("reload local provider for root-a: %v", err)
	}

	results, err := localProvider.Search(context.Background(), SearchQuery{Raw: "new-a"}, 10)
	if err != nil {
		t.Fatalf("search local provider for reloaded root-a file: %v", err)
	}
	if len(results) != 1 || results[0].Path != rootANewFilePath {
		t.Fatalf("expected root-a new file %q after root reload, got %#v", rootANewFilePath, results)
	}

	results, err = localProvider.Search(context.Background(), SearchQuery{Raw: "initial-a"}, 10)
	if err != nil {
		t.Fatalf("search local provider for removed root-a file: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected removed root-a file %q to be evicted after root reload, got %#v", rootAInitialFilePath, results)
	}

	results, err = localProvider.Search(context.Background(), SearchQuery{Raw: "initial-b"}, 10)
	if err != nil {
		t.Fatalf("search local provider for untouched root-b file: %v", err)
	}
	if len(results) != 1 || results[0].Path != rootBInitialFilePath {
		t.Fatalf("expected untouched root-b file %q after root-a reload, got %#v", rootBInitialFilePath, results)
	}
}

func TestScannerProcessDirtyQueueReloadsLocalProviderAfterReconcile(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-incremental-reload")
	nestedDirPath := filepath.Join(rootPath, "nested")
	initialFilePath := filepath.Join(nestedDirPath, "initial.txt")
	newFilePath := filepath.Join(nestedDirPath, "new.txt")

	mustMkdirAll(t, nestedDirPath)
	mustWriteTestFile(t, initialFilePath, "initial")

	root := RootRecord{
		ID:        "root-incremental-reload",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.dirtyQueueConfig = DirtyQueueConfig{
		DebounceWindow:               defaultDirtyDebounceWindow,
		SiblingMergeThreshold:        8,
		RootEscalationPathThreshold:  512,
		RootEscalationDirectoryRatio: 0,
	}
	scanner.dirtyQueue = NewDirtyQueue(scanner.dirtyQueueConfig)
	engine := &Engine{
		db:            db,
		localProvider: localProvider,
		scanner:       scanner,
	}
	scanner.scanAllRoots(ctx)

	results, err := localProvider.Search(context.Background(), SearchQuery{Raw: "initial"}, 10)
	if err != nil {
		t.Fatalf("search local provider for initial file after full build: %v", err)
	}
	if len(results) != 1 || results[0].Path != initialFilePath {
		t.Fatalf("expected local provider to include initial file %q after full build, got %#v", initialFilePath, results)
	}

	if err := os.Remove(initialFilePath); err != nil {
		t.Fatalf("remove initial file %q: %v", initialFilePath, err)
	}
	mustWriteTestFile(t, newFilePath, "new")
	if ok := scanner.enqueueDirtyForPath(ctx, newFilePath); !ok {
		t.Fatalf("expected scanner to route dirty path %q to root %q", newFilePath, root.ID)
	}
	status, err := engine.GetStatus(ctx)
	if err != nil {
		t.Fatalf("get status after enqueueing dirty path: %v", err)
	}
	if status.PendingDirtyRootCount != 1 || status.PendingDirtyPathCount != 1 {
		t.Fatalf("expected pending dirty counts root=1 path=1 after enqueue, got root=%d path=%d", status.PendingDirtyRootCount, status.PendingDirtyPathCount)
	}
	processAt := time.Now().Add(2 * defaultDirtyDebounceWindow)

	if err := scanner.processDirtyQueue(ctx, processAt); err != nil {
		t.Fatalf("process dirty queue: %v", err)
	}

	results, err = localProvider.Search(context.Background(), SearchQuery{Raw: "new"}, 10)
	if err != nil {
		t.Fatalf("search local provider for new file: %v", err)
	}
	if len(results) != 1 || results[0].Path != newFilePath {
		t.Fatalf("expected local provider to reload new file %q, got %#v", newFilePath, results)
	}

	results, err = localProvider.Search(context.Background(), SearchQuery{Raw: "initial"}, 10)
	if err != nil {
		t.Fatalf("search local provider for removed initial file: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected removed file %q to be evicted from local provider, got %#v", initialFilePath, results)
	}
}

func TestScannerProcessDirtyQueueReloadsDirectChildUnderRoot(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-direct-child")
	initialFilePath := filepath.Join(rootPath, "initial.txt")
	newFilePath := filepath.Join(rootPath, "sync-target.txt")

	mustMkdirAll(t, rootPath)
	mustWriteTestFile(t, initialFilePath, "initial")

	root := RootRecord{
		ID:        "root-direct-child",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.dirtyQueueConfig = DirtyQueueConfig{
		DebounceWindow:               defaultDirtyDebounceWindow,
		SiblingMergeThreshold:        8,
		RootEscalationPathThreshold:  512,
		RootEscalationDirectoryRatio: 0,
	}
	scanner.dirtyQueue = NewDirtyQueue(scanner.dirtyQueueConfig)
	engine := &Engine{
		db:            db,
		localProvider: localProvider,
		scanner:       scanner,
	}
	scanner.scanAllRoots(ctx)

	if err := os.Remove(initialFilePath); err != nil {
		t.Fatalf("remove initial file %q: %v", initialFilePath, err)
	}
	mustWriteTestFile(t, newFilePath, "new")
	if ok := scanner.enqueueDirtyForPath(ctx, newFilePath); !ok {
		t.Fatalf("expected scanner to route direct child dirty path %q to root %q", newFilePath, root.ID)
	}

	status, err := engine.GetStatus(ctx)
	if err != nil {
		t.Fatalf("get status after enqueueing direct child dirty path: %v", err)
	}
	if status.PendingDirtyRootCount != 1 || status.PendingDirtyPathCount != 0 {
		t.Fatalf("expected pending dirty counts root=1 path=0 after direct child enqueue, got root=%d path=%d", status.PendingDirtyRootCount, status.PendingDirtyPathCount)
	}

	if err := scanner.processDirtyQueue(ctx, time.Now().Add(2*defaultDirtyDebounceWindow)); err != nil {
		t.Fatalf("process dirty queue: %v", err)
	}

	results, err := localProvider.Search(context.Background(), SearchQuery{Raw: "sync-target"}, 10)
	if err != nil {
		t.Fatalf("search local provider for direct child file: %v", err)
	}
	if len(results) != 1 || results[0].Path != newFilePath {
		t.Fatalf("expected direct child file %q to be searchable after dirty processing, got %#v", newFilePath, results)
	}
}

func TestScannerProcessDirtyQueueRequeuesRemainingBatchesAfterFailure(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootAPath := filepath.Join(t.TempDir(), "root-a")
	rootBPath := filepath.Join(t.TempDir(), "root-b")
	rootAScopePath := filepath.Join(rootAPath, "scope")
	rootBScopePath := filepath.Join(rootBPath, "scope")
	rootAChildPath := filepath.Join(rootAScopePath, "child")
	rootBChildPath := filepath.Join(rootBScopePath, "child")
	rootAInitialFilePath := filepath.Join(rootAScopePath, "initial-a.txt")
	rootBInitialFilePath := filepath.Join(rootBScopePath, "initial-b.txt")
	rootANewFilePath := filepath.Join(rootAChildPath, "new-a.txt")
	rootBNewFilePath := filepath.Join(rootBChildPath, "new-b.txt")

	mustMkdirAll(t, rootAChildPath)
	mustMkdirAll(t, rootBChildPath)
	mustWriteTestFile(t, rootAInitialFilePath, "initial-a")
	mustWriteTestFile(t, rootBInitialFilePath, "initial-b")

	mustInsertRoot(t, ctx, db, RootRecord{
		ID:        "root-a",
		Path:      rootAPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustInsertRoot(t, ctx, db, RootRecord{
		ID:        "root-b",
		Path:      rootBPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	})

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.dirtyQueueConfig = DirtyQueueConfig{
		DebounceWindow:               defaultDirtyDebounceWindow,
		SiblingMergeThreshold:        8,
		RootEscalationPathThreshold:  512,
		RootEscalationDirectoryRatio: 0,
	}
	scanner.dirtyQueue = NewDirtyQueue(scanner.dirtyQueueConfig)
	engine := &Engine{
		db:            db,
		localProvider: localProvider,
		scanner:       scanner,
	}
	scanner.scanAllRoots(ctx)

	mustWriteTestFile(t, rootANewFilePath, "new-a")
	mustWriteTestFile(t, rootBNewFilePath, "new-b")

	outOfScopePath := filepath.Join(t.TempDir(), "outside-root-a", "broken.txt")
	scanner.enqueueDirty(DirtySignal{
		Kind:   DirtySignalKindPath,
		RootID: "root-a",
		Path:   outOfScopePath,
		At:     time.Now(),
	})
	if ok := scanner.enqueueDirtyForPath(ctx, rootBNewFilePath); !ok {
		t.Fatalf("expected scanner to route dirty path %q", rootBNewFilePath)
	}

	if err := scanner.processDirtyQueue(ctx, time.Now().Add(2*defaultDirtyDebounceWindow)); err == nil {
		t.Fatalf("expected dirty queue processing to fail for out-of-scope root-a batch")
	}

	failedRoot, err := db.FindRootByID(ctx, "root-a")
	if err != nil {
		t.Fatalf("load failed root after dirty queue error: %v", err)
	}
	if failedRoot.FeedState != RootFeedStateDegraded {
		t.Fatalf("expected failed root feed state degraded, got %q", failedRoot.FeedState)
	}

	status, err := engine.GetStatus(ctx)
	if err != nil {
		t.Fatalf("get status after failed dirty queue processing: %v", err)
	}
	if status.PendingDirtyRootCount != 2 || status.PendingDirtyPathCount != 1 {
		t.Fatalf("expected requeued pending dirty counts root=2 path=1 after failure, got root=%d path=%d", status.PendingDirtyRootCount, status.PendingDirtyPathCount)
	}

	if err := scanner.processDirtyQueue(ctx, time.Now().Add(2*defaultDirtyDebounceWindow)); err != nil {
		t.Fatalf("process dirty queue after degraded root requeue: %v", err)
	}

	recoveredRoot, err := db.FindRootByID(ctx, "root-a")
	if err != nil {
		t.Fatalf("load recovered root after retry: %v", err)
	}
	if recoveredRoot.FeedState != RootFeedStateReady {
		t.Fatalf("expected recovered root feed state ready, got %q", recoveredRoot.FeedState)
	}

	results, err := localProvider.Search(context.Background(), SearchQuery{Raw: "new-a"}, 10)
	if err != nil {
		t.Fatalf("search local provider for root-a new file: %v", err)
	}
	if len(results) != 1 || results[0].Path != rootANewFilePath {
		t.Fatalf("expected root-a new file %q after retry, got %#v", rootANewFilePath, results)
	}

	results, err = localProvider.Search(context.Background(), SearchQuery{Raw: "new-b"}, 10)
	if err != nil {
		t.Fatalf("search local provider for root-b new file: %v", err)
	}
	if len(results) != 1 || results[0].Path != rootBNewFilePath {
		t.Fatalf("expected root-b new file %q after retry, got %#v", rootBNewFilePath, results)
	}
}

func TestScannerProcessDirtyQueueCapturesFreshCursorAfterRootReconcile(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-reconcile-snapshot")
	initialFilePath := filepath.Join(rootPath, "initial.txt")
	updatedFilePath := filepath.Join(rootPath, "updated.txt")

	mustWriteTestFile(t, initialFilePath, "initial")

	expectedCursor := mustEncodeFeedCursorForTest(t, FeedCursor{
		FeedType:  RootFeedTypeFSEvents,
		UpdatedAt: time.Now().UnixMilli(),
		FSEventID: 222,
	})

	mustInsertRoot(t, ctx, db, RootRecord{
		ID:       "root-reconcile-snapshot",
		Path:     rootPath,
		Kind:     RootKindUser,
		Status:   RootStatusIdle,
		FeedType: RootFeedTypeFSEvents,
		FeedCursor: mustEncodeFeedCursorForTest(t, FeedCursor{
			FeedType:  RootFeedTypeFSEvents,
			UpdatedAt: time.Now().Add(-time.Hour).UnixMilli(),
			FSEventID: 100,
		}),
		FeedState: RootFeedStateUnavailable,
		CreatedAt: now,
		UpdatedAt: now,
	})

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.changeFeed = newTestSnapshotChangeFeed(func(root RootRecord) (RootFeedSnapshot, error) {
		return RootFeedSnapshot{
			FeedType:   RootFeedTypeFSEvents,
			FeedCursor: expectedCursor,
			FeedState:  RootFeedStateReady,
		}, nil
	})
	scanner.scanAllRoots(ctx)

	if err := os.Remove(initialFilePath); err != nil {
		t.Fatalf("remove initial file: %v", err)
	}
	mustWriteTestFile(t, updatedFilePath, "updated")

	scanner.updateRootFeedState(ctx, "root-reconcile-snapshot", RootFeedStateUnavailable)
	scanner.enqueueDirty(DirtySignal{
		Kind:          DirtySignalKindRoot,
		RootID:        "root-reconcile-snapshot",
		Path:          rootPath,
		PathIsDir:     true,
		PathTypeKnown: true,
		At:            time.Now(),
	})

	if err := scanner.processDirtyQueue(ctx, time.Now().Add(2*defaultDirtyDebounceWindow)); err != nil {
		t.Fatalf("process dirty queue root reconcile: %v", err)
	}

	rootAfter, err := db.FindRootByID(ctx, "root-reconcile-snapshot")
	if err != nil {
		t.Fatalf("find root after root reconcile: %v", err)
	}
	if rootAfter == nil {
		t.Fatalf("expected root after root reconcile")
	}
	if rootAfter.FeedCursor != expectedCursor {
		t.Fatalf("expected root reconcile to persist fresh feed cursor %q, got %q", expectedCursor, rootAfter.FeedCursor)
	}
	if rootAfter.FeedState != RootFeedStateReady {
		t.Fatalf("expected root reconcile to recover feed state to ready, got %q", rootAfter.FeedState)
	}
}

func TestScannerReloadLocalProviderRootFromDBSerializesConcurrentSameRootRefreshes(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now()
	rootPath := filepath.Join(t.TempDir(), "root-queued-refresh")
	initialFilePath := filepath.Join(rootPath, "initial.txt")
	firstRefreshFilePath := filepath.Join(rootPath, "queued-first.txt")
	secondRefreshFilePath := filepath.Join(rootPath, "queued-second.txt")

	mustMkdirAll(t, rootPath)
	mustWriteTestFile(t, initialFilePath, "initial")
	mustWriteTestFile(t, firstRefreshFilePath, "queued-first")
	mustWriteTestFile(t, secondRefreshFilePath, "queued-second")

	root := RootRecord{
		ID:        "root-queued-refresh",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now.UnixMilli(),
		UpdatedAt: now.UnixMilli(),
	}
	mustInsertRoot(t, ctx, db, root)

	initialInfo, err := os.Stat(initialFilePath)
	if err != nil {
		t.Fatalf("stat initial file: %v", err)
	}
	if err := db.ReplaceRootEntries(ctx, root, []EntryRecord{
		makeTestEntryRecord(root, initialFilePath, initialInfo.IsDir(), initialInfo.Size(), initialInfo.ModTime()),
	}, nil); err != nil {
		t.Fatalf("replace initial root entries: %v", err)
	}

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	defer scanner.Stop()

	if _, err := scanner.reloadLocalProviderRootFromDB(ctx, root.ID); err != nil {
		t.Fatalf("initial root reload: %v", err)
	}

	firstRefreshInfo, err := os.Stat(firstRefreshFilePath)
	if err != nil {
		t.Fatalf("stat first refresh file: %v", err)
	}
	if err := db.ReplaceRootEntries(ctx, root, []EntryRecord{
		makeTestEntryRecord(root, firstRefreshFilePath, firstRefreshInfo.IsDir(), firstRefreshInfo.Size(), firstRefreshInfo.ModTime()),
	}, nil); err != nil {
		t.Fatalf("replace root entries for first refresh: %v", err)
	}

	firstApplyReached := make(chan struct{})
	secondApplyReached := make(chan struct{})
	releaseFirstApply := make(chan struct{})
	var applyCallMu sync.Mutex
	applyCalls := 0
	scanner.beforeApplyRootReload = func(reloadRootID string, entries []EntryRecord) {
		if reloadRootID != root.ID {
			return
		}

		applyCallMu.Lock()
		applyCalls++
		callIndex := applyCalls
		applyCallMu.Unlock()

		switch callIndex {
		case 1:
			close(firstApplyReached)
			<-releaseFirstApply
		case 2:
			close(secondApplyReached)
		}
	}

	firstReloadDone := make(chan error, 1)
	go func() {
		_, err := scanner.reloadLocalProviderRootFromDB(ctx, root.ID)
		firstReloadDone <- err
	}()

	select {
	case <-firstApplyReached:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first root reload to reach apply")
	}

	secondRefreshInfo, err := os.Stat(secondRefreshFilePath)
	if err != nil {
		t.Fatalf("stat second refresh file: %v", err)
	}
	if err := db.ReplaceRootEntries(ctx, root, []EntryRecord{
		makeTestEntryRecord(root, secondRefreshFilePath, secondRefreshInfo.IsDir(), secondRefreshInfo.Size(), secondRefreshInfo.ModTime()),
	}, nil); err != nil {
		t.Fatalf("replace root entries for second refresh: %v", err)
	}

	secondReloadDone := make(chan error, 1)
	go func() {
		_, err := scanner.reloadLocalProviderRootFromDB(ctx, root.ID)
		secondReloadDone <- err
	}()

	select {
	case <-secondApplyReached:
		t.Fatalf("expected same-root reload requests to remain queued behind the in-flight apply")
	case <-time.After(150 * time.Millisecond):
	}

	close(releaseFirstApply)

	select {
	case err := <-firstReloadDone:
		if err != nil {
			t.Fatalf("first concurrent root reload: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first concurrent root reload")
	}

	select {
	case err := <-secondReloadDone:
		if err != nil {
			t.Fatalf("second concurrent root reload: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for second concurrent root reload")
	}

	results, err := localProvider.Search(context.Background(), SearchQuery{Raw: "queued-second"}, 10)
	if err != nil {
		t.Fatalf("search local provider for latest root refresh file: %v", err)
	}
	if len(results) != 1 || results[0].Path != secondRefreshFilePath {
		t.Fatalf("expected latest root refresh file %q, got %#v", secondRefreshFilePath, results)
	}

	results, err = localProvider.Search(context.Background(), SearchQuery{Raw: "queued-first"}, 10)
	if err != nil {
		t.Fatalf("search local provider for stale root refresh file: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected stale root refresh file %q to be absent, got %#v", firstRefreshFilePath, results)
	}
}

func TestScopePathForDirtySignalPreservesFilesystemRootDirectory(t *testing.T) {
	scopePath, ok := scopePathForDirtySignal(string(filepath.Separator), true, true)
	if !ok {
		t.Fatalf("expected filesystem root to produce a scope")
	}
	if scopePath != string(filepath.Separator) {
		t.Fatalf("expected filesystem root to resolve back to %q, got %q", string(filepath.Separator), scopePath)
	}
}

func TestScannerRootReloadWorkerExpiresAfterIdleTimeout(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now()
	rootPath := filepath.Join(t.TempDir(), "root-worker-idle")
	filePath := filepath.Join(rootPath, "idle.txt")

	mustMkdirAll(t, rootPath)
	mustWriteTestFile(t, filePath, "idle")

	root := RootRecord{
		ID:        "root-worker-idle",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now.UnixMilli(),
		UpdatedAt: now.UnixMilli(),
	}
	mustInsertRoot(t, ctx, db, root)

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat idle test file: %v", err)
	}
	if err := db.ReplaceRootEntries(ctx, root, []EntryRecord{
		makeTestEntryRecord(root, filePath, info.IsDir(), info.Size(), info.ModTime()),
	}, nil); err != nil {
		t.Fatalf("replace root entries for idle worker test: %v", err)
	}

	scanner := NewScanner(db, NewLocalIndexProvider())
	scanner.rootReloadWorkerIdleTimeout = 20 * time.Millisecond
	defer scanner.Stop()

	if _, err := scanner.reloadLocalProviderRootFromDB(ctx, root.ID); err != nil {
		t.Fatalf("initial root reload: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		scanner.reloadWorkersMu.Lock()
		workerCount := len(scanner.reloadWorkers)
		scanner.reloadWorkersMu.Unlock()
		if workerCount == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	scanner.reloadWorkersMu.Lock()
	workerCount := len(scanner.reloadWorkers)
	scanner.reloadWorkersMu.Unlock()
	t.Fatalf("expected idle reload worker to expire, found %d workers", workerCount)
}

func TestScannerQueuesDirtySignalsForNextRunDuringExecution(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-queue-next-run")
	nestedPath := filepath.Join(rootPath, "nested")
	initialFilePath := filepath.Join(nestedPath, "initial.txt")
	firstFilePath := filepath.Join(nestedPath, "first.txt")
	secondFilePath := filepath.Join(nestedPath, "second.txt")

	mustMkdirAll(t, nestedPath)
	mustWriteTestFile(t, initialFilePath, "initial")

	root := RootRecord{
		ID:        "root-queue-next-run",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.plannerBudgetOverride = &splitBudget{
		LeafEntryBudget:     3,
		LeafWriteBudget:     3,
		LeafMemoryBudget:    1 << 20,
		DirectFileBatchSize: 1,
	}
	engine := &Engine{db: db, localProvider: localProvider, scanner: scanner}
	scanner.scanAllRoots(ctx)

	mustWriteTestFile(t, firstFilePath, "first")
	if ok := scanner.enqueueDirtyForPath(ctx, firstFilePath); !ok {
		t.Fatalf("expected scanner to route first dirty path %q", firstFilePath)
	}

	var (
		queuedSecondMu sync.Mutex
		queuedSecond   bool
	)
	scanner.SetStateChangeHandler(func(changeCtx context.Context) {
		status, err := engine.GetStatus(changeCtx)
		if err != nil {
			t.Fatalf("get status during incremental run: %v", err)
		}
		if status.ActiveStage != RunStageExecuting || status.ActiveRunStatus != RunStatusExecuting {
			return
		}
		queuedSecondMu.Lock()
		if queuedSecond {
			queuedSecondMu.Unlock()
			return
		}
		queuedSecond = true
		queuedSecondMu.Unlock()

		mustWriteTestFile(t, secondFilePath, "second")
		if ok := scanner.enqueueDirtyForPath(changeCtx, secondFilePath); !ok {
			t.Fatalf("expected scanner to route second dirty path %q", secondFilePath)
		}
	})

	if err := scanner.processDirtyQueue(ctx, time.Now().Add(2*defaultDirtyDebounceWindow)); err != nil {
		t.Fatalf("process first incremental run: %v", err)
	}

	status, err := engine.GetStatus(ctx)
	if err != nil {
		t.Fatalf("get status after first incremental run: %v", err)
	}
	if status.PendingDirtyRootCount != 1 || status.PendingDirtyPathCount != 1 {
		t.Fatalf("expected queued second signal for next run, got roots=%d paths=%d", status.PendingDirtyRootCount, status.PendingDirtyPathCount)
	}

	results, err := localProvider.Search(context.Background(), SearchQuery{Raw: "first"}, 10)
	if err != nil {
		t.Fatalf("search for first file after first incremental run: %v", err)
	}
	if len(results) != 1 || results[0].Path != firstFilePath {
		t.Fatalf("expected first file %q after first incremental run, got %#v", firstFilePath, results)
	}

	if err := scanner.processDirtyQueue(ctx, time.Now().Add(4*defaultDirtyDebounceWindow)); err != nil {
		t.Fatalf("process second incremental run: %v", err)
	}

	status, err = engine.GetStatus(ctx)
	if err != nil {
		t.Fatalf("get status after second incremental run: %v", err)
	}
	if status.PendingDirtyRootCount != 0 || status.PendingDirtyPathCount != 0 {
		t.Fatalf("expected dirty queue to drain after second run, got roots=%d paths=%d", status.PendingDirtyRootCount, status.PendingDirtyPathCount)
	}

	results, err = localProvider.Search(context.Background(), SearchQuery{Raw: "second"}, 10)
	if err != nil {
		t.Fatalf("search for second file after second incremental run: %v", err)
	}
	if len(results) != 1 || results[0].Path != secondFilePath {
		t.Fatalf("expected second file %q after second incremental run, got %#v", secondFilePath, results)
	}
}

func TestScannerIncrementalRunFailsFastAndKeepsQueue(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootAPath := filepath.Join(t.TempDir(), "root-a-fast-fail")
	rootBPath := filepath.Join(t.TempDir(), "root-b-fast-fail")
	rootAChildPath := filepath.Join(rootAPath, "child")
	rootBChildPath := filepath.Join(rootBPath, "child")
	rootAFilePath := filepath.Join(rootAChildPath, "initial-a.txt")
	rootBFilePath := filepath.Join(rootBChildPath, "initial-b.txt")
	rootBNewFilePath := filepath.Join(rootBChildPath, "new-b.txt")

	mustMkdirAll(t, rootAChildPath)
	mustMkdirAll(t, rootBChildPath)
	mustWriteTestFile(t, rootAFilePath, "initial-a")
	mustWriteTestFile(t, rootBFilePath, "initial-b")

	mustInsertRoot(t, ctx, db, RootRecord{ID: "root-a-fast-fail", Path: rootAPath, Kind: RootKindUser, Status: RootStatusIdle, CreatedAt: now, UpdatedAt: now})
	mustInsertRoot(t, ctx, db, RootRecord{ID: "root-b-fast-fail", Path: rootBPath, Kind: RootKindUser, Status: RootStatusIdle, CreatedAt: now, UpdatedAt: now})

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.dirtyQueueConfig = DirtyQueueConfig{
		DebounceWindow:               defaultDirtyDebounceWindow,
		SiblingMergeThreshold:        8,
		RootEscalationPathThreshold:  512,
		RootEscalationDirectoryRatio: 0,
	}
	scanner.dirtyQueue = NewDirtyQueue(scanner.dirtyQueueConfig)
	engine := &Engine{db: db, localProvider: localProvider, scanner: scanner}
	scanner.scanAllRoots(ctx)

	mustWriteTestFile(t, rootBNewFilePath, "new-b")
	outOfScopePath := filepath.Join(t.TempDir(), "outside-root-a-fast-fail", "broken.txt")
	scanner.enqueueDirty(DirtySignal{
		Kind:   DirtySignalKindPath,
		RootID: "root-a-fast-fail",
		Path:   outOfScopePath,
		At:     time.Now(),
	})
	if ok := scanner.enqueueDirtyForPath(ctx, rootBNewFilePath); !ok {
		t.Fatalf("expected scanner to route dirty path %q", rootBNewFilePath)
	}

	if err := scanner.processDirtyQueue(ctx, time.Now().Add(2*defaultDirtyDebounceWindow)); err == nil {
		t.Fatal("expected incremental run to fail fast")
	}

	status, err := engine.GetStatus(ctx)
	if err != nil {
		t.Fatalf("get status after failed incremental run: %v", err)
	}
	if status.PendingDirtyRootCount != 2 || status.PendingDirtyPathCount != 1 {
		t.Fatalf("expected failed incremental run to preserve queue, got roots=%d paths=%d", status.PendingDirtyRootCount, status.PendingDirtyPathCount)
	}

	failedRoot, err := db.FindRootByID(ctx, "root-a-fast-fail")
	if err != nil {
		t.Fatalf("load failed root after incremental failure: %v", err)
	}
	if failedRoot == nil || failedRoot.FeedState != RootFeedStateDegraded {
		t.Fatalf("expected failed root feed state degraded, got %#v", failedRoot)
	}
}

func TestScannerIncrementalPermissionFailureStopsHotLoopingFailedRoot(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-permission-stop")
	root := RootRecord{
		ID:        "root-permission-stop",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	scanner := NewScanner(db, nil)
	engine := &Engine{db: db, scanner: scanner}
	batches := []ReconcileBatch{{
		RootID: root.ID,
		Mode:   ReconcileModeRoot,
	}}

	scanner.handleIncrementalRunFailure(ctx, []RootRecord{root}, batches, &runRootError{
		RootID: root.ID,
		Err:    &os.PathError{Op: "open", Path: filepath.Join(rootPath, "CSC"), Err: os.ErrPermission},
	})

	status, err := engine.GetStatus(ctx)
	if err != nil {
		t.Fatalf("get status after permission failure: %v", err)
	}
	if status.PendingDirtyRootCount != 0 || status.PendingDirtyPathCount != 0 {
		t.Fatalf("expected permission failure to stop requeueing failed root, got roots=%d paths=%d", status.PendingDirtyRootCount, status.PendingDirtyPathCount)
	}

	failedRoot, err := db.FindRootByID(ctx, root.ID)
	if err != nil {
		t.Fatalf("load failed root after permission failure: %v", err)
	}
	if failedRoot == nil {
		t.Fatal("expected failed root after permission failure")
	}
	if failedRoot.FeedState != RootFeedStateDegraded {
		t.Fatalf("expected permission failure to degrade feed state, got %#v", failedRoot)
	}
	if failedRoot.Status != RootStatusError {
		t.Fatalf("expected permission failure to persist root error status, got %#v", failedRoot)
	}
	if failedRoot.LastError == nil || *failedRoot.LastError == "" {
		t.Fatalf("expected permission failure to persist last error, got %#v", failedRoot)
	}
}

func TestRunPlannerIncrementalFailureKeepsActualFailedRootID(t *testing.T) {
	_, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootAPath := filepath.Join(t.TempDir(), "root-planner-failure-a")
	rootBPath := filepath.Join(t.TempDir(), "root-planner-failure-b")
	rootAFilePath := filepath.Join(rootAPath, "ok.txt")
	rootBFilePath := filepath.Join(rootBPath, "bad.txt")

	mustWriteTestFile(t, rootAFilePath, "ok")
	mustWriteTestFile(t, rootBFilePath, "bad")

	rootA := RootRecord{ID: "root-planner-failure-a", Path: rootAPath, Kind: RootKindUser, Status: RootStatusIdle, CreatedAt: now, UpdatedAt: now}
	rootB := RootRecord{ID: "root-planner-failure-b", Path: rootBPath, Kind: RootKindUser, Status: RootStatusIdle, CreatedAt: now, UpdatedAt: now}
	planner := NewRunPlanner(newPolicyState(Policy{}))
	_, err := planner.PlanIncrementalRun(ctx, []RootRecord{rootA, rootB}, []ReconcileBatch{
		{
			RootID: rootA.ID,
			Mode:   ReconcileModeSubtree,
			Paths:  []string{rootAPath},
		},
		{
			RootID: rootB.ID,
			Mode:   ReconcileModeSubtree,
			Paths:  []string{rootBFilePath},
		},
	})
	if err == nil {
		t.Fatal("expected incremental planner failure for file-scoped subtree path")
	}

	var rootErr *runRootError
	if !errors.As(err, &rootErr) || rootErr == nil {
		t.Fatalf("expected runRootError from incremental planner, got %T: %v", err, err)
	}
	if got, want := rootErr.RootID, rootB.ID; got != want {
		t.Fatalf("expected planner failure to keep actual failed root id, got %q want %q", got, want)
	}
}
