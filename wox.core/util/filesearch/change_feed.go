package filesearch

import (
	"context"
	"time"
)

type ChangeSignalKind string

const (
	ChangeSignalKindDirtyRoot             ChangeSignalKind = "dirty_root"
	ChangeSignalKindDirtyPath             ChangeSignalKind = "dirty_path"
	ChangeSignalKindRequiresRootReconcile ChangeSignalKind = "requires_root_reconcile"
	ChangeSignalKindFeedUnavailable       ChangeSignalKind = "feed_unavailable"
)

type ChangeSignal struct {
	Kind          ChangeSignalKind
	RootID        string
	FeedType      RootFeedType
	Path          string
	PathIsDir     bool
	PathTypeKnown bool
	Reason        string
	Cursor        string
	At            time.Time
}

type ChangeFeed interface {
	Mode() string
	Signals() <-chan ChangeSignal
	Refresh(ctx context.Context, roots []RootRecord) error
	Close() error
}

type RootFeedSnapshot struct {
	FeedType   RootFeedType
	FeedCursor string
	FeedState  RootFeedState
}

type RootFeedSnapshotter interface {
	SnapshotRootFeed(ctx context.Context, root RootRecord) (RootFeedSnapshot, error)
}
