package filesearch

import "context"

type testSnapshotChangeFeed struct {
	signals  chan ChangeSignal
	snapshot func(root RootRecord) (RootFeedSnapshot, error)
}

func newTestSnapshotChangeFeed(snapshot func(root RootRecord) (RootFeedSnapshot, error)) *testSnapshotChangeFeed {
	return &testSnapshotChangeFeed{
		signals:  make(chan ChangeSignal),
		snapshot: snapshot,
	}
}

func (t *testSnapshotChangeFeed) Mode() string {
	return "test"
}

func (t *testSnapshotChangeFeed) Signals() <-chan ChangeSignal {
	return t.signals
}

func (t *testSnapshotChangeFeed) Refresh(ctx context.Context, roots []RootRecord) error {
	_ = ctx
	_ = roots
	return nil
}

func (t *testSnapshotChangeFeed) Close() error {
	return nil
}

func (t *testSnapshotChangeFeed) SnapshotRootFeed(ctx context.Context, root RootRecord) (RootFeedSnapshot, error) {
	_ = ctx
	if t.snapshot == nil {
		return RootFeedSnapshot{}, nil
	}
	return t.snapshot(root)
}
