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
	policy              *policyState
	directFileBatchSize int
}

func NewSnapshotBuilder(policy *policyState) *SnapshotBuilder {
	if policy == nil {
		policy = newPolicyState(Policy{})
	}
	return &SnapshotBuilder{
		policy:              policy,
		directFileBatchSize: defaultSplitBudget().DirectFileBatchSize,
	}
}

func (b *SnapshotBuilder) SetDirectFileBatchSize(size int) {
	if b == nil || size <= 0 {
		return
	}
	b.directFileBatchSize = size
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

// BuildDirectFilesJobSnapshot materializes the full direct-files scope owned by
// one sealed job. The planner no longer splits one directory into many direct-
// files jobs because that older shape made stale-file pruning ambiguous. This
// helper remains for tests and small direct-files paths; runtime execution now
// prefers StreamDirectFilesJobBatches to keep SQLite staging bounded in memory.
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
		isDir, childInfo, infoErr := strictDirEntryType(scopePath, dirEntry)
		if infoErr != nil {
			return batch, infoErr
		}
		if shouldSkipSystemPath(childPath, isDir) {
			continue
		}
		if !b.shouldIndexPath(root, childPath, isDir) {
			continue
		}
		if isDir {
			continue
		}
		if childInfo == nil {
			// The earlier eager Info() call paid metadata I/O even for entries that
			// policy or system-path filtering would skip. We only load FileInfo once
			// the file is confirmed indexable because newEntryRecord needs the full
			// stat payload for persisted metadata.
			childInfo, infoErr = strictDirEntryInfo(scopePath, dirEntry)
			if infoErr != nil {
				return batch, infoErr
			}
		}
		directFiles = append(directFiles, newEntryRecord(root, childPath, childInfo))
	}

	sort.Slice(directFiles, func(left int, right int) bool {
		return directFiles[left].Path < directFiles[right].Path
	})

	scanTimestamp := time.Now().UnixMilli()
	batch.Directories = append(batch.Directories, DirectoryRecord{
		Path:         scopePath,
		RootID:       root.ID,
		ParentPath:   filepath.Dir(scopePath),
		LastScanTime: scanTimestamp,
		Exists:       true,
	})
	batch.Entries = append(batch.Entries, newEntryRecord(root, scopePath, info))
	batch.Entries = append(batch.Entries, directFiles...)
	return batch, nil
}

// StreamDirectFilesJobBatches emits one directory-owned direct-files job as
// bounded staging batches. The older planner solved memory pressure by turning
// one directory into many jobs, but that split stale-file ownership and made
// direct-file pruning ambiguous. Keeping one job per directory and streaming
// its files in small batches keeps ownership correct without rebuilding the
// original whole-directory memory spike.
func (b *SnapshotBuilder) StreamDirectFilesJobBatches(ctx context.Context, root RootRecord, job Job, onBatch func(SubtreeSnapshotBatch) error) error {
	if onBatch == nil {
		return fmt.Errorf("direct-files batch callback is required")
	}

	scopePath := filepath.Clean(job.ScopePath)
	info, err := b.validateScopePath(root, scopePath)
	if err != nil || info == nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("direct-files scope %q is not a directory", scopePath)
	}

	dirEntries, err := os.ReadDir(scopePath)
	if err != nil {
		return fmt.Errorf("failed to read direct-files scope %s: %w", scopePath, err)
	}

	scanTimestamp := time.Now().UnixMilli()
	newBatch := func(includeScope bool) SubtreeSnapshotBatch {
		batch := SubtreeSnapshotBatch{
			RootID:    root.ID,
			ScopePath: scopePath,
		}
		if !includeScope {
			return batch
		}
		batch.Directories = append(batch.Directories, DirectoryRecord{
			Path:         scopePath,
			RootID:       root.ID,
			ParentPath:   filepath.Dir(scopePath),
			LastScanTime: scanTimestamp,
			Exists:       true,
		})
		batch.Entries = append(batch.Entries, newEntryRecord(root, scopePath, info))
		return batch
	}
	flushBatch := func(batch *SubtreeSnapshotBatch) error {
		if batch == nil {
			return nil
		}
		if len(batch.Directories) == 0 && len(batch.Entries) == 0 {
			return nil
		}
		return onBatch(*batch)
	}

	batch := newBatch(true)
	filesInBatch := 0
	maxFilesPerBatch := b.directFileBatchSize
	if maxFilesPerBatch <= 0 {
		maxFilesPerBatch = defaultSplitBudget().DirectFileBatchSize
	}
	if maxFilesPerBatch <= 0 {
		maxFilesPerBatch = 1024
	}

	for _, dirEntry := range dirEntries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		childPath := filepath.Join(scopePath, dirEntry.Name())
		isDir, childInfo, infoErr := strictDirEntryType(scopePath, dirEntry)
		if infoErr != nil {
			return infoErr
		}
		if shouldSkipSystemPath(childPath, isDir) {
			continue
		}
		if !b.shouldIndexPath(root, childPath, isDir) {
			continue
		}
		if isDir {
			continue
		}
		if childInfo == nil {
			// Streaming direct-files batches now delays Info() until a child file
			// survives skip/policy checks, because the old eager stat path repeated
			// metadata work for entries that never reached SQLite staging.
			childInfo, infoErr = strictDirEntryInfo(scopePath, dirEntry)
			if infoErr != nil {
				return infoErr
			}
		}

		batch.Entries = append(batch.Entries, newEntryRecord(root, childPath, childInfo))
		filesInBatch++
		if filesInBatch < maxFilesPerBatch {
			continue
		}
		if err := flushBatch(&batch); err != nil {
			return err
		}
		batch = newBatch(false)
		filesInBatch = 0
	}

	return flushBatch(&batch)
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
		path string
		info os.FileInfo
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
			isDir, info, infoErr := strictDirEntryType(current.path, dirEntry)
			if infoErr != nil {
				continue
			}

			if shouldSkipSystemPath(childPath, isDir) {
				continue
			}
			if !b.shouldIndexPath(root, childPath, isDir) {
				continue
			}
			if info == nil {
				// Recursive subtree snapshots must still persist real mtime/size data,
				// but loading FileInfo after skip/policy checks avoids extra stat calls
				// for entries that would never be indexed.
				info, infoErr = strictDirEntryInfo(current.path, dirEntry)
				if infoErr != nil {
					continue
				}
			}

			if !isDir {
				batch.Entries = append(batch.Entries, newEntryRecord(root, childPath, info))
				continue
			}

			// Snapshot execution never consults gitignore patterns here. The previous
			// traversal still loaded every directory's .gitignore and copied pattern
			// slices forward, which added filesystem reads and allocations to the
			// dominant build_snapshot phase without changing which entries were kept.
			queue = append(queue, queueItem{
				path: childPath,
				info: info,
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
