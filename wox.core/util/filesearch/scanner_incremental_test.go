package filesearch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

func TestScopePathForDirtySignalPreservesFilesystemRootDirectory(t *testing.T) {
	scopePath, ok := scopePathForDirtySignal(string(filepath.Separator), true, true)
	if !ok {
		t.Fatalf("expected filesystem root to produce a scope")
	}
	if scopePath != string(filepath.Separator) {
		t.Fatalf("expected filesystem root to resolve back to %q, got %q", string(filepath.Separator), scopePath)
	}
}
