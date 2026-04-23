package filesearch

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"
	"wox/util"

	_ "github.com/mattn/go-sqlite3"
)

type FileSearchDB struct {
	db     *sql.DB
	dbPath string
	// Bulk sync mode defers expensive FTS maintenance until the full scan cycle
	// finishes. The previous all-at-once in-memory index build avoided per-entry
	// write amplification, so the SQLite-first path needs an explicit bulk gate
	// to keep full rescans from thrashing the FTS tables.
	bulkSyncMu    sync.Mutex
	bulkSyncDepth int
}

func NewFileSearchDB(ctx context.Context) (*FileSearchDB, error) {
	dbPath := filepath.Join(util.GetLocation().GetFileSearchDirectory(), "filesearch.db")
	dsn := dbPath + "?" +
		"_journal_mode=WAL&" +
		"_synchronous=NORMAL&" +
		"_cache_size=2000&" +
		"_foreign_keys=true&" +
		"_busy_timeout=5000"

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open filesearch database: %w", err)
	}

	// File indexing uses long write transactions. Allow a few extra read
	// connections so queries and status polling can keep using the last
	// committed snapshot instead of blocking behind the writer.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)

	fileSearchDB := &FileSearchDB{db: db, dbPath: dbPath}
	if err := fileSearchDB.initTables(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return fileSearchDB, nil
}

