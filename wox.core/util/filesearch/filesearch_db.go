package filesearch

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
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

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
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
	`

	_, err := d.db.ExecContext(ctx, createSQL)
	return err
}

func (d *FileSearchDB) UpsertRoot(ctx context.Context, root RootRecord) error {
	query := `
	INSERT INTO roots (id, path, kind, status, progress_current, progress_total, last_error, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(path) DO UPDATE SET
		kind = excluded.kind,
		status = excluded.status,
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
	SET status = ?, progress_current = ?, progress_total = ?, last_error = ?, updated_at = ?
	WHERE id = ?
	`

	_, err := d.db.ExecContext(
		ctx,
		query,
		string(root.Status),
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
		SELECT id, path, kind, status, progress_current, progress_total, last_error, created_at, updated_at
		FROM roots
		ORDER BY path ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roots []RootRecord
	for rows.Next() {
		var root RootRecord
		var kind string
		var status string
		if err := rows.Scan(
			&root.ID,
			&root.Path,
			&kind,
			&status,
			&root.ProgressCurrent,
			&root.ProgressTotal,
			&root.LastError,
			&root.CreatedAt,
			&root.UpdatedAt,
		); err != nil {
			return nil, err
		}
		root.Kind = RootKind(kind)
		root.Status = RootStatus(status)
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
		SELECT id, path, kind, status, progress_current, progress_total, last_error, created_at, updated_at
		FROM roots
		WHERE path = ?
	`, rootPath)

	var root RootRecord
	var kind string
	var status string
	if err := row.Scan(
		&root.ID,
		&root.Path,
		&kind,
		&status,
		&root.ProgressCurrent,
		&root.ProgressTotal,
		&root.LastError,
		&root.CreatedAt,
		&root.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	root.Kind = RootKind(kind)
	root.Status = RootStatus(status)
	return &root, nil
}

func (d *FileSearchDB) ReplaceRootEntries(ctx context.Context, root RootRecord, entries []EntryRecord) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE root_id = ?`, root.ID); err != nil {
		return err
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

	for _, entry := range entries {
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
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
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
