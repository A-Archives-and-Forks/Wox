package filesearch

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"wox/util"

	_ "github.com/mattn/go-sqlite3"
)

type FileSearchDB struct {
	db *sql.DB
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

	fileSearchDB := &FileSearchDB{db: db}
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
	createSQL := `
	CREATE TABLE IF NOT EXISTS meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS roots (
		id TEXT PRIMARY KEY,
		path TEXT NOT NULL UNIQUE,
		kind TEXT NOT NULL,
		status TEXT NOT NULL,
		feed_type TEXT NOT NULL DEFAULT '',
		feed_cursor TEXT NOT NULL DEFAULT '',
		feed_state TEXT NOT NULL DEFAULT '',
		last_reconcile_at INTEGER NOT NULL DEFAULT 0,
		last_full_scan_at INTEGER NOT NULL DEFAULT 0,
		progress_current INTEGER NOT NULL DEFAULT 0,
		progress_total INTEGER NOT NULL DEFAULT 0,
		last_error TEXT,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS entries (
		path TEXT PRIMARY KEY,
		root_id TEXT NOT NULL,
		parent_path TEXT NOT NULL,
		name TEXT NOT NULL,
		normalized_name TEXT NOT NULL,
		normalized_path TEXT NOT NULL,
		pinyin_full TEXT,
		pinyin_initials TEXT,
		is_dir INTEGER NOT NULL,
		mtime INTEGER NOT NULL,
		size INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_entries_root_id ON entries(root_id);
	CREATE INDEX IF NOT EXISTS idx_entries_parent_path ON entries(parent_path);
	CREATE INDEX IF NOT EXISTS idx_entries_normalized_name ON entries(normalized_name);
	CREATE INDEX IF NOT EXISTS idx_entries_pinyin_full ON entries(pinyin_full);
	CREATE INDEX IF NOT EXISTS idx_entries_pinyin_initials ON entries(pinyin_initials);
	CREATE INDEX IF NOT EXISTS idx_entries_is_dir ON entries(is_dir);

	CREATE TABLE IF NOT EXISTS directories (
		path TEXT PRIMARY KEY,
		root_id TEXT NOT NULL,
		parent_path TEXT NOT NULL,
		last_scan_time INTEGER NOT NULL,
		"exists" INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_directories_root_id ON directories(root_id);
	CREATE INDEX IF NOT EXISTS idx_directories_parent_path ON directories(parent_path);
	`

	_, err := d.db.ExecContext(ctx, createSQL)
	if err != nil {
		return err
	}

	alterTableSQLs := []string{
		`ALTER TABLE roots ADD COLUMN feed_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE roots ADD COLUMN feed_cursor TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE roots ADD COLUMN feed_state TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE roots ADD COLUMN last_reconcile_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE roots ADD COLUMN last_full_scan_at INTEGER NOT NULL DEFAULT 0`,
	}

	for _, alterSQL := range alterTableSQLs {
		_, alterErr := d.db.ExecContext(ctx, alterSQL)
		if alterErr != nil && !strings.Contains(alterErr.Error(), "duplicate column name") {
			return alterErr
		}
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE root_id = ?`, root.ID); err != nil {
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

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO entries (
			path, root_id, parent_path, name, normalized_name, normalized_path,
			pinyin_full, pinyin_initials, is_dir, mtime, size, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	totalEntries := int64(len(entries))
	reportProgress(ReplaceEntriesProgress{
		Stage: ReplaceEntriesStageWriting,
		Total: totalEntries,
	})

	lastReportedCurrent := int64(-1)
	lastReportedAt := time.Now()

	for index, entry := range entries {
		if _, err := stmt.ExecContext(
			ctx,
			entry.Path,
			entry.RootID,
			entry.ParentPath,
			entry.Name,
			entry.NormalizedName,
			entry.NormalizedPath,
			entry.PinyinFull,
			entry.PinyinInitials,
			boolToInt(entry.IsDir),
			entry.Mtime,
			entry.Size,
			entry.UpdatedAt,
		); err != nil {
			return err
		}

		currentEntries := int64(index + 1)
		if currentEntries == totalEntries || currentEntries%progressBatchSize == 0 || time.Since(lastReportedAt) >= progressUpdateGap {
			if currentEntries != lastReportedCurrent {
				reportProgress(ReplaceEntriesProgress{
					Stage:   ReplaceEntriesStageWriting,
					Current: currentEntries,
					Total:   totalEntries,
				})
				lastReportedCurrent = currentEntries
				lastReportedAt = time.Now()
			}
		}
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

func (d *FileSearchDB) ReplaceSubtreeSnapshot(ctx context.Context, batch SubtreeSnapshotBatch) error {
	return d.ReplaceSubtreeSnapshots(ctx, []SubtreeSnapshotBatch{batch})
}

func (d *FileSearchDB) ReplaceSubtreeSnapshots(ctx context.Context, batches []SubtreeSnapshotBatch) error {
	if len(batches) == 0 {
		return nil
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	directoryStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			root_id = excluded.root_id,
			parent_path = excluded.parent_path,
			last_scan_time = excluded.last_scan_time,
			"exists" = excluded."exists"
	`)
	if err != nil {
		return err
	}
	defer directoryStmt.Close()

	entryStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO entries (
			path, root_id, parent_path, name, normalized_name, normalized_path,
			pinyin_full, pinyin_initials, is_dir, mtime, size, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer entryStmt.Close()

	for _, batch := range batches {
		root, err := lockRootForSubtreeSnapshot(ctx, tx, batch.RootID)
		if err != nil {
			return err
		}
		if !pathWithinScope(root.Path, batch.ScopePath) {
			return fmt.Errorf("subtree snapshot scope path %q is outside root path %q", batch.ScopePath, root.Path)
		}

		if err := validateSubtreeSnapshotBatch(batch); err != nil {
			return err
		}

		if err := deleteScopedRows(tx, ctx, "entries", batch.RootID, batch.ScopePath); err != nil {
			return err
		}
		if err := tombstoneScopedDirectories(tx, ctx, batch.RootID, batch.ScopePath, subtreeBatchScanTime(batch)); err != nil {
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

		for _, entry := range batch.Entries {
			if _, err := entryStmt.ExecContext(
				ctx,
				entry.Path,
				entry.RootID,
				entry.ParentPath,
				entry.Name,
				entry.NormalizedName,
				entry.NormalizedPath,
				entry.PinyinFull,
				entry.PinyinInitials,
				boolToInt(entry.IsDir),
				entry.Mtime,
				entry.Size,
				entry.UpdatedAt,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
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
		SELECT path, root_id, parent_path, name, normalized_name, normalized_path,
		       pinyin_full, pinyin_initials, is_dir, mtime, size, updated_at
		FROM entries
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []EntryRecord
	for rows.Next() {
		var entry EntryRecord
		var isDir int
		if err := rows.Scan(
			&entry.Path,
			&entry.RootID,
			&entry.ParentPath,
			&entry.Name,
			&entry.NormalizedName,
			&entry.NormalizedPath,
			&entry.PinyinFull,
			&entry.PinyinInitials,
			&isDir,
			&entry.Mtime,
			&entry.Size,
			&entry.UpdatedAt,
		); err != nil {
			return nil, err
		}
		entry.IsDir = isDir == 1
		entries = append(entries, entry)
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
