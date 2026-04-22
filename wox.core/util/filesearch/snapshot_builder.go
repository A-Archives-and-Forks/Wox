package filesearch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	snapshot, err := b.BuildSubtreeJobSnapshot(ctx, root, Job{
		RootID:    root.ID,
		RootPath:  root.Path,
		ScopePath: root.Path,
		Kind:      JobKindSubtree,
	})
	if err != nil {
		return nil, err
	}

	return snapshot.Entries, nil
}

// BuildDirectFilesJobSnapshot materializes only the direct-file slice owned by
// one sealed job. The previous whole-root snapshot flow kept every sibling
// entry alive until writeback, which is why large roots caused indexing-time
// memory spikes even after the planner had already split the work into chunks.
func (b *SnapshotBuilder) BuildDirectFilesJobSnapshot(ctx context.Context, root RootRecord, job Job) (SubtreeSnapshotBatch, error) {
	scopePath := filepath.Clean(job.ScopePath)
	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: scopePath,
	}

	info, err := b.validateScopePath(root, scopePath)
	if err != nil || info == nil {
		return batch, err
	}
	if !info.IsDir() {
		return batch, fmt.Errorf("direct-files scope %q is not a directory", scopePath)
	}

	dirEntries, err := os.ReadDir(scopePath)
	if err != nil {
		return batch, fmt.Errorf("failed to read direct-files scope %s: %w", scopePath, err)
	}

	directFiles := make([]EntryRecord, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		select {
		case <-ctx.Done():
			return batch, ctx.Err()
		default:
		}

		childPath := filepath.Join(scopePath, dirEntry.Name())
		childInfo, infoErr := strictDirEntryInfo(scopePath, dirEntry)
		if infoErr != nil {
			return batch, infoErr
		}
		if childInfo.IsDir() {
			continue
		}
		if shouldSkipSystemPath(childPath, false) {
			continue
		}
		if !b.shouldIndexPath(root, childPath, false) {
			continue
		}
		directFiles = append(directFiles, newEntryRecord(root, childPath, childInfo))
	}

	sort.Slice(directFiles, func(left int, right int) bool {
		return directFiles[left].Path < directFiles[right].Path
	})

	start := 0
	end := len(directFiles)
	if job.DirectFileChunkCount > 0 {
		start = job.DirectFileChunkOffset
		end = start + job.DirectFileChunkCount
		if start < 0 || start > len(directFiles) {
			return batch, fmt.Errorf("direct-files chunk offset %d is outside %q", job.DirectFileChunkOffset, scopePath)
		}
		if end > len(directFiles) {
			return batch, fmt.Errorf("direct-files chunk end %d exceeds %d files in %q", end, len(directFiles), scopePath)
		}
	}

	// Chunk zero still owns the directory record because the planner counted the
	// scope directory itself once. Later chunks intentionally skip that record so
	// execution can honor the sealed chunk budget without rebuilding a full
	// directory snapshot for every direct-file chunk.
	if job.DirectFileChunkCount == 0 || job.DirectFileChunkIndex == 0 {
		scanTimestamp := time.Now().UnixMilli()
		batch.Directories = append(batch.Directories, DirectoryRecord{
			Path:         scopePath,
			RootID:       root.ID,
			ParentPath:   filepath.Dir(scopePath),
			LastScanTime: scanTimestamp,
			Exists:       true,
		})
		batch.Entries = append(batch.Entries, newEntryRecord(root, scopePath, info))
	}

	batch.Entries = append(batch.Entries, directFiles[start:end]...)
	return batch, nil
}

// BuildSubtreeJobSnapshot materializes only the subtree owned by one planned
// job. The previous whole-root accumulation forced execution to hold an entire
// root snapshot in memory even when the planner had already split that root
// into bounded jobs, which is what drove the earlier indexing-time memory spike.
func (b *SnapshotBuilder) BuildSubtreeJobSnapshot(ctx context.Context, root RootRecord, job Job) (SubtreeSnapshotBatch, error) {
	return b.BuildSubtreeSnapshot(ctx, root, job.ScopePath)
}

func (b *SnapshotBuilder) BuildSubtreeSnapshot(ctx context.Context, root RootRecord, scopePath string) (SubtreeSnapshotBatch, error) {
	scopePath = filepath.Clean(scopePath)
	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: scopePath,
	}

	info, err := b.validateScopePath(root, scopePath)
	if err != nil {
		return batch, err
	}
	if info == nil {
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

func (b *SnapshotBuilder) validateScopePath(root RootRecord, scopePath string) (os.FileInfo, error) {
	if root.ID == "" {
		return nil, fmt.Errorf("root id is required")
	}
	if root.Path == "" {
		return nil, fmt.Errorf("root path is required")
	}
	if scopePath == "" || !filepath.IsAbs(scopePath) {
		return nil, fmt.Errorf("scope path %q is invalid", scopePath)
	}
	if !pathWithinScope(root.Path, scopePath) {
		return nil, fmt.Errorf("scope path %q is outside root path %q", scopePath, root.Path)
	}

	info, err := os.Stat(scopePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	if scopePath != filepath.Clean(root.Path) && !b.shouldIndexPath(root, scopePath, info.IsDir()) {
		return nil, nil
	}

	return info, nil
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
