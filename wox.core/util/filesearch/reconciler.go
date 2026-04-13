package filesearch

import (
	"context"
	"fmt"
	"wox/util"
)

type ReconcileResult struct {
	RootID       string
	Mode         ReconcileMode
	ReloadNeeded bool
}

type Reconciler struct {
	db       *FileSearchDB
	snapshot *SnapshotBuilder
}

func NewReconciler(db *FileSearchDB) *Reconciler {
	return newReconciler(db, NewSnapshotBuilder())
}

func newReconciler(db *FileSearchDB, snapshot *SnapshotBuilder) *Reconciler {
	if snapshot == nil {
		snapshot = NewSnapshotBuilder()
	}

	return &Reconciler{
		db:       db,
		snapshot: snapshot,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, batch ReconcileBatch) (ReconcileResult, error) {
	result := ReconcileResult{
		RootID: batch.RootID,
		Mode:   batch.Mode,
	}

	root, err := r.db.FindRootByID(ctx, batch.RootID)
	if err != nil {
		return result, err
	}
	if root == nil {
		return result, fmt.Errorf("root %q not found", batch.RootID)
	}

	switch batch.Mode {
	case ReconcileModeRoot:
		snapshot, err := r.snapshot.BuildSubtreeSnapshot(ctx, *root, root.Path)
		if err != nil {
			return result, err
		}
		if err := r.db.ReplaceRootSnapshot(ctx, *root, snapshot.Directories, snapshot.Entries, nil); err != nil {
			return result, err
		}
		now := util.GetSystemTimestamp()
		root.LastReconcileAt = now
		root.FeedState = nextFeedStateAfterSuccessfulReconcile(*root)
		root.UpdatedAt = now
		if err := r.db.UpdateRootState(ctx, *root); err != nil {
			return result, err
		}
		result.ReloadNeeded = true
		return result, nil
	case ReconcileModeSubtree:
		snapshots := make([]SubtreeSnapshotBatch, 0, len(batch.Paths))
		for _, scopePath := range batch.Paths {
			snapshot, err := r.snapshot.BuildSubtreeSnapshot(ctx, *root, scopePath)
			if err != nil {
				return result, err
			}
			snapshots = append(snapshots, snapshot)
		}
		if err := r.db.ReplaceSubtreeSnapshots(ctx, snapshots); err != nil {
			return result, err
		}
		if len(batch.Paths) > 0 {
			now := util.GetSystemTimestamp()
			root.LastReconcileAt = now
			root.FeedState = nextFeedStateAfterSuccessfulReconcile(*root)
			root.UpdatedAt = now
			if err := r.db.UpdateRootState(ctx, *root); err != nil {
				return result, err
			}
		}
		result.ReloadNeeded = len(batch.Paths) > 0
		return result, nil
	default:
		return result, fmt.Errorf("unsupported reconcile mode %q", batch.Mode)
	}
}

func nextFeedStateAfterSuccessfulReconcile(root RootRecord) RootFeedState {
	if root.FeedState == RootFeedStateUnavailable {
		return RootFeedStateUnavailable
	}
	return RootFeedStateReady
}
