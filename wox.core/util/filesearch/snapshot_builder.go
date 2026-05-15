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
	rootExclusions      map[string][]string
}

// Diagnostic addition: the real-index artifact needs the dominant streaming
// traversal split into filesystem read, entry type, policy, and metadata costs.
// Keeping this accumulator local to one subtree stream avoids cross-job locking
// while preserving the existing single-goroutine traversal semantics.
type subtreeStreamDiagnostics struct {
	readDirNanos      int64
	readDirCount      int
	dirEntryTypeNanos int64
	dirEntryTypeCount int
	policyCheckNanos  int64
	policyCheckCount  int
	dirEntryInfoNanos int64
	dirEntryInfoCount int
}

func (d *subtreeStreamDiagnostics) recordReadDir(elapsed time.Duration) {
	d.readDirNanos += elapsed.Nanoseconds()
	d.readDirCount++
}

func (d *subtreeStreamDiagnostics) recordDirEntryType(elapsed time.Duration) {
	d.dirEntryTypeNanos += elapsed.Nanoseconds()
	d.dirEntryTypeCount++
}

func (d *subtreeStreamDiagnostics) recordPolicyCheck(elapsed time.Duration) {
	d.policyCheckNanos += elapsed.Nanoseconds()
	d.policyCheckCount++
}

func (d *subtreeStreamDiagnostics) recordDirEntryInfo(elapsed time.Duration) {
	d.dirEntryInfoNanos += elapsed.Nanoseconds()
	d.dirEntryInfoCount++
}

func (d *subtreeStreamDiagnostics) log(ctx context.Context, scope string) {
	if d == nil {
		return
	}
	// Bug fix: this must read the accumulator at function exit. A value receiver
	// on a deferred method copied the zero-value counters at defer time, which
	// made the real-index artifact show scan timings with work_count=0.
	logFilesearchScanDiagnostic(ctx, "subtree_stream_readdir", scope, scanDiagnosticMillis(d.readDirNanos), d.readDirCount)
	logFilesearchScanDiagnostic(ctx, "subtree_stream_direntry_type", scope, scanDiagnosticMillis(d.dirEntryTypeNanos), d.dirEntryTypeCount)
	logFilesearchScanDiagnostic(ctx, "subtree_stream_policy_check", scope, scanDiagnosticMillis(d.policyCheckNanos), d.policyCheckCount)
	logFilesearchScanDiagnostic(ctx, "subtree_stream_direntry_info", scope, scanDiagnosticMillis(d.dirEntryInfoNanos), d.dirEntryInfoCount)
}

func scanDiagnosticMillis(nanos int64) int64 {
	if nanos <= 0 {
		return 0
	}
	return (nanos + int64(time.Millisecond) - 1) / int64(time.Millisecond)
}

func NewSnapshotBuilder(policy *policyState) *SnapshotBuilder {
	if policy == nil {
		policy = newPolicyState(Policy{})
	}
	return &SnapshotBuilder{
		policy:              policy,
		directFileBatchSize: defaultSplitBudget().DirectFileBatchSize,
		rootExclusions:      map[string][]string{},
	}
}

func (b *SnapshotBuilder) SetDirectFileBatchSize(size int) {
	if b == nil || size <= 0 {
		return
	}
	b.directFileBatchSize = size
}

