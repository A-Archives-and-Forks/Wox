package filesearch

import (
	"context"
	"fmt"
	"time"

	"wox/util"
)

func (s *Scanner) startupRestore(ctx context.Context) {
	if s.localProvider != nil {
		restoredEntries, err := s.reloadLocalProviderFromDB(ctx)
		if err != nil {
			util.GetLogger().Warn(ctx, "filesearch startup restore failed to load persisted entries: "+err.Error())
			s.scanAllRootsWithReason(ctx, "startup_restore_fallback")
			s.refreshChangeFeed(ctx)
			return
		}

		util.GetLogger().Info(ctx, fmt.Sprintf("filesearch startup restore loaded persisted entries: entries=%d", restoredEntries))
	} else {
		snapshot, err := s.db.SearchIndexSnapshot(ctx)
		if err != nil {
			util.GetLogger().Warn(ctx, "filesearch startup restore failed to load persisted sqlite snapshot: "+err.Error())
			s.scanAllRootsWithReason(ctx, "startup_restore_fallback")
			s.refreshChangeFeed(ctx)
			return
		}
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
