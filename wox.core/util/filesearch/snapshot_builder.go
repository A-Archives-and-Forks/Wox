package filesearch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"wox/util"
)

type SnapshotBuilder struct {
	policy *policyState
}

func NewSnapshotBuilder(policy *policyState) *SnapshotBuilder {
	if policy == nil {
		policy = newPolicyState(Policy{})
	}
	return &SnapshotBuilder{policy: policy}
}

func (b *SnapshotBuilder) BuildRootEntries(ctx context.Context, root RootRecord) ([]EntryRecord, error) {
	snapshot, err := b.BuildSubtreeSnapshot(ctx, root, root.Path)
	if err != nil {
		return nil, err
	}

	return snapshot.Entries, nil
}

func (b *SnapshotBuilder) BuildSubtreeSnapshot(ctx context.Context, root RootRecord, scopePath string) (SubtreeSnapshotBatch, error) {
	scopePath = filepath.Clean(scopePath)
	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: scopePath,
	}

	if root.ID == "" {
		return batch, fmt.Errorf("root id is required")
	}
	if root.Path == "" {
		return batch, fmt.Errorf("root path is required")
	}
	if scopePath == "" || !filepath.IsAbs(scopePath) {
		return batch, fmt.Errorf("scope path %q is invalid", scopePath)
	}
	if !pathWithinScope(root.Path, scopePath) {
		return batch, fmt.Errorf("scope path %q is outside root path %q", scopePath, root.Path)
	}

	info, err := os.Stat(scopePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return batch, nil
		}
		return batch, err
	}

	if scopePath != filepath.Clean(root.Path) && !b.shouldIndexPath(root, scopePath, info.IsDir()) {
		return batch, nil
	}

	if !info.IsDir() {
		batch.Entries = append(batch.Entries, newEntryRecord(root, scopePath, info))
		return batch, nil
	}

	type queueItem struct {
		path     string
		patterns []gitIgnorePattern
		info     os.FileInfo
	}

	queue := []queueItem{{
		path: scopePath,
		info: info,
	}}
	scanTimestamp := time.Now().UnixMilli()

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return batch, ctx.Err()
		default:
		}

		current := queue[0]
		queue = queue[1:]

		batch.Directories = append(batch.Directories, DirectoryRecord{
			Path:         current.path,
			RootID:       root.ID,
			ParentPath:   filepath.Dir(current.path),
			LastScanTime: scanTimestamp,
			Exists:       true,
		})
		batch.Entries = append(batch.Entries, newEntryRecord(root, current.path, current.info))

		localPatterns := append([]gitIgnorePattern{}, current.patterns...)
		localPatterns = append(localPatterns, loadGitIgnorePatterns(current.path)...)

		dirEntries, readErr := os.ReadDir(current.path)
		if readErr != nil {
			if current.path == scopePath {
				return batch, fmt.Errorf("failed to read scope directory %s: %w", current.path, readErr)
			}
			util.GetLogger().Warn(ctx, "filesearch skipped unreadable directory "+current.path+": "+readErr.Error())
			continue
		}

		for _, dirEntry := range dirEntries {
			childPath := filepath.Join(current.path, dirEntry.Name())
			info, infoErr := dirEntry.Info()
			if infoErr != nil {
				continue
			}

			isDir := info.IsDir()
			if shouldSkipSystemPath(childPath, isDir) {
				continue
			}
			if !b.shouldIndexPath(root, childPath, isDir) {
				continue
			}

			if !isDir {
				batch.Entries = append(batch.Entries, newEntryRecord(root, childPath, info))
				continue
			}

			queue = append(queue, queueItem{
				path:     childPath,
				patterns: localPatterns,
				info:     info,
			})
		}
	}

	return batch, nil
}

func (b *SnapshotBuilder) shouldIndexPath(root RootRecord, path string, isDir bool) bool {
	if shouldSkipSystemPath(path, isDir) {
		return false
	}
	if b == nil || b.policy == nil {
		return true
	}
	return b.policy.shouldIndexPath(root, path, isDir)
}