func (b *SnapshotBuilder) SetRootExclusions(exclusions map[string][]string) {
	if b == nil {
		return
	}
	b.rootExclusions = copyRootExclusions(exclusions)
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
		if errors.Is(err, os.ErrNotExist) {
			// Dirty scopes may disappear after validation in temp/build folders.
			// Returning the empty owned scope lets the DB prune stale rows without
			// promoting a vanished child path into a root-wide retry.
			return batch, nil
		}
		return batch, fmt.Errorf("failed to read direct-files scope %s: %w", scopePath, err)
	}

	policyContext := b.policy.newTraversalContext(root, scopePath)
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
		if isDir && b.isExcludedPath(root.ID, childPath) {
			// Dynamic child roots own their directory entry as well as descendants.
			// Direct-files scopes therefore skip the directory itself, not only the
			// recursive scan that a subtree builder would otherwise queue later.
			continue
		}
		if shouldSkipSystemPath(childPath, isDir) {
			continue
		}
		if !policyContext.ShouldIndexPath(childPath, isDir) {
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
		if errors.Is(err, os.ErrNotExist) {
			// Streaming direct-files jobs still own the directory scope. If the
			// directory vanishes after validation, an empty stream is enough for the
			// caller to prune staged direct-file rows at the same narrow boundary.
			return nil
		}
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

	policyContext := b.policy.newTraversalContext(root, scopePath)
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
		if isDir && b.isExcludedPath(root.ID, childPath) {
			// Streaming direct-files batches share the same ownership contract as
			// materialized snapshots: a parent root must not stage the dynamic
			// child's directory row because that would steal the path-owned entry.
			continue
		}
		if shouldSkipSystemPath(childPath, isDir) {
			continue
		}
		if !policyContext.ShouldIndexPath(childPath, isDir) {
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
		path   string
		info   os.FileInfo
		policy TraversalPolicyContext
	}

	queue := []queueItem{{
		path:   scopePath,
		info:   info,
		policy: b.policy.newTraversalContext(root, scopePath),
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
		if current.path != scopePath && b.isExcludedPath(root.ID, current.path) {
			// Exclusions are a correctness boundary, not just a traversal shortcut.
			// If this directory reached the queue from an older plan, dropping it
			// before writing either the directory row or entry keeps ownership with
			// the dynamic root.
			continue
		}

		dirEntries, readErr := os.ReadDir(current.path)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				// The snapshot builder used to record the directory as existing
				// before ReadDir. A temp/build directory can disappear in that tiny
				// window, so missing scopes now produce an empty owned snapshot that
				// prunes stale rows without retrying the whole root.
				if current.path == scopePath {
					return SubtreeSnapshotBatch{RootID: root.ID, ScopePath: scopePath}, nil
				}
				continue
			}
			batch.Directories = append(batch.Directories, DirectoryRecord{
				Path:         current.path,
				RootID:       root.ID,
				ParentPath:   filepath.Dir(current.path),
				LastScanTime: scanTimestamp,
				Exists:       true,
			})
			batch.Entries = append(batch.Entries, newEntryRecord(root, current.path, current.info))
			if current.path == scopePath {
				return batch, fmt.Errorf("failed to read scope directory %s: %w", current.path, readErr)
			}
			util.GetLogger().Warn(ctx, "filesearch skipped unreadable directory "+current.path+": "+readErr.Error())
			continue
		}
		batch.Directories = append(batch.Directories, DirectoryRecord{
			Path:         current.path,
			RootID:       root.ID,
			ParentPath:   filepath.Dir(current.path),
			LastScanTime: scanTimestamp,
			Exists:       true,
		})
		batch.Entries = append(batch.Entries, newEntryRecord(root, current.path, current.info))

		for _, dirEntry := range dirEntries {
			childPath := filepath.Join(current.path, dirEntry.Name())
			isDir, info, infoErr := strictDirEntryType(current.path, dirEntry)
			if infoErr != nil {
				continue
			}
			if isDir && b.isExcludedPath(root.ID, childPath) {
				// The dynamic child directory itself is excluded before entry
				// materialization and before BFS enqueueing. Otherwise SQLite's
				// path-unique upsert would silently move that path back to the parent.
				continue
			}

			if shouldSkipSystemPath(childPath, isDir) {
				continue
			}
			if !current.policy.ShouldIndexPath(childPath, isDir) {
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

			// Optimization: recursive snapshots now carry the same traversal policy
			// context as the streaming path. The previous per-path callback rebuilt
			// ignore ancestors for every child, while the queued context keeps
			// .gitignore/configured-rule state aligned with the accepted directory.
			queue = append(queue, queueItem{
				path:   childPath,
				info:   info,
				policy: current.policy.Descend(childPath),
			})
		}
	}

	return batch, nil
}

