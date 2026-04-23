package filesearch

import (
	"context"
	"fmt"
	"time"

	"wox/util"
)

func (s *Scanner) startupRestore(ctx context.Context) {
	persistedEntryCount := int64(0)

	if s.localProvider != nil {
		restoredEntries, err := s.reloadLocalProviderFromDB(ctx)
		if err != nil {
			util.GetLogger().Warn(ctx, "filesearch startup restore failed to load persisted entries: "+err.Error())
			s.scanAllRootsWithReason(ctx, "startup_restore_fallback")
			s.refreshChangeFeed(ctx)
			return
		}

		persistedEntryCount = int64(restoredEntries)
		util.GetLogger().Info(ctx, fmt.Sprintf("filesearch startup restore loaded persisted entries: entries=%d", restoredEntries))
	} else {
		snapshot, err := s.db.SearchIndexSnapshot(ctx)
		if err != nil {
			util.GetLogger().Warn(ctx, "filesearch startup restore failed to load persisted sqlite snapshot: "+err.Error())
			s.scanAllRootsWithReason(ctx, "startup_restore_fallback")
			s.refreshChangeFeed(ctx)
			return
		}
		persistedEntryCount = snapshot.EntryCount
		util.GetLogger().Info(ctx, fmt.Sprintf(
			"filesearch startup restore loaded persisted sqlite search state: entries=%d bigram_rows=%d",
			snapshot.EntryCount,
			snapshot.BigramRowCount,
		))
	}

	roots, err := s.db.ListRoots(ctx)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch startup restore failed to load roots: "+err.Error())
		s.refreshChangeFeed(ctx)
		return
	}

	s.refreshChangeFeed(ctx)
	if startupNeedsInitialFullScan(roots, persistedEntryCount) {
		// Startup restore used to treat an empty persisted index as good enough and
		// enqueue root-dirty incremental reconcile for every never-scanned root.
		// That sent the very first index build down the incremental path, which
		// bypassed full-run bulk sync and subtree grouping. When persisted search
		// state is empty and at least one root has never completed a full scan, we
		// force the real full-run path so initial indexing uses the intended heavy-
		// scan execution strategy instead of replaying root-level dirty batches.
		util.GetLogger().Info(ctx, fmt.Sprintf(
			"filesearch startup restore escalating to full scan: roots=%d persisted_entries=%d",
			len(roots),
			persistedEntryCount,
		))
		s.scanAllRootsWithReason(ctx, "startup_restore_initial_full")
		return
	}

	reconcileRoots := startupReconcileRoots(roots, time.Now())
	if len(reconcileRoots) == 0 {
		util.GetLogger().Info(ctx, "filesearch startup restore completed without reconcile")
		if s.localProvider != nil {
			logLocalIndexSnapshot(ctx, "startup_restore_complete", s.localProvider.snapshot(), true)
		} else if snapshot, err := s.db.SearchIndexSnapshot(ctx); err == nil {
			logSQLiteIndexSnapshot(ctx, "startup_restore_complete", snapshot, true)
		}
		return
	}

	for _, root := range reconcileRoots {
		s.enqueueDirtyWithContext(ctx, DirtySignal{
			Kind:          DirtySignalKindRoot,
			RootID:        root.ID,
			Path:          root.Path,
			PathIsDir:     true,
			PathTypeKnown: true,
			At:            time.Now(),
		})
	}

	util.GetLogger().Info(ctx, fmt.Sprintf("filesearch startup restore queued selective reconcile: roots=%d", len(reconcileRoots)))
	if err := s.processDirtyQueue(ctx, time.Now().Add(2*s.dirtyDebounceWindow())); err != nil {
		util.GetLogger().Warn(ctx, "filesearch startup restore failed to process selective reconcile: "+err.Error())
	}
	if s.localProvider != nil {
		logLocalIndexSnapshot(ctx, "startup_restore_complete", s.localProvider.snapshot(), true)
	} else if snapshot, err := s.db.SearchIndexSnapshot(ctx); err == nil {
		logSQLiteIndexSnapshot(ctx, "startup_restore_complete", snapshot, true)
	}
}

func startupNeedsInitialFullScan(roots []RootRecord, persistedEntryCount int64) bool {
	if persistedEntryCount > 0 {
		return false
	}

	for _, root := range roots {
		if root.LastFullScanAt <= 0 {
			return true
		}
	}

	return false
}

func startupReconcileRoots(roots []RootRecord, now time.Time) []RootRecord {
	selected := make([]RootRecord, 0, len(roots))
	for _, root := range roots {
		if !rootNeedsStartupReconcile(root, now) {
			continue
		}
		selected = append(selected, root)
	}
	return selected
}

func rootNeedsStartupReconcile(root RootRecord, now time.Time) bool {
	if root.LastFullScanAt <= 0 {
		return true
	}

	if root.FeedState == RootFeedStateDegraded || root.FeedState == RootFeedStateUnavailable {
		return true
	}

	switch root.FeedType {
	case RootFeedTypeFSEvents:
		cursor, ok := decodeFeedCursor(root.FeedCursor, RootFeedTypeFSEvents)
		return !ok || !feedCursorFresh(cursor, now, defaultFeedCursorSafeWindow)
	case RootFeedTypeUSN:
		return usnRootNeedsStartupReconcile(root, now)
	case RootFeedTypeFallback, "":
		return true
	default:
		return true
	}
}
