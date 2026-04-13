package filesearch

import (
	"path/filepath"
	"strings"
	"time"
)

type usnJournalState struct {
	Volume    string
	JournalID uint64
	FirstUSN  int64
	NextUSN   int64
}

type preparedUSNVolumeRefresh struct {
	roots    []RootRecord
	startUSN int64
	signals  []ChangeSignal
}

func prepareUSNVolumeRefresh(roots []RootRecord, journal usnJournalState, now time.Time, safeWindow time.Duration) preparedUSNVolumeRefresh {
	prepared := preparedUSNVolumeRefresh{
		roots:    append([]RootRecord(nil), roots...),
		startUSN: journal.NextUSN,
	}

	haveFreshCursor := false
	for _, root := range roots {
		if root.FeedState == RootFeedStateUnavailable {
			prepared.signals = append(prepared.signals, newUSNRecoverySignal(root, "usn feed recovered", now))
			continue
		}

		cursor, ok := decodeFeedCursor(root.FeedCursor, RootFeedTypeUSN)
		if !ok {
			if root.FeedCursor != "" {
				prepared.signals = append(prepared.signals, newUSNRecoverySignal(root, "invalid usn cursor", now))
			}
			continue
		}
		if !feedCursorFresh(cursor, now, safeWindow) {
			prepared.signals = append(prepared.signals, newUSNRecoverySignal(root, "expired usn cursor", now))
			continue
		}
		if cursor.JournalID != journal.JournalID {
			prepared.signals = append(prepared.signals, newUSNRecoverySignal(root, "stale usn journal id", now))
			continue
		}
		if cursor.Volume != "" && !strings.EqualFold(cursor.Volume, journal.Volume) {
			prepared.signals = append(prepared.signals, newUSNRecoverySignal(root, "usn cursor volume changed", now))
			continue
		}
		if cursor.USN < journal.FirstUSN || cursor.USN > journal.NextUSN {
			prepared.signals = append(prepared.signals, newUSNRecoverySignal(root, "usn cursor outside journal retention window", now))
			continue
		}

		if !haveFreshCursor || cursor.USN < prepared.startUSN {
			prepared.startUSN = cursor.USN
			haveFreshCursor = true
		}
	}

	if !haveFreshCursor && prepared.startUSN < 0 {
		prepared.startUSN = 0
	}

	return prepared
}

func translateUSNDelta(root RootRecord, journal usnJournalState, path string, pathIsDir bool, pathTypeKnown bool, usn int64, at time.Time) ChangeSignal {
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == "" {
		cleanPath = filepath.Clean(root.Path)
	}

	cursorText := ""
	if usn > 0 {
		cursor, err := encodeFeedCursor(FeedCursor{
			FeedType:  RootFeedTypeUSN,
			UpdatedAt: at.UnixMilli(),
			JournalID: journal.JournalID,
			USN:       usn,
			Volume:    journal.Volume,
		})
		if err == nil {
			cursorText = cursor
		}
	}

	kind := ChangeSignalKindDirtyPath
	if cleanPath == filepath.Clean(root.Path) {
		kind = ChangeSignalKindDirtyRoot
		pathIsDir = true
		pathTypeKnown = true
	}

	return ChangeSignal{
		Kind:          kind,
		RootID:        root.ID,
		FeedType:      RootFeedTypeUSN,
		Path:          cleanPath,
		PathIsDir:     pathIsDir,
		PathTypeKnown: pathTypeKnown,
		Cursor:        cursorText,
		At:            at,
	}
}

func newUSNRecoverySignal(root RootRecord, reason string, at time.Time) ChangeSignal {
	return ChangeSignal{
		Kind:          ChangeSignalKindRequiresRootReconcile,
		RootID:        root.ID,
		FeedType:      RootFeedTypeUSN,
		Path:          root.Path,
		PathIsDir:     true,
		PathTypeKnown: true,
		Reason:        reason,
		At:            at,
	}
}
