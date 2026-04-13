//go:build darwin

package filesearch

import (
	"path/filepath"
	"time"
)

const (
	fseventSinceNow            uint64 = ^uint64(0)
	fseventFlagMustScanSubDirs        = 0x00000001
	fseventFlagUserDropped            = 0x00000002
	fseventFlagKernelDropped          = 0x00000004
	fseventFlagEventIDsWrapped        = 0x00000008
	fseventFlagRootChanged            = 0x00000020
	fseventFlagMount                  = 0x00000040
	fseventFlagUnmount                = 0x00000080
	fseventFlagItemIsFile             = 0x00010000
	fseventFlagItemIsDir              = 0x00020000
)

type preparedFSEventsRefresh struct {
	watchRoots   []RootRecord
	sinceEventID uint64
	signals      []ChangeSignal
}

func prepareFSEventsRefresh(roots []RootRecord, now time.Time, safeWindow time.Duration) preparedFSEventsRefresh {
	prepared := preparedFSEventsRefresh{
		watchRoots:   append([]RootRecord(nil), roots...),
		sinceEventID: fseventSinceNow,
	}

	haveFreshCursor := false
	for _, root := range roots {
		if root.FeedState == RootFeedStateUnavailable {
			prepared.signals = append(prepared.signals, newFSEventsRecoverySignal(root, "fsevents feed recovered", now))
		}

		cursor, ok := decodeFeedCursor(root.FeedCursor, RootFeedTypeFSEvents)
		if !ok {
			if root.FeedCursor != "" {
				prepared.signals = append(prepared.signals, newFSEventsRecoverySignal(root, "invalid fsevents cursor", now))
			}
			continue
		}
		if !feedCursorFresh(cursor, now, safeWindow) {
			prepared.signals = append(prepared.signals, newFSEventsRecoverySignal(root, "expired fsevents cursor", now))
			continue
		}

		if !haveFreshCursor || cursor.FSEventID < prepared.sinceEventID {
			prepared.sinceEventID = cursor.FSEventID
			haveFreshCursor = true
		}
	}

	if !haveFreshCursor {
		prepared.sinceEventID = fseventSinceNow
	}

	return prepared
}

func translateFSEvent(root RootRecord, eventPath string, flags uint64, eventID uint64, at time.Time) []ChangeSignal {
	eventPath = filepath.Clean(eventPath)
	if eventPath == "" {
		eventPath = root.Path
	}

	cursorText := ""
	if eventID > 0 {
		cursor, err := encodeFeedCursor(FeedCursor{
			FeedType:  RootFeedTypeFSEvents,
			UpdatedAt: at.UnixMilli(),
			FSEventID: eventID,
		})
		if err == nil {
			cursorText = cursor
		}
	}

	if fseventRequiresRootReconcile(flags) {
		return []ChangeSignal{{
			Kind:          ChangeSignalKindRequiresRootReconcile,
			RootID:        root.ID,
			FeedType:      RootFeedTypeFSEvents,
			Path:          root.Path,
			PathIsDir:     true,
			PathTypeKnown: true,
			Reason:        "fsevents flagged history loss or root change",
			Cursor:        cursorText,
			At:            at,
		}}
	}

	pathIsDir := flags&fseventFlagItemIsDir != 0
	pathTypeKnown := pathIsDir || flags&fseventFlagItemIsFile != 0
	kind := ChangeSignalKindDirtyPath
	if eventPath == filepath.Clean(root.Path) {
		kind = ChangeSignalKindDirtyRoot
		pathIsDir = true
		pathTypeKnown = true
	}

	return []ChangeSignal{{
		Kind:          kind,
		RootID:        root.ID,
		FeedType:      RootFeedTypeFSEvents,
		Path:          eventPath,
		PathIsDir:     pathIsDir,
		PathTypeKnown: pathTypeKnown,
		Cursor:        cursorText,
		At:            at,
	}}
}

func fseventRequiresRootReconcile(flags uint64) bool {
	return flags&(fseventFlagMustScanSubDirs|fseventFlagUserDropped|fseventFlagKernelDropped|fseventFlagEventIDsWrapped|fseventFlagRootChanged|fseventFlagMount|fseventFlagUnmount) != 0
}

func newFSEventsRecoverySignal(root RootRecord, reason string, at time.Time) ChangeSignal {
	return ChangeSignal{
		Kind:          ChangeSignalKindRequiresRootReconcile,
		RootID:        root.ID,
		FeedType:      RootFeedTypeFSEvents,
		Path:          root.Path,
		PathIsDir:     true,
		PathTypeKnown: true,
		Reason:        reason,
		At:            at,
	}
}
