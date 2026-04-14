package filesearch

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestScannerScanAllRootsPersistsDirectorySnapshotsAndFullScanTimestamp(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-full-scan")
	levelOnePath := filepath.Join(rootPath, "level-one")
	levelTwoPath := filepath.Join(levelOnePath, "level-two")
	filePath := filepath.Join(levelTwoPath, "target.txt")

	mustWriteTestFile(t, filePath, "target")

	root := RootRecord{
		ID:        "root-full-scan",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	scanner := NewScanner(db, NewLocalIndexProvider())
	scanner.scanAllRoots(ctx)

	rootAfter, err := db.FindRootByID(ctx, root.ID)
	if err != nil {
		t.Fatalf("find root after full scan: %v", err)
	}
	if rootAfter == nil {
		t.Fatalf("expected root %q to exist after full scan", root.ID)
	}
	if rootAfter.LastFullScanAt <= 0 {
		t.Fatalf("expected full scan timestamp to be recorded, got %d", rootAfter.LastFullScanAt)
	}

	directoryCount, err := db.CountDirectoriesByRoot(ctx, root.ID)
	if err != nil {
		t.Fatalf("count directory snapshots by root: %v", err)
	}
	if directoryCount != 3 {
		t.Fatalf("expected 3 live directory snapshots after full scan, got %d", directoryCount)
	}

	directories, err := db.ListDirectoriesByRoot(ctx, root.ID)
	if err != nil {
		t.Fatalf("list directory snapshots after full scan: %v", err)
	}

	seen := map[string]bool{}
	for _, directory := range directories {
		if !directory.Exists {
			t.Fatalf("expected full scan directory snapshot %q to be live", directory.Path)
		}
		seen[directory.Path] = true
	}

	expectedPaths := []string{rootPath, levelOnePath, levelTwoPath}
	for _, expectedPath := range expectedPaths {
		if !seen[expectedPath] {
			t.Fatalf("expected full scan directory snapshot %q to exist", expectedPath)
		}
	}
}

func TestScannerScanAllRootsCapturesFreshRootFeedSnapshot(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-full-scan-snapshot")
	filePath := filepath.Join(rootPath, "target.txt")

	mustWriteTestFile(t, filePath, "target")

	initialCursor := mustEncodeFeedCursorForTest(t, FeedCursor{
		FeedType:  RootFeedTypeFSEvents,
		UpdatedAt: time.Now().Add(-26 * time.Hour).UnixMilli(),
		FSEventID: 12,
	})
	mustInsertRoot(t, ctx, db, RootRecord{
		ID:         "root-full-scan-snapshot",
		Path:       rootPath,
		Kind:       RootKindUser,
		Status:     RootStatusIdle,
		FeedType:   RootFeedTypeFSEvents,
		FeedCursor: initialCursor,
		FeedState:  RootFeedStateUnavailable,
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	expectedCursor := mustEncodeFeedCursorForTest(t, FeedCursor{
		FeedType:  RootFeedTypeFSEvents,
		UpdatedAt: time.Now().UnixMilli(),
		FSEventID: 99,
	})

	scanner := NewScanner(db, NewLocalIndexProvider())
	scanner.changeFeed = newTestSnapshotChangeFeed(func(root RootRecord) (RootFeedSnapshot, error) {
		return RootFeedSnapshot{
			FeedType:   RootFeedTypeFSEvents,
			FeedCursor: expectedCursor,
			FeedState:  RootFeedStateReady,
		}, nil
	})
	scanner.scanAllRoots(ctx)

	rootAfter, err := db.FindRootByID(ctx, "root-full-scan-snapshot")
	if err != nil {
		t.Fatalf("find root after snapshotting full scan: %v", err)
	}
	if rootAfter == nil {
		t.Fatalf("expected root to exist after full scan")
	}
	if rootAfter.FeedType != RootFeedTypeFSEvents {
		t.Fatalf("expected full scan to persist fsevents feed type, got %q", rootAfter.FeedType)
	}
	if rootAfter.FeedCursor != expectedCursor {
		t.Fatalf("expected full scan to persist fresh feed cursor %q, got %q", expectedCursor, rootAfter.FeedCursor)
	}
	if rootAfter.FeedState != RootFeedStateReady {
		t.Fatalf("expected full scan to recover root feed state to ready, got %q", rootAfter.FeedState)
	}
}

func TestNewScannerUsesSpecDirtyQueueDefaults(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	_ = ctx

	scanner := NewScanner(db, NewLocalIndexProvider())

	if scanner.dirtyQueueConfig.SiblingMergeThreshold != 8 {
		t.Fatalf("expected sibling merge threshold 8, got %d", scanner.dirtyQueueConfig.SiblingMergeThreshold)
	}
	if scanner.dirtyQueueConfig.RootEscalationPathThreshold != 512 {
		t.Fatalf("expected root escalation path threshold 512, got %d", scanner.dirtyQueueConfig.RootEscalationPathThreshold)
	}
	if scanner.dirtyQueueConfig.RootEscalationDirectoryRatio != 0.10 {
		t.Fatalf("expected root escalation directory ratio 0.10, got %f", scanner.dirtyQueueConfig.RootEscalationDirectoryRatio)
	}
}

func TestScannerAddWatchForNewDirectoryKeepsRootOnlyFallback(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-watch-list")
	childPath := filepath.Join(rootPath, "child")

	mustMkdirAll(t, childPath)
	mustInsertRoot(t, ctx, db, RootRecord{
		ID:        "root-watch-list",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	})

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	if err := addRootOnlyWatches(watcher, []RootRecord{{
		ID:   "root-watch-list",
		Path: rootPath,
	}}); err != nil {
		t.Fatalf("add root-only watches: %v", err)
	}

	scanner := NewScanner(db, NewLocalIndexProvider())
	if err := scanner.addWatchForNewDirectory(watcher, childPath); err != nil {
		t.Fatalf("add watch for new directory: %v", err)
	}

	watchList := watcher.WatchList()
	if len(watchList) != 1 {
		t.Fatalf("expected root-only fallback to keep exactly one watch, got %d: %#v", len(watchList), watchList)
	}
	if watchList[0] != rootPath {
		t.Fatalf("expected root-only watch list to contain only %q, got %#v", rootPath, watchList)
	}
}

func TestScannerScanAllRootsLeavesExistingSearchResultsAvailableDuringVerification(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-provider-reload")
	filePath := filepath.Join(rootPath, "existing.txt")

	mustWriteTestFile(t, filePath, "existing")
	mustInsertRoot(t, ctx, db, RootRecord{
		ID:        "root-provider-reload",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	})

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.scanAllRoots(ctx)

	results, err := localProvider.Search(context.Background(), SearchQuery{Raw: "existing"}, 10)
	if err != nil {
		t.Fatalf("search local provider after full scan: %v", err)
	}
	if len(results) != 1 || results[0].Path != filePath {
		t.Fatalf("expected provider reload to include %q, got %#v", filePath, results)
	}
}

func TestScannerStartupRestoreLoadsProviderFromDBWithoutFullScanForFreshCursor(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now()
	rootPath := filepath.Join(t.TempDir(), "root-startup-restore-fresh")
	staleFilePath := filepath.Join(rootPath, "stale.txt")
	lastFullScanAt := now.Add(-time.Hour).UnixMilli()

	root := RootRecord{
		ID:             "root-startup-restore-fresh",
		Path:           rootPath,
		Kind:           RootKindUser,
		Status:         RootStatusIdle,
		FeedType:       RootFeedTypeFSEvents,
		FeedCursor:     mustEncodeFeedCursorForTest(t, FeedCursor{FeedType: RootFeedTypeFSEvents, UpdatedAt: now.UnixMilli(), FSEventID: 88}),
		FeedState:      RootFeedStateReady,
		LastFullScanAt: lastFullScanAt,
		CreatedAt:      now.UnixMilli(),
		UpdatedAt:      now.UnixMilli(),
	}
	mustInsertRoot(t, ctx, db, root)

	if err := db.ReplaceRootEntries(ctx, root, []EntryRecord{
		makeTestEntryRecord(root, staleFilePath, false, 42, now),
	}, nil); err != nil {
		t.Fatalf("seed root entries for startup restore: %v", err)
	}

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.changeFeed = newTestSnapshotChangeFeed(nil)

	scanner.startupRestore(ctx)

	results, err := localProvider.Search(context.Background(), SearchQuery{Raw: "stale"}, 10)
	if err != nil {
		t.Fatalf("search local provider after startup restore: %v", err)
	}
	if len(results) != 1 || results[0].Path != staleFilePath {
		t.Fatalf("expected startup restore to load persisted entry %q, got %#v", staleFilePath, results)
	}

	rootAfter, err := db.FindRootByID(ctx, root.ID)
	if err != nil {
		t.Fatalf("find root after startup restore: %v", err)
	}
	if rootAfter == nil {
		t.Fatalf("expected root after startup restore")
	}
	if rootAfter.LastFullScanAt != lastFullScanAt {
		t.Fatalf("expected startup restore to skip full scan and keep LastFullScanAt=%d, got %d", lastFullScanAt, rootAfter.LastFullScanAt)
	}
}

func TestScannerStartupRestoreReconcilesFallbackRoots(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now()
	rootPath := filepath.Join(t.TempDir(), "root-startup-restore-fallback")
	staleFilePath := filepath.Join(rootPath, "stale.txt")
	actualFilePath := filepath.Join(rootPath, "actual.txt")

	mustMkdirAll(t, rootPath)
	mustWriteTestFile(t, actualFilePath, "actual")

	root := RootRecord{
		ID:             "root-startup-restore-fallback",
		Path:           rootPath,
		Kind:           RootKindUser,
		Status:         RootStatusIdle,
		FeedType:       RootFeedTypeFallback,
		FeedState:      RootFeedStateReady,
		LastFullScanAt: now.Add(-2 * time.Hour).UnixMilli(),
		CreatedAt:      now.UnixMilli(),
		UpdatedAt:      now.UnixMilli(),
	}
	mustInsertRoot(t, ctx, db, root)

	if err := db.ReplaceRootEntries(ctx, root, []EntryRecord{
		makeTestEntryRecord(root, staleFilePath, false, 12, now.Add(-time.Minute)),
	}, nil); err != nil {
		t.Fatalf("seed stale fallback entries: %v", err)
	}

	localProvider := NewLocalIndexProvider()
	scanner := NewScanner(db, localProvider)
	scanner.changeFeed = newTestSnapshotChangeFeed(nil)

	scanner.startupRestore(ctx)

	results, err := localProvider.Search(context.Background(), SearchQuery{Raw: "actual"}, 10)
	if err != nil {
		t.Fatalf("search actual file after fallback startup reconcile: %v", err)
	}
	if len(results) != 1 || results[0].Path != actualFilePath {
		t.Fatalf("expected startup restore to reconcile fallback root to %q, got %#v", actualFilePath, results)
	}

	results, err = localProvider.Search(context.Background(), SearchQuery{Raw: "stale"}, 10)
	if err != nil {
		t.Fatalf("search stale file after fallback startup reconcile: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected stale fallback entry %q to be removed after startup reconcile, got %#v", staleFilePath, results)
	}
}