// StreamSubtreeJobBatches walks one recursive subtree once and emits bounded
// batches to the caller. Full indexing no longer performs an exact pre-scan, so
// this streaming path is the primary large-root traversal instead of a second
// copy of the planner's earlier walk.
func (b *SnapshotBuilder) StreamSubtreeJobBatches(ctx context.Context, root RootRecord, job Job, onBatch func(SubtreeSnapshotBatch) error) error {
	if onBatch == nil {
		return fmt.Errorf("subtree batch callback is required")
	}
	if job.Kind != JobKindSubtree {
		return fmt.Errorf("stream subtree requires kind %q, got %q", JobKindSubtree, job.Kind)
	}

	scopePath := filepath.Clean(job.ScopePath)
	info, err := b.validateScopePath(root, scopePath)
	if err != nil {
		return err
	}
	if info == nil {
		return onBatch(SubtreeSnapshotBatch{RootID: root.ID, ScopePath: scopePath})
	}
	if !info.IsDir() {
		return onBatch(SubtreeSnapshotBatch{
			RootID:    root.ID,
			ScopePath: scopePath,
			Entries:   []EntryRecord{newEntryRecord(root, scopePath, info)},
		})
	}

	diagnostics := &subtreeStreamDiagnostics{}
	defer diagnostics.log(ctx, scopePath)

	type queueItem struct {
		path   string
		info   os.FileInfo
		policy TraversalPolicyContext
	}

	scanTimestamp := time.Now().UnixMilli()
	// Optimization: a streaming full-run scope can create tens of thousands of
	// entries. Reusing one update timestamp per scope removes a hot per-entry
	// clock call while preserving the existing "this scan wrote this row" marker.
	entryUpdatedAt := scanTimestamp
	maxRecordsPerBatch := b.directFileBatchSize
	if maxRecordsPerBatch <= 0 {
		maxRecordsPerBatch = defaultSplitBudget().DirectFileBatchSize
	}
	if maxRecordsPerBatch <= 0 {
		maxRecordsPerBatch = 1024
	}

	newBatch := func() SubtreeSnapshotBatch {
		return SubtreeSnapshotBatch{
			RootID:    root.ID,
			ScopePath: scopePath,
		}
	}
	flushBatch := func(batch *SubtreeSnapshotBatch) error {
		if batch == nil {
			return nil
		}
		if len(batch.Directories) == 0 && len(batch.Entries) == 0 {
			return nil
		}
		if err := onBatch(*batch); err != nil {
			return err
		}
		*batch = newBatch()
		return nil
	}
	shouldFlush := func(batch SubtreeSnapshotBatch) bool {
		return len(batch.Directories)+len(batch.Entries) >= maxRecordsPerBatch
	}

	queue := []queueItem{{path: scopePath, info: info, policy: b.policy.newTraversalContext(root, scopePath)}}
	batch := newBatch()
	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		current := queue[0]
		queue = queue[1:]
		if current.path != scopePath && b.isExcludedPath(root.ID, current.path) {
			continue
		}

		readStartedAt := time.Now()
		dirEntries, readErr := os.ReadDir(current.path)
		diagnostics.recordReadDir(time.Since(readStartedAt))
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				if current.path == scopePath {
					return nil
				}
				continue
			}
			batch.Directories = append(batch.Directories, DirectoryRecord{
				Path:         current.path,
				RootID:       root.ID,
				ParentPath:   filepath.Dir(current.path),
				LastScanTime: scanTimestamp,
				Exists:       true,
			})
			batch.Entries = append(batch.Entries, newEntryRecordWithUpdatedAt(root, current.path, current.info, entryUpdatedAt))
			if current.path == scopePath {
				return fmt.Errorf("failed to read scope directory %s: %w", current.path, readErr)
			}
			util.GetLogger().Warn(ctx, "filesearch skipped unreadable directory "+current.path+": "+readErr.Error())
			if shouldFlush(batch) {
				if err := flushBatch(&batch); err != nil {
					return err
				}
			}
			continue
		}

		batch.Directories = append(batch.Directories, DirectoryRecord{
			Path:         current.path,
			RootID:       root.ID,
			ParentPath:   filepath.Dir(current.path),
			LastScanTime: scanTimestamp,
			Exists:       true,
		})
		batch.Entries = append(batch.Entries, newEntryRecordWithUpdatedAt(root, current.path, current.info, entryUpdatedAt))
		if shouldFlush(batch) {
			if err := flushBatch(&batch); err != nil {
				return err
			}
		}

		for _, dirEntry := range dirEntries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			childPath := filepath.Join(current.path, dirEntry.Name())
			typeStartedAt := time.Now()
			isDir, childInfo, infoErr := strictDirEntryType(current.path, dirEntry)
			diagnostics.recordDirEntryType(time.Since(typeStartedAt))
			if infoErr != nil {
				continue
			}
			if isDir && b.isExcludedPath(root.ID, childPath) {
				continue
			}
			if shouldSkipSystemPath(childPath, isDir) {
				continue
			}
			policyStartedAt := time.Now()
			shouldIndex := current.policy.ShouldIndexPath(childPath, isDir)
			diagnostics.recordPolicyCheck(time.Since(policyStartedAt))
			if !shouldIndex {
				continue
			}
			if childInfo == nil {
				infoStartedAt := time.Now()
				childInfo, infoErr = strictDirEntryInfo(current.path, dirEntry)
				diagnostics.recordDirEntryInfo(time.Since(infoStartedAt))
				if infoErr != nil {
					continue
				}
			}
			if isDir {
				queue = append(queue, queueItem{
					path:   childPath,
					info:   childInfo,
					policy: current.policy.Descend(childPath),
				})
				continue
			}

			batch.Entries = append(batch.Entries, newEntryRecordWithUpdatedAt(root, childPath, childInfo, entryUpdatedAt))
			if shouldFlush(batch) {
				if err := flushBatch(&batch); err != nil {
					return err
				}
			}
		}
	}

	return flushBatch(&batch)
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
	if scopePath != filepath.Clean(root.Path) && b.isExcludedPath(root.ID, scopePath) {
		// A stale parent-root dirty batch can still point at a path that has since
		// been promoted. Returning an empty owned snapshot lets the DB prune only
		// the parent-owned rows at that scope while leaving the dynamic root's rows
		// untouched.
		return nil, nil
	}

	info, err := os.Stat(scopePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	if scopePath != filepath.Clean(root.Path) && !b.shouldIndexScopePath(root, scopePath, info.IsDir()) {
		return nil, nil
	}

	return info, nil
}

func (b *SnapshotBuilder) isExcludedPath(rootID string, path string) bool {
	if b == nil || len(b.rootExclusions) == 0 {
		return false
	}
	for _, excludedPath := range b.rootExclusions[rootID] {
		if pathWithinScope(excludedPath, path) {
			return true
		}
	}
	return false
}

func (b *SnapshotBuilder) shouldIndexScopePath(root RootRecord, path string, isDir bool) bool {
	if shouldSkipSystemPath(path, isDir) {
		return false
	}
	if b == nil || b.policy == nil {
		return true
	}
	cleanPath := filepath.Clean(path)
	context := b.policy.newTraversalContext(root, filepath.Dir(cleanPath))
	// Bug fix: dirty-scope validation now uses the same traversal policy as the
	// subtree scanner. Keeping scope checks on the removed per-path callback would
	// let ignored dirty paths enter execution through a different matcher.
	return context.ShouldIndexPath(cleanPath, isDir)
}