func (d *FileSearchDB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

func (d *FileSearchDB) initTables(ctx context.Context) error {
	if err := d.ensureBaseTables(ctx); err != nil {
		return err
	}
	if err := d.ensureSQLiteSearchSchema(ctx); err != nil {
		return err
	}
	return nil
}

func (d *FileSearchDB) UpsertRoot(ctx context.Context, root RootRecord) error {
	query := `
	INSERT INTO roots (
		id, path, kind, status, feed_type, feed_cursor, feed_state, last_reconcile_at, last_full_scan_at,
		progress_current, progress_total, last_error, created_at, updated_at
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(path) DO UPDATE SET
		kind = excluded.kind,
		status = excluded.status,
		feed_type = excluded.feed_type,
		feed_cursor = excluded.feed_cursor,
		feed_state = excluded.feed_state,
		last_reconcile_at = excluded.last_reconcile_at,
		last_full_scan_at = excluded.last_full_scan_at,
		progress_current = excluded.progress_current,
		progress_total = excluded.progress_total,
		last_error = excluded.last_error,
		updated_at = excluded.updated_at
	`

	_, err := d.db.ExecContext(
		ctx,
		query,
		root.ID,
		root.Path,
		string(root.Kind),
		string(root.Status),
		string(root.FeedType),
		root.FeedCursor,
		string(root.FeedState),
		root.LastReconcileAt,
		root.LastFullScanAt,
		root.ProgressCurrent,
		root.ProgressTotal,
		root.LastError,
		root.CreatedAt,
		root.UpdatedAt,
	)
	return err
}

func (d *FileSearchDB) UpdateRootState(ctx context.Context, root RootRecord) error {
	query := `
	UPDATE roots
	SET status = ?, feed_type = ?, feed_cursor = ?, feed_state = ?, last_reconcile_at = ?, last_full_scan_at = ?,
	    progress_current = ?, progress_total = ?, last_error = ?, updated_at = ?
	WHERE id = ?
	`

	_, err := d.db.ExecContext(
		ctx,
		query,
		string(root.Status),
		string(root.FeedType),
		root.FeedCursor,
		string(root.FeedState),
		root.LastReconcileAt,
		root.LastFullScanAt,
		root.ProgressCurrent,
		root.ProgressTotal,
		root.LastError,
		root.UpdatedAt,
		root.ID,
	)
	return err
}

func (d *FileSearchDB) ListRoots(ctx context.Context) ([]RootRecord, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, path, kind, status, feed_type, feed_cursor, feed_state, last_reconcile_at, last_full_scan_at,
		       progress_current, progress_total, last_error, created_at, updated_at
		FROM roots
		ORDER BY path ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roots []RootRecord
	for rows.Next() {
		root, err := scanRootRecord(rows)
		if err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}

	return roots, rows.Err()
}

func (d *FileSearchDB) DeleteRoot(ctx context.Context, rootID string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete search artifacts from the persisted facts so root removal no longer
	// depends on the in-memory snapshot helpers that were removed by the SQLite-first path.
	rows, err := selectStoredEntriesTx(ctx, tx, `
		SELECT entry_id, path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
		       pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
		FROM entries
		WHERE root_id = ?
		ORDER BY path ASC
	`, rootID)
	if err != nil {
		return err
	}
	artifactSync, err := newEntrySearchArtifactSyncTx(ctx, tx)
	if err != nil {
		return err
	}
	defer artifactSync.Close()
	for _, row := range rows {
		if err := deleteEntrySearchArtifactsWithSyncTx(ctx, artifactSync, row); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM directories WHERE root_id = ?`, rootID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE root_id = ?`, rootID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM roots WHERE id = ?`, rootID); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *FileSearchDB) FindRootByPath(ctx context.Context, rootPath string) (*RootRecord, error) {
	row := d.db.QueryRowContext(ctx, `
		SELECT id, path, kind, status, feed_type, feed_cursor, feed_state, last_reconcile_at, last_full_scan_at,
		       progress_current, progress_total, last_error, created_at, updated_at
		FROM roots
		WHERE path = ?
	`, rootPath)

	root, err := scanRootRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &root, nil
}

func (d *FileSearchDB) ReplaceRootSnapshot(
	ctx context.Context,
	root RootRecord,
	directories []DirectoryRecord,
	entries []EntryRecord,
	onProgress func(ReplaceEntriesProgress),
) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := validateSubtreeSnapshotBatch(SubtreeSnapshotBatch{
		RootID:      root.ID,
		ScopePath:   root.Path,
		Directories: directories,
		Entries:     entries,
	}); err != nil {
		return err
	}

	reportProgress := func(progress ReplaceEntriesProgress) {
		if onProgress == nil {
			return
		}
		onProgress(progress)
	}

	reportProgress(ReplaceEntriesProgress{Stage: ReplaceEntriesStagePreparing})

	if _, err := tx.ExecContext(ctx, `DELETE FROM directories WHERE root_id = ?`, root.ID); err != nil {
		return err
	}

	directoryStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer directoryStmt.Close()

	for _, directory := range directories {
		if _, err := directoryStmt.ExecContext(
			ctx,
			directory.Path,
			directory.RootID,
			directory.ParentPath,
			directory.LastScanTime,
			boolToInt(directory.Exists),
		); err != nil {
			return err
		}
	}

	totalEntries := int64(len(entries))
	// Root replacement used to delete every row and reinsert it, which changed
	// rowids and broke any external index keyed by the persisted entry identity.
	// The SQLite search tables now depend on stable entry_id values, so root
	// snapshots must upsert facts and delete only the stale paths.
	if totalEntries == 0 {
		reportProgress(ReplaceEntriesProgress{
			Stage:   ReplaceEntriesStageWriting,
			Current: 1,
			Total:   1,
		})
	} else {
		reportProgress(ReplaceEntriesProgress{
			Stage: ReplaceEntriesStageWriting,
			Total: totalEntries,
		})
	}
	if err := d.replaceRootEntriesTx(ctx, tx, root, entries, func(current int64, total int64) {
		reportProgress(ReplaceEntriesProgress{
			Stage:   ReplaceEntriesStageWriting,
			Current: current,
			Total:   total,
		})
	}); err != nil {
		return err
	}
	reportProgress(ReplaceEntriesProgress{Stage: ReplaceEntriesStageFinalizing})

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (d *FileSearchDB) ReplaceRootEntries(ctx context.Context, root RootRecord, entries []EntryRecord, onProgress func(ReplaceEntriesProgress)) error {
	return d.ReplaceRootSnapshot(ctx, root, nil, entries, onProgress)
}

func (d *FileSearchDB) ApplyDirectFilesJob(ctx context.Context, job Job, batch SubtreeSnapshotBatch) error {
	if job.Kind != JobKindDirectFiles {
		return fmt.Errorf("apply direct-files job requires kind %q, got %q", JobKindDirectFiles, job.Kind)
	}
	if err := validateJobSnapshotBatch(job, batch); err != nil {
		return err
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	root, err := lockRootForSubtreeSnapshot(ctx, tx, batch.RootID)
	if err != nil {
		return err
	}
	if !pathWithinScope(root.Path, batch.ScopePath) {
		return fmt.Errorf("direct-files job scope path %q is outside root path %q", batch.ScopePath, root.Path)
	}

	directoryStmt, err := prepareDirectoryUpsertStmtTx(ctx, tx)
	if err != nil {
		return err
	}
	defer directoryStmt.Close()

	for _, directory := range batch.Directories {
		if _, err := directoryStmt.ExecContext(
			ctx,
			directory.Path,
			directory.RootID,
			directory.ParentPath,
			directory.LastScanTime,
			boolToInt(directory.Exists),
		); err != nil {
			return err
		}
	}

	// Bounded direct-file jobs used to share the root-wide replace path, which
	// deleted sibling chunks and subtree scopes before their own jobs ran. The
	// job-oriented path now upserts only the rows owned by this job so replay is
	// safe and unrelated sibling scopes remain intact until their jobs apply.
	if err := d.applyDirectFilesEntriesTx(ctx, tx, batch); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *FileSearchDB) ApplyDirectFilesJobStream(ctx context.Context, root RootRecord, job Job, snapshot *SnapshotBuilder) error {
	if job.Kind != JobKindDirectFiles {
		return fmt.Errorf("apply direct-files stream requires kind %q, got %q", JobKindDirectFiles, job.Kind)
	}
	if snapshot == nil {
		return fmt.Errorf("direct-files stream requires a snapshot builder")
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	lockedRoot, err := lockRootForSubtreeSnapshot(ctx, tx, root.ID)
	if err != nil {
		return err
	}
	if !pathWithinScope(lockedRoot.Path, job.ScopePath) {
		return fmt.Errorf("direct-files job scope path %q is outside root path %q", job.ScopePath, lockedRoot.Path)
	}

	directoryStmt, err := prepareDirectoryUpsertStmtTx(ctx, tx)
	if err != nil {
		return err
	}
	defer directoryStmt.Close()

	stageStmt, err := prepareEntryStageInsertStmtTx(ctx, tx)
	if err != nil {
		return err
	}
	defer stageStmt.Close()

	// Direct-files jobs now own the whole directory scope. Streaming each batch
	// into the temporary stage table keeps SQLite writes bounded without losing
	// the single-scope stale prune that chunked jobs could not express safely.
	if err := snapshot.StreamDirectFilesJobBatches(ctx, *lockedRoot, job, func(batch SubtreeSnapshotBatch) error {
		if err := validateJobSnapshotBatch(job, batch); err != nil {
			return err
		}
		for _, directory := range batch.Directories {
			if _, err := directoryStmt.ExecContext(
				ctx,
				directory.Path,
				directory.RootID,
				directory.ParentPath,
				directory.LastScanTime,
				boolToInt(directory.Exists),
			); err != nil {
				return err
			}
		}
		return stageEntryRecordsWithStmtTx(ctx, stageStmt, batch.Entries)
	}); err != nil {
		return err
	}

	if err := d.replaceDirectFilesEntriesFromStageTx(ctx, tx, job.RootID, job.ScopePath); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *FileSearchDB) ApplySubtreeJob(ctx context.Context, job Job, batch SubtreeSnapshotBatch) error {
	if job.Kind != JobKindSubtree {
		return fmt.Errorf("apply subtree job requires kind %q, got %q", JobKindSubtree, job.Kind)
	}
	if err := validateJobSnapshotBatch(job, batch); err != nil {
		return err
	}

	// Subtree jobs still own a complete recursive scope, so the existing scoped
	// replace helper remains correct and keeps this job-oriented wrapper small.
	return d.ReplaceSubtreeSnapshot(ctx, batch)
}

func (d *FileSearchDB) FinalizeRootRun(ctx context.Context, root RootRecord) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := lockRootForSubtreeSnapshot(ctx, tx, root.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM directories
		WHERE root_id = ?
		  AND "exists" = 0
	`, root.ID); err != nil {
		return err
	}
	if err := d.finalizeRootRunTx(ctx, tx, root); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	d.checkpointWALAfterFinalize(ctx)
	return nil
}

func (d *FileSearchDB) ReplaceSubtreeSnapshot(ctx context.Context, batch SubtreeSnapshotBatch) error {
	return d.ReplaceSubtreeSnapshots(ctx, []SubtreeSnapshotBatch{batch})
}

func (d *FileSearchDB) ReplaceSubtreeSnapshots(ctx context.Context, batches []SubtreeSnapshotBatch) error {
	if len(batches) == 0 {
		return nil
	}

	startedAt := util.GetSystemTimestamp()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	directoryStmt, err := prepareDirectoryUpsertStmtTx(ctx, tx)
	if err != nil {
		return err
	}
	defer directoryStmt.Close()

	for _, batch := range batches {
		lockStartedAt := util.GetSystemTimestamp()
		root, err := lockRootForSubtreeSnapshot(ctx, tx, batch.RootID)
		if err != nil {
			return err
		}
		// Small subtree jobs were still taking ~0.8s even when the changed-set replay
		// itself was cheap. Logging the root lock separately shows whether the fixed
		// cost starts with transaction-level contention before any diff work runs.
		logFilesearchSQLiteMaintenance(ctx, "subtree_lock_root", batch.ScopePath, util.GetSystemTimestamp()-lockStartedAt, 1)
		if !pathWithinScope(root.Path, batch.ScopePath) {
			return fmt.Errorf("subtree snapshot scope path %q is outside root path %q", batch.ScopePath, root.Path)
		}

		if err := validateSubtreeSnapshotBatch(batch); err != nil {
			return err
		}

		tombstoneStartedAt := util.GetSystemTimestamp()
		if err := tombstoneScopedDirectories(tx, ctx, batch.RootID, batch.ScopePath, subtreeBatchScanTime(batch)); err != nil {
			return err
		}
		// The previous logs only covered entry-table maintenance, so directory tombstones
		// could hide inside the "apply_snapshot" wall time without attribution.
		logFilesearchSQLiteMaintenance(ctx, "subtree_tombstone_directories", batch.ScopePath, util.GetSystemTimestamp()-tombstoneStartedAt, len(batch.Directories))

		upsertDirectoriesStartedAt := util.GetSystemTimestamp()
		for _, directory := range batch.Directories {
			if _, err := directoryStmt.ExecContext(
				ctx,
				directory.Path,
				directory.RootID,
				directory.ParentPath,
				directory.LastScanTime,
				boolToInt(directory.Exists),
			); err != nil {
				return err
			}
		}
		// Tiny WinSxS subtree scopes often carry very few entries, so even directory
		// upserts need their own timing to tell whether the fixed overhead is in
		// directory bookkeeping or later entry diff/commit work.
		logFilesearchSQLiteMaintenance(ctx, "subtree_upsert_directories", batch.ScopePath, util.GetSystemTimestamp()-upsertDirectoriesStartedAt, len(batch.Directories))

		// Subtree refreshes can update, delete, and rename within the scope. The
		// explicit delete-old -> upsert -> insert-new order keeps FTS and bigram
		// rows aligned with the fact table so incremental reconcile does not leave
		// stale matches behind.
		replaceEntriesStartedAt := util.GetSystemTimestamp()
		if err := d.replaceSubtreeEntriesTx(ctx, tx, batch); err != nil {
			return err
		}
		// The changed-set replay now has inner logs, but the subtree wrapper still owns
		// all entry-side work for a batch. Keeping a parent phase here makes it obvious
		// whether the missing time is inside diff collection or outside the entry path.
		logFilesearchSQLiteMaintenance(ctx, "subtree_replace_entries", batch.ScopePath, util.GetSystemTimestamp()-replaceEntriesStartedAt, len(batch.Entries))
	}

	commitStartedAt := util.GetSystemTimestamp()
	if err := tx.Commit(); err != nil {
		return err
	}
	// Recent traces showed many tiny subtree batches paying a near-constant ~0.8s.
	// Logging commit separately distinguishes SQLite flush/lock cost from the batch
	// body so we can tell whether transaction finalization dominates the slowdown.
	logFilesearchSQLiteMaintenance(ctx, "subtree_commit", fmt.Sprintf("batches=%d", len(batches)), util.GetSystemTimestamp()-commitStartedAt, len(batches))
	logFilesearchSQLiteMaintenance(ctx, "subtree_apply_total", fmt.Sprintf("batches=%d", len(batches)), util.GetSystemTimestamp()-startedAt, len(batches))
	return nil
}

func prepareDirectoryUpsertStmtTx(ctx context.Context, tx *sql.Tx) (*sql.Stmt, error) {
	return tx.PrepareContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			root_id = excluded.root_id,
			parent_path = excluded.parent_path,
			last_scan_time = excluded.last_scan_time,
			"exists" = excluded."exists"
	`)
}

func validateJobSnapshotBatch(job Job, batch SubtreeSnapshotBatch) error {
	if err := validateSubtreeSnapshotBatch(batch); err != nil {
		return err
	}
	if job.RootID == "" {
		return fmt.Errorf("job root id is required")
	}
	if batch.RootID != job.RootID {
		return fmt.Errorf("job root id %q does not match batch root id %q", job.RootID, batch.RootID)
	}
	if filepath.Clean(batch.ScopePath) != filepath.Clean(job.ScopePath) {
		return fmt.Errorf("job scope path %q does not match batch scope path %q", job.ScopePath, batch.ScopePath)
	}
	return nil
}

func subtreeBatchScanTime(batch SubtreeSnapshotBatch) int64 {
	scanTime := int64(0)
	for _, directory := range batch.Directories {
		if directory.LastScanTime > scanTime {
			scanTime = directory.LastScanTime
		}
	}
	return scanTime
}

func tombstoneScopedDirectories(tx *sql.Tx, ctx context.Context, rootID string, scopePath string, scanTime int64) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT path
		FROM directories
		WHERE root_id = ? AND "exists" = 1
	`, rootID)
	if err != nil {
		return err
	}
	defer rows.Close()

	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return err
		}
		if pathWithinScope(scopePath, path) {
			paths = append(paths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, path := range paths {
		if _, err := tx.ExecContext(ctx, `
			UPDATE directories
			SET "exists" = 0, last_scan_time = ?
			WHERE root_id = ? AND path = ?
		`, scanTime, rootID, path); err != nil {
			return err
		}
	}

	return nil
}

func (d *FileSearchDB) DeleteDirectoryTombstones(ctx context.Context, rootID string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM directories
		WHERE root_id = ?
		  AND "exists" = 0
	`, rootID); err != nil {
		return err
	}

	return tx.Commit()
}

func validateSubtreeSnapshotBatch(batch SubtreeSnapshotBatch) error {
	if batch.RootID == "" {
		return fmt.Errorf("subtree snapshot root id is required")
	}

	cleanScope := filepath.Clean(batch.ScopePath)
	if batch.ScopePath == "" || cleanScope == "." || !filepath.IsAbs(cleanScope) {
		return fmt.Errorf("subtree snapshot scope path %q is invalid", batch.ScopePath)
	}

	for _, directory := range batch.Directories {
		if directory.RootID != batch.RootID {
			return fmt.Errorf("directory %q belongs to root %q, want %q", directory.Path, directory.RootID, batch.RootID)
		}
		if filepath.Clean(directory.ParentPath) != filepath.Dir(filepath.Clean(directory.Path)) {
			return fmt.Errorf("directory %q has parent %q, want %q", directory.Path, directory.ParentPath, filepath.Dir(filepath.Clean(directory.Path)))
		}
		if !pathWithinScope(batch.ScopePath, directory.Path) {
			return fmt.Errorf("directory %q is outside subtree scope %q", directory.Path, batch.ScopePath)
		}
	}

	for _, entry := range batch.Entries {
		if entry.RootID != batch.RootID {
			return fmt.Errorf("entry %q belongs to root %q, want %q", entry.Path, entry.RootID, batch.RootID)
		}
		if filepath.Clean(entry.ParentPath) != filepath.Dir(filepath.Clean(entry.Path)) {
			return fmt.Errorf("entry %q has parent %q, want %q", entry.Path, entry.ParentPath, filepath.Dir(filepath.Clean(entry.Path)))
		}
		if !pathWithinScope(batch.ScopePath, entry.Path) {
			return fmt.Errorf("entry %q is outside subtree scope %q", entry.Path, batch.ScopePath)
		}
	}

	return nil
}

func (d *FileSearchDB) FindRootByID(ctx context.Context, rootID string) (*RootRecord, error) {
	row := d.db.QueryRowContext(ctx, `
		SELECT id, path, kind, status, feed_type, feed_cursor, feed_state, last_reconcile_at, last_full_scan_at,
		       progress_current, progress_total, last_error, created_at, updated_at
		FROM roots
		WHERE id = ?
	`, rootID)

	root, err := scanRootRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &root, nil
}

func lockRootForSubtreeSnapshot(ctx context.Context, tx *sql.Tx, rootID string) (*RootRecord, error) {
	result, err := tx.ExecContext(ctx, `
		UPDATE roots
		SET updated_at = updated_at
		WHERE id = ?
	`, rootID)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("root %q not found", rootID)
	}

	row := tx.QueryRowContext(ctx, `
		SELECT id, path, kind, status, feed_type, feed_cursor, feed_state, last_reconcile_at, last_full_scan_at,
		       progress_current, progress_total, last_error, created_at, updated_at
		FROM roots
		WHERE id = ?
	`, rootID)

	root, err := scanRootRecord(row)
	if err != nil {
		return nil, err
	}

	return &root, nil
}

func deleteScopedRows(tx *sql.Tx, ctx context.Context, table string, rootID string, scopePath string) error {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT path FROM %s WHERE root_id = ?`, table), rootID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return err
		}
		if pathWithinScope(scopePath, path) {
			paths = append(paths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, path := range paths {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE root_id = ? AND path = ?`, table), rootID, path); err != nil {
			return err
		}
	}

	return nil
}

func pathWithinScope(scopePath, candidatePath string) bool {
	cleanScope := filepath.Clean(scopePath)
	cleanCandidate := filepath.Clean(candidatePath)

	rel, err := filepath.Rel(cleanScope, cleanCandidate)
	if err != nil {
		return false
	}

	if rel == "." {
		return true
	}

	parentPrefix := ".." + string(filepath.Separator)
	if rel == ".." || len(rel) >= len(parentPrefix) && rel[:len(parentPrefix)] == parentPrefix {
		return false
	}

	return true
}

func (d *FileSearchDB) ListDirectoriesByRoot(ctx context.Context, rootID string) ([]DirectoryRecord, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT path, root_id, parent_path, last_scan_time, "exists"
		FROM directories
		WHERE root_id = ?
		ORDER BY path ASC
	`, rootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var directories []DirectoryRecord
	for rows.Next() {
		var directory DirectoryRecord
		var exists int
		if err := rows.Scan(
			&directory.Path,
			&directory.RootID,
			&directory.ParentPath,
			&directory.LastScanTime,
			&exists,
		); err != nil {
			return nil, err
		}
		directory.Exists = exists == 1
		directories = append(directories, directory)
	}

	return directories, rows.Err()
}

func (d *FileSearchDB) CountDirectoriesByRoot(ctx context.Context, rootID string) (int, error) {
	row := d.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM directories
		WHERE root_id = ? AND "exists" = 1
	`, rootID)

	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}

	return count, nil
}

func (d *FileSearchDB) ListEntries(ctx context.Context) ([]EntryRecord, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT entry_id, path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
		       pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
		FROM entries
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []EntryRecord
	for rows.Next() {
		row, err := scanStoredEntryRecord(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, row.toEntryRecord())
	}

	return entries, rows.Err()
}

func (d *FileSearchDB) ListEntriesByRoot(ctx context.Context, rootID string) ([]EntryRecord, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT entry_id, path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
		       pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
		FROM entries
		WHERE root_id = ?
	`, rootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []EntryRecord
	for rows.Next() {
		row, err := scanStoredEntryRecord(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, row.toEntryRecord())
	}

	return entries, rows.Err()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type rootScanner interface {
	Scan(dest ...any) error
}

func scanRootRecord(scanner rootScanner) (RootRecord, error) {
	var root RootRecord
	var kind string
	var status string
	var feedType string
	var feedState string
	if err := scanner.Scan(
		&root.ID,
		&root.Path,
		&kind,
		&status,
		&feedType,
		&root.FeedCursor,
		&feedState,
		&root.LastReconcileAt,
		&root.LastFullScanAt,
		&root.ProgressCurrent,
		&root.ProgressTotal,
		&root.LastError,
		&root.CreatedAt,
		&root.UpdatedAt,
	); err != nil {
		return RootRecord{}, err
	}

	root.Kind = RootKind(kind)
	root.Status = RootStatus(status)
	root.FeedType = RootFeedType(feedType)
	root.FeedState = RootFeedState(feedState)
	return root, nil
}
