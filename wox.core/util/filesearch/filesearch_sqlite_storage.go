package filesearch

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const fileSearchSchemaVersion = 1

const (
	searchBigramFieldName       = "name"
	searchBigramFieldPinyinFull = "pinyin_full"
)

var filesearchFTSTables = []string{
	"entries_name_fts",
	"entries_path_fts",
	"entries_pinyin_full_fts",
	"entries_initials_fts",
}

type storedEntryRecord struct {
	EntryID        int64
	Path           string
	RootID         string
	ParentPath     string
	Name           string
	NormalizedName string
	NameKey        string
	NormalizedPath string
	PinyinFull     string
	PinyinInitials string
	Extension      string
	IsDir          bool
	Mtime          int64
	Size           int64
	UpdatedAt      int64
}

type sqliteIndexSnapshot struct {
	RootCount          int
	EntryCount         int64
	BigramRowCount     int64
	DBFileBytes        int64
	NameFTSVocab       int64
	PathFTSVocab       int64
	PinyinFullFTSVocab int64
	InitialsFTSVocab   int64
	TopRoots           []sqliteRootSnapshot
}

type sqliteRootSnapshot struct {
	RootID string
	Path   string
	Docs   int64
}

func (d *FileSearchDB) ensureBaseTables(ctx context.Context) error {
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

	if _, err := d.db.ExecContext(ctx, createSQL); err != nil {
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

func (d *FileSearchDB) ensureSQLiteSearchSchema(ctx context.Context) error {
	if err := d.probeFTS5(ctx); err != nil {
		return err
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := migrateEntriesTableIfNeeded(ctx, tx); err != nil {
		return err
	}
	if err := createEntriesIndexes(ctx, tx); err != nil {
		return err
	}
	if err := createSearchTables(ctx, tx); err != nil {
		return err
	}
	if err := rebuildAllSearchArtifactsTx(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, fileSearchSchemaVersion)); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *FileSearchDB) probeFTS5(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, `
		CREATE VIRTUAL TABLE IF NOT EXISTS temp.filesearch_fts5_probe USING fts5(value);
	`); err != nil {
		return fmt.Errorf("filesearch requires sqlite FTS5 support; rebuild with -tags sqlite_fts5: %w", err)
	}
	if _, err := d.db.ExecContext(ctx, `DROP TABLE IF EXISTS temp.filesearch_fts5_probe`); err != nil {
		return fmt.Errorf("drop filesearch FTS5 probe table: %w", err)
	}
	return nil
}

func migrateEntriesTableIfNeeded(ctx context.Context, tx *sql.Tx) error {
	exists, err := tableExists(ctx, tx, "entries")
	if err != nil {
		return err
	}
	if !exists {
		return createEntriesTable(ctx, tx)
	}

	columns, err := tableColumnNames(ctx, tx, "entries")
	if err != nil {
		return err
	}
	if columns["entry_id"] && columns["name_key"] && columns["extension"] {
		return nil
	}

	// Rebuild the entries table once because the old schema used path as the
	// primary key. SQLite-first search needs a stable integer entry_id so FTS and
	// side tables can reference entries without rowid churn.
	if _, err := tx.ExecContext(ctx, `ALTER TABLE entries RENAME TO entries_legacy`); err != nil {
		return fmt.Errorf("rename legacy entries table: %w", err)
	}
	if err := createEntriesTable(ctx, tx); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT path, root_id, parent_path, name, normalized_name, normalized_path,
		       pinyin_full, pinyin_initials, is_dir, mtime, size, updated_at
		FROM entries_legacy
		ORDER BY path ASC
	`)
	if err != nil {
		return fmt.Errorf("load legacy entries: %w", err)
	}
	defer rows.Close()

	insertStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO entries (
			path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
			pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare migrated entry insert: %w", err)
	}
	defer insertStmt.Close()

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
			return fmt.Errorf("scan legacy entry: %w", err)
		}
		entry.IsDir = isDir == 1
		stored := buildStoredEntryRecord(entry)
		if _, err := insertStmt.ExecContext(
			ctx,
			stored.Path,
			stored.RootID,
			stored.ParentPath,
			stored.Name,
			stored.NormalizedName,
			stored.NameKey,
			stored.NormalizedPath,
			nullIfEmpty(stored.PinyinFull),
			nullIfEmpty(stored.PinyinInitials),
			stored.Extension,
			boolToInt(stored.IsDir),
			stored.Mtime,
			stored.Size,
			stored.UpdatedAt,
		); err != nil {
			return fmt.Errorf("insert migrated entry %q: %w", stored.Path, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate legacy entries: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE entries_legacy`); err != nil {
		return fmt.Errorf("drop legacy entries table: %w", err)
	}
	return nil
}

func createEntriesTable(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE entries (
			entry_id INTEGER PRIMARY KEY,
			path TEXT NOT NULL UNIQUE,
			root_id TEXT NOT NULL,
			parent_path TEXT NOT NULL,
			name TEXT NOT NULL,
			normalized_name TEXT NOT NULL,
			name_key TEXT NOT NULL DEFAULT '',
			normalized_path TEXT NOT NULL,
			pinyin_full TEXT NOT NULL DEFAULT '',
			pinyin_initials TEXT NOT NULL DEFAULT '',
			extension TEXT NOT NULL DEFAULT '',
			is_dir INTEGER NOT NULL,
			mtime INTEGER NOT NULL,
			size INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create entries table: %w", err)
	}
	return nil
}

func createEntriesIndexes(ctx context.Context, tx *sql.Tx) error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_entries_root_id ON entries(root_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_parent_path ON entries(parent_path)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_name_key ON entries(name_key)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_extension ON entries(extension)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_is_dir ON entries(is_dir)`,
	}
	for _, statement := range indexes {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func createSearchTables(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS entries_bigram (
			field TEXT NOT NULL,
			gram TEXT NOT NULL,
			entry_id INTEGER NOT NULL,
			PRIMARY KEY(field, gram, entry_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_bigram_entry_id ON entries_bigram(entry_id)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS entries_name_fts USING fts5(
			normalized_name,
			content='entries',
			content_rowid='entry_id',
			tokenize='trigram',
			detail='none'
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS entries_path_fts USING fts5(
			normalized_path,
			content='entries',
			content_rowid='entry_id',
			tokenize='trigram',
			detail='none'
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS entries_pinyin_full_fts USING fts5(
			pinyin_full,
			content='entries',
			content_rowid='entry_id',
			tokenize='trigram',
			detail='none'
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS entries_initials_fts USING fts5(
			pinyin_initials,
			content='entries',
			content_rowid='entry_id',
			tokenize='unicode61',
			prefix='1 2 3 4 5 6 7 8'
		)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	for _, tableName := range filesearchFTSTables {
		if err := configureFTSTableTx(ctx, tx, tableName); err != nil {
			return err
		}
	}
	return nil
}

func rebuildAllSearchArtifactsTx(ctx context.Context, tx *sql.Tx) error {
	// The SQLite-first search tables are derived data. Rebuilding them during
	// schema init keeps migrations deterministic and avoids serving stale FTS or
	// bigram rows from earlier partial schemas.
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries_bigram`); err != nil {
		return fmt.Errorf("clear entries_bigram: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT entry_id, path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
		       pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
		FROM entries
		ORDER BY entry_id ASC
	`)
	if err != nil {
		return fmt.Errorf("load entries for bigram rebuild: %w", err)
	}
	defer rows.Close()

	insertBigramStmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO entries_bigram (field, gram, entry_id)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare bigram rebuild insert: %w", err)
	}
	defer insertBigramStmt.Close()

	for rows.Next() {
		row, err := scanStoredEntryRecord(rows)
		if err != nil {
			return err
		}
		if err := insertEntryBigramsTx(ctx, insertBigramStmt, row); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate entries for bigram rebuild: %w", err)
	}

	for _, tableName := range filesearchFTSTables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s(%s) VALUES('rebuild')`, tableName, tableName)); err != nil {
			return fmt.Errorf("rebuild %s: %w", tableName, err)
		}
	}

	return nil
}

func configureFTSTableTx(ctx context.Context, tx *sql.Tx, tableName string) error {
	commands := []string{
		fmt.Sprintf(`INSERT INTO %s(%s, rank) VALUES('automerge', 8)`, tableName, tableName),
		fmt.Sprintf(`INSERT INTO %s(%s, rank) VALUES('crisismerge', 16)`, tableName, tableName),
		fmt.Sprintf(`INSERT INTO %s(%s, rank) VALUES('usermerge', 4)`, tableName, tableName),
	}
	for _, command := range commands {
		if _, err := tx.ExecContext(ctx, command); err != nil {
			return fmt.Errorf("configure %s: %w", tableName, err)
		}
	}
	return nil
}

func tableExists(ctx context.Context, tx *sql.Tx, tableName string) (bool, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM sqlite_master
		WHERE type IN ('table', 'view') AND name = ?
	`, tableName)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func tableColumnNames(ctx context.Context, tx *sql.Tx, tableName string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func buildStoredEntryRecord(entry EntryRecord) storedEntryRecord {
	normalizedName := normalizeIndexText(entry.NormalizedName)
	if normalizedName == "" {
		normalizedName = normalizeIndexText(entry.Name)
	}

	normalizedPath := normalizeEntryPathKey(entry)
	if normalizedPath == "" {
		normalizedPath = normalizeIndexText(normalizePath(entry.Path))
	}

	pinyinFull := normalizeIndexText(entry.PinyinFull)
	pinyinInitials := normalizeIndexText(entry.PinyinInitials)
	if shouldDropRedundantPinyinPayload(normalizedName, pinyinFull, pinyinInitials) {
		pinyinFull = ""
		pinyinInitials = ""
	}

	return storedEntryRecord{
		Path:           filepath.Clean(entry.Path),
		RootID:         entry.RootID,
		ParentPath:     filepath.Clean(entry.ParentPath),
		Name:           entry.Name,
		NormalizedName: normalizedName,
		NameKey:        keepLettersAndDigits(normalizedName),
		NormalizedPath: normalizedPath,
		PinyinFull:     pinyinFull,
		PinyinInitials: pinyinInitials,
		Extension:      normalizeExtension(filepath.Ext(entry.Name)),
		IsDir:          entry.IsDir,
		Mtime:          entry.Mtime,
		Size:           entry.Size,
		UpdatedAt:      entry.UpdatedAt,
	}
}

func scanStoredEntryRecord(scanner interface{ Scan(dest ...any) error }) (storedEntryRecord, error) {
	var row storedEntryRecord
	var isDir int
	if err := scanner.Scan(
		&row.EntryID,
		&row.Path,
		&row.RootID,
		&row.ParentPath,
		&row.Name,
		&row.NormalizedName,
		&row.NameKey,
		&row.NormalizedPath,
		&row.PinyinFull,
		&row.PinyinInitials,
		&row.Extension,
		&isDir,
		&row.Mtime,
		&row.Size,
		&row.UpdatedAt,
	); err != nil {
		return storedEntryRecord{}, err
	}
	row.IsDir = isDir == 1
	return row, nil
}

func (row storedEntryRecord) toEntryRecord() EntryRecord {
	return EntryRecord{
		Path:           row.Path,
		RootID:         row.RootID,
		ParentPath:     row.ParentPath,
		Name:           row.Name,
		NormalizedName: row.NormalizedName,
		NormalizedPath: row.NormalizedPath,
		PinyinFull:     row.PinyinFull,
		PinyinInitials: row.PinyinInitials,
		IsDir:          row.IsDir,
		Mtime:          row.Mtime,
		Size:           row.Size,
		UpdatedAt:      row.UpdatedAt,
	}
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func insertEntryBigramsTx(ctx context.Context, stmt *sql.Stmt, row storedEntryRecord) error {
	for _, gram := range uniqueNgrams(row.NormalizedName, 2) {
		if _, err := stmt.ExecContext(ctx, searchBigramFieldName, gram, row.EntryID); err != nil {
			return fmt.Errorf("insert name bigram for %q: %w", row.Path, err)
		}
	}
	for _, gram := range uniqueNgrams(row.PinyinFull, 2) {
		if _, err := stmt.ExecContext(ctx, searchBigramFieldPinyinFull, gram, row.EntryID); err != nil {
			return fmt.Errorf("insert pinyin bigram for %q: %w", row.Path, err)
		}
	}
	return nil
}

func nextPrefixUpperBound(prefix string) string {
	if prefix == "" {
		return ""
	}
	runes := []rune(prefix)
	last := len(runes) - 1
	runes[last]++
	return string(runes[:last+1])
}

func uniqueInt64(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	unique := make([]int64, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Slice(unique, func(left int, right int) bool {
		return unique[left] < unique[right]
	})
	return unique
}

func escapeLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func longestLiteralFromWildcard(raw string) string {
	wildcard := buildWildcardQuery(raw)
	if wildcard == nil {
		return ""
	}
	return longestString(buildWildcardLiterals(raw))
}

func trimCandidateIDs(candidateIDs []int64, limit int) []int64 {
	candidateIDs = uniqueInt64(candidateIDs)
	if limit > 0 && len(candidateIDs) > limit {
		return append([]int64(nil), candidateIDs[:limit]...)
	}
	return candidateIDs
}

func utf8LenString(value string) int {
	return utf8.RuneCountInString(value)
}

func (d *FileSearchDB) SearchIndexSnapshot(ctx context.Context) (sqliteIndexSnapshot, error) {
	if d == nil || d.db == nil {
		return sqliteIndexSnapshot{}, nil
	}

	snapshot := sqliteIndexSnapshot{}
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM roots`).Scan(&snapshot.RootCount); err != nil {
		return sqliteIndexSnapshot{}, err
	}
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM entries`).Scan(&snapshot.EntryCount); err != nil {
		return sqliteIndexSnapshot{}, err
	}
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM entries_bigram`).Scan(&snapshot.BigramRowCount); err != nil {
		return sqliteIndexSnapshot{}, err
	}

	var pageCount, pageSize int64
	if err := d.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pageCount); err != nil {
		return sqliteIndexSnapshot{}, err
	}
	if err := d.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return sqliteIndexSnapshot{}, err
	}
	snapshot.DBFileBytes = pageCount * pageSize

	nameVocab, err := countFTSVocabRows(ctx, d.db, "entries_name_fts")
	if err != nil {
		return sqliteIndexSnapshot{}, err
	}
	snapshot.NameFTSVocab = nameVocab
	pathVocab, err := countFTSVocabRows(ctx, d.db, "entries_path_fts")
	if err != nil {
		return sqliteIndexSnapshot{}, err
	}
	snapshot.PathFTSVocab = pathVocab
	pinyinFullVocab, err := countFTSVocabRows(ctx, d.db, "entries_pinyin_full_fts")
	if err != nil {
		return sqliteIndexSnapshot{}, err
	}
	snapshot.PinyinFullFTSVocab = pinyinFullVocab
	initialsVocab, err := countFTSVocabRows(ctx, d.db, "entries_initials_fts")
	if err != nil {
		return sqliteIndexSnapshot{}, err
	}
	snapshot.InitialsFTSVocab = initialsVocab

	rows, err := d.db.QueryContext(ctx, `
		SELECT roots.id, roots.path, count(entries.entry_id) AS docs
		FROM roots
		LEFT JOIN entries ON entries.root_id = roots.id
		GROUP BY roots.id, roots.path
		ORDER BY docs DESC, roots.path ASC
		LIMIT 5
	`)
	if err != nil {
		return sqliteIndexSnapshot{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var root sqliteRootSnapshot
		if err := rows.Scan(&root.RootID, &root.Path, &root.Docs); err != nil {
			return sqliteIndexSnapshot{}, err
		}
		snapshot.TopRoots = append(snapshot.TopRoots, root)
	}
	if err := rows.Err(); err != nil {
		return sqliteIndexSnapshot{}, err
	}

	return snapshot, nil
}

func countFTSVocabRows(ctx context.Context, db *sql.DB, tableName string) (int64, error) {
	vocabTable := fmt.Sprintf("temp.%s_vocab", tableName)
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, vocabTable)); err != nil {
		return 0, err
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE VIRTUAL TABLE %s USING fts5vocab(%s, 'row')`, vocabTable, tableName)); err != nil {
		return 0, err
	}
	defer db.ExecContext(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s`, vocabTable))

	var count int64
	if err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, vocabTable)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (d *FileSearchDB) BeginBulkSync() {
	if d == nil {
		return
	}
	d.bulkSyncMu.Lock()
	d.bulkSyncDepth++
	d.bulkSyncMu.Unlock()
}

func (d *FileSearchDB) EndBulkSync(ctx context.Context) error {
	if d == nil {
		return nil
	}

	d.bulkSyncMu.Lock()
	if d.bulkSyncDepth > 0 {
		d.bulkSyncDepth--
	}
	shouldFinalize := d.bulkSyncDepth == 0
	d.bulkSyncMu.Unlock()

	if !shouldFinalize {
		return nil
	}

	// Bulk mode skips per-entry FTS maintenance during the scan cycle because
	// replaying FTS deletes/inserts for every discovered file is much slower than
	// rebuilding the derived indexes once after the facts settle.
	if err := d.rebuildFTSTables(ctx, true); err != nil {
		return err
	}
	return nil
}

func (d *FileSearchDB) isBulkSyncEnabled() bool {
	if d == nil {
		return false
	}
	d.bulkSyncMu.Lock()
	defer d.bulkSyncMu.Unlock()
	return d.bulkSyncDepth > 0
}

func (d *FileSearchDB) replaceRootEntriesTx(ctx context.Context, tx *sql.Tx, root RootRecord, entries []EntryRecord) error {
	if err := stageEntryRecordsTx(ctx, tx, entries); err != nil {
		return err
	}

	staleRows, changedOldRows, changedOrNewRows, err := collectChangedEntrySetsTx(ctx, tx, root.ID, root.Path)
	if err != nil {
		return err
	}

	if err := applyChangedEntrySetsTx(ctx, tx, staleRows, changedOldRows, changedOrNewRows, !d.isBulkSyncEnabled()); err != nil {
		return err
	}

	if d.isBulkSyncEnabled() {
		if err := refreshRootBigramsTx(ctx, tx, root.ID); err != nil {
			return err
		}
	}

	return nil
}

func (d *FileSearchDB) replaceSubtreeEntriesTx(ctx context.Context, tx *sql.Tx, batch SubtreeSnapshotBatch) error {
	if err := stageEntryRecordsTx(ctx, tx, batch.Entries); err != nil {
		return err
	}

	staleRows, changedOldRows, changedOrNewRows, err := collectChangedEntrySetsTx(ctx, tx, batch.RootID, batch.ScopePath)
	if err != nil {
		return err
	}

	return applyChangedEntrySetsTx(ctx, tx, staleRows, changedOldRows, changedOrNewRows, true)
}

func stageEntryRecordsTx(ctx context.Context, tx *sql.Tx, entries []EntryRecord) error {
	if _, err := tx.ExecContext(ctx, `
			CREATE TEMP TABLE IF NOT EXISTS filesearch_stage_entries (
			path TEXT PRIMARY KEY,
			root_id TEXT NOT NULL,
			parent_path TEXT NOT NULL,
			name TEXT NOT NULL,
			normalized_name TEXT NOT NULL,
			name_key TEXT NOT NULL,
			normalized_path TEXT NOT NULL,
			pinyin_full TEXT NOT NULL,
			pinyin_initials TEXT NOT NULL,
			extension TEXT NOT NULL,
			is_dir INTEGER NOT NULL,
			mtime INTEGER NOT NULL,
			size INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
		`); err != nil {
		return fmt.Errorf("create stage entries table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM filesearch_stage_entries`); err != nil {
		return fmt.Errorf("clear stage entries table: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO filesearch_stage_entries (
			path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
			pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare stage entry insert: %w", err)
	}
	defer stmt.Close()

	for _, entry := range entries {
		row := buildStoredEntryRecord(entry)
		if _, err := stmt.ExecContext(
			ctx,
			row.Path,
			row.RootID,
			row.ParentPath,
			row.Name,
			row.NormalizedName,
			row.NameKey,
			row.NormalizedPath,
			row.PinyinFull,
			row.PinyinInitials,
			row.Extension,
			boolToInt(row.IsDir),
			row.Mtime,
			row.Size,
			row.UpdatedAt,
		); err != nil {
			return fmt.Errorf("stage entry %q: %w", row.Path, err)
		}
	}

	return nil
}

func upsertEntryFactsTx(ctx context.Context, tx *sql.Tx, row storedEntryRecord) (storedEntryRecord, error) {
	mutator, err := newEntryFactMutatorTx(ctx, tx)
	if err != nil {
		return storedEntryRecord{}, err
	}
	defer mutator.Close()
	return upsertEntryFactsWithMutatorTx(ctx, mutator, row)
}

func upsertEntryFactsWithMutatorTx(ctx context.Context, mutator *entryFactMutatorTx, row storedEntryRecord) (storedEntryRecord, error) {
	scanner := mutator.upsertStmt.QueryRowContext(ctx,
		row.Path,
		row.RootID,
		row.ParentPath,
		row.Name,
		row.NormalizedName,
		row.NameKey,
		row.NormalizedPath,
		row.PinyinFull,
		row.PinyinInitials,
		row.Extension,
		boolToInt(row.IsDir),
		row.Mtime,
		row.Size,
		row.UpdatedAt,
	)

	current, err := scanStoredEntryRecord(scanner)
	if err != nil {
		return storedEntryRecord{}, fmt.Errorf("upsert entry %q: %w", row.Path, err)
	}
	return current, nil
}

func collectChangedEntrySetsTx(ctx context.Context, tx *sql.Tx, rootID string, scopePath string) ([]storedEntryRecord, []storedEntryRecord, []storedEntryRecord, error) {
	scopeQuery, scopeArgs := buildEntryScopeQuery(scopePath, "e.path")
	// Compare the staged snapshot against persisted facts inside SQLite so no-op
	// subtree refreshes do not have to materialize every root row back into Go
	// just to discover that nothing changed.
	staleRows, err := selectStoredEntriesTx(ctx, tx, fmt.Sprintf(`
		SELECT e.entry_id, e.path, e.root_id, e.parent_path, e.name, e.normalized_name, e.name_key, e.normalized_path,
		       e.pinyin_full, e.pinyin_initials, e.extension, e.is_dir, e.mtime, e.size, e.updated_at
		FROM entries e
		LEFT JOIN filesearch_stage_entries s ON s.path = e.path
		WHERE e.root_id = ? AND %s AND s.path IS NULL
		ORDER BY e.path ASC
	`, scopeQuery), append([]any{rootID}, scopeArgs...)...)
	if err != nil {
		return nil, nil, nil, err
	}

	diffPredicate := buildEntryDifferencePredicate("e", "s")
	changedOldRows, err := selectStoredEntriesTx(ctx, tx, fmt.Sprintf(`
		SELECT e.entry_id, e.path, e.root_id, e.parent_path, e.name, e.normalized_name, e.name_key, e.normalized_path,
		       e.pinyin_full, e.pinyin_initials, e.extension, e.is_dir, e.mtime, e.size, e.updated_at
		FROM entries e
		INNER JOIN filesearch_stage_entries s ON s.path = e.path
		WHERE e.root_id = ? AND %s AND (%s)
		ORDER BY e.path ASC
	`, scopeQuery, diffPredicate), append([]any{rootID}, scopeArgs...)...)
	if err != nil {
		return nil, nil, nil, err
	}

	changedOrNewRows, err := selectStoredEntriesTx(ctx, tx, fmt.Sprintf(`
		SELECT CAST(COALESCE(e.entry_id, 0) AS INTEGER) AS entry_id,
		       s.path, s.root_id, s.parent_path, s.name, s.normalized_name, s.name_key, s.normalized_path,
		       s.pinyin_full, s.pinyin_initials, s.extension, s.is_dir, s.mtime, s.size, s.updated_at
		FROM filesearch_stage_entries s
		LEFT JOIN entries e ON e.path = s.path
		WHERE e.entry_id IS NULL OR (%s)
		ORDER BY s.path ASC
	`, diffPredicate), nil...)
	if err != nil {
		return nil, nil, nil, err
	}

	return staleRows, changedOldRows, changedOrNewRows, nil
}

func applyChangedEntrySetsTx(ctx context.Context, tx *sql.Tx, staleRows []storedEntryRecord, changedOldRows []storedEntryRecord, changedOrNewRows []storedEntryRecord, syncSearchArtifacts bool) error {
	var artifactSync *entrySearchArtifactSyncTx
	var factMutator *entryFactMutatorTx
	var err error
	if syncSearchArtifacts {
		// Large subtree refreshes spend most of their time replaying the derived
		// search indexes. Reusing prepared statements within the transaction keeps
		// the SQLite-first path from paying a prepare/close round-trip for every row.
		artifactSync, err = newEntrySearchArtifactSyncTx(ctx, tx)
		if err != nil {
			return err
		}
		defer artifactSync.Close()
	}
	if len(staleRows) > 0 || len(changedOrNewRows) > 0 {
		// The SQL-side diff reduced how many rows we touch. Reusing the fact
		// mutation statements preserves that gain instead of reparsing the same
		// RETURNING upsert for every changed entry.
		factMutator, err = newEntryFactMutatorTx(ctx, tx)
		if err != nil {
			return err
		}
		defer factMutator.Close()
	}

	if syncSearchArtifacts {
		for _, existing := range staleRows {
			if err := deleteEntrySearchArtifactsWithSyncTx(ctx, artifactSync, existing); err != nil {
				return err
			}
		}
		for _, existing := range changedOldRows {
			if err := deleteEntrySearchArtifactsWithSyncTx(ctx, artifactSync, existing); err != nil {
				return err
			}
		}
	}

	for _, existing := range staleRows {
		if _, err := factMutator.deleteStmt.ExecContext(ctx, existing.EntryID); err != nil {
			return fmt.Errorf("delete stale entry %q: %w", existing.Path, err)
		}
	}

	for _, staged := range changedOrNewRows {
		current, err := upsertEntryFactsWithMutatorTx(ctx, factMutator, staged)
		if err != nil {
			return err
		}
		if syncSearchArtifacts {
			if err := insertEntrySearchArtifactsWithSyncTx(ctx, artifactSync, current); err != nil {
				return err
			}
		}
	}

	return nil
}

type entryFactMutatorTx struct {
	statements []*sql.Stmt
	upsertStmt *sql.Stmt
	deleteStmt *sql.Stmt
}

func newEntryFactMutatorTx(ctx context.Context, tx *sql.Tx) (*entryFactMutatorTx, error) {
	mutator := &entryFactMutatorTx{}

	prepare := func(query string) (*sql.Stmt, error) {
		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		mutator.statements = append(mutator.statements, stmt)
		return stmt, nil
	}

	var err error
	if mutator.upsertStmt, err = prepare(`
		INSERT INTO entries (
			path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
			pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			root_id = excluded.root_id,
			parent_path = excluded.parent_path,
			name = excluded.name,
			normalized_name = excluded.normalized_name,
			name_key = excluded.name_key,
			normalized_path = excluded.normalized_path,
			pinyin_full = excluded.pinyin_full,
			pinyin_initials = excluded.pinyin_initials,
			extension = excluded.extension,
			is_dir = excluded.is_dir,
			mtime = excluded.mtime,
			size = excluded.size,
			updated_at = excluded.updated_at
		RETURNING entry_id, path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
		          pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
	`); err != nil {
		return nil, fmt.Errorf("prepare entry upsert: %w", err)
	}
	if mutator.deleteStmt, err = prepare(`DELETE FROM entries WHERE entry_id = ?`); err != nil {
		return nil, fmt.Errorf("prepare entry delete: %w", err)
	}

	return mutator, nil
}

func (m *entryFactMutatorTx) Close() {
	if m == nil {
		return
	}
	for _, stmt := range m.statements {
		stmt.Close()
	}
}

type entrySearchArtifactSyncTx struct {
	statements []*sql.Stmt

	deleteNameFTSStmt       *sql.Stmt
	deletePathFTSStmt       *sql.Stmt
	deletePinyinFullFTSStmt *sql.Stmt
	deleteInitialsFTSStmt   *sql.Stmt

	insertNameFTSStmt       *sql.Stmt
	insertPathFTSStmt       *sql.Stmt
	insertPinyinFullFTSStmt *sql.Stmt
	insertInitialsFTSStmt   *sql.Stmt

	deleteBigramStmt *sql.Stmt
	insertBigramStmt *sql.Stmt
}

func newEntrySearchArtifactSyncTx(ctx context.Context, tx *sql.Tx) (*entrySearchArtifactSyncTx, error) {
	syncer := &entrySearchArtifactSyncTx{}

	prepare := func(query string) (*sql.Stmt, error) {
		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		syncer.statements = append(syncer.statements, stmt)
		return stmt, nil
	}

	var err error
	if syncer.deleteNameFTSStmt, err = prepare(`INSERT INTO entries_name_fts(entries_name_fts, rowid, normalized_name) VALUES('delete', ?, ?)`); err != nil {
		return nil, fmt.Errorf("prepare name fts delete: %w", err)
	}
	if syncer.deletePathFTSStmt, err = prepare(`INSERT INTO entries_path_fts(entries_path_fts, rowid, normalized_path) VALUES('delete', ?, ?)`); err != nil {
		return nil, fmt.Errorf("prepare path fts delete: %w", err)
	}
	if syncer.deletePinyinFullFTSStmt, err = prepare(`INSERT INTO entries_pinyin_full_fts(entries_pinyin_full_fts, rowid, pinyin_full) VALUES('delete', ?, ?)`); err != nil {
		return nil, fmt.Errorf("prepare pinyin full fts delete: %w", err)
	}
	if syncer.deleteInitialsFTSStmt, err = prepare(`INSERT INTO entries_initials_fts(entries_initials_fts, rowid, pinyin_initials) VALUES('delete', ?, ?)`); err != nil {
		return nil, fmt.Errorf("prepare initials fts delete: %w", err)
	}

	if syncer.insertNameFTSStmt, err = prepare(`INSERT INTO entries_name_fts(rowid, normalized_name) VALUES(?, ?)`); err != nil {
		return nil, fmt.Errorf("prepare name fts insert: %w", err)
	}
	if syncer.insertPathFTSStmt, err = prepare(`INSERT INTO entries_path_fts(rowid, normalized_path) VALUES(?, ?)`); err != nil {
		return nil, fmt.Errorf("prepare path fts insert: %w", err)
	}
	if syncer.insertPinyinFullFTSStmt, err = prepare(`INSERT INTO entries_pinyin_full_fts(rowid, pinyin_full) VALUES(?, ?)`); err != nil {
		return nil, fmt.Errorf("prepare pinyin full fts insert: %w", err)
	}
	if syncer.insertInitialsFTSStmt, err = prepare(`INSERT INTO entries_initials_fts(rowid, pinyin_initials) VALUES(?, ?)`); err != nil {
		return nil, fmt.Errorf("prepare initials fts insert: %w", err)
	}

	if syncer.deleteBigramStmt, err = prepare(`DELETE FROM entries_bigram WHERE entry_id = ?`); err != nil {
		return nil, fmt.Errorf("prepare bigram delete: %w", err)
	}
	if syncer.insertBigramStmt, err = prepare(`
		INSERT OR IGNORE INTO entries_bigram (field, gram, entry_id)
		VALUES (?, ?, ?)
	`); err != nil {
		return nil, fmt.Errorf("prepare bigram insert: %w", err)
	}

	return syncer, nil
}

func (s *entrySearchArtifactSyncTx) Close() {
	if s == nil {
		return
	}
	for _, stmt := range s.statements {
		stmt.Close()
	}
}

func selectStoredEntriesTx(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]storedEntryRecord, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var loaded []storedEntryRecord
	for rows.Next() {
		row, err := scanStoredEntryRecord(rows)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, row)
	}
	return loaded, rows.Err()
}

func buildEntryScopeQuery(scopePath string, column string) (string, []any) {
	cleanScope := filepath.Clean(scopePath)
	// Escape the scope prefix after appending the platform separator. On Windows
	// a raw trailing "\" would escape the next LIKE token and make stale rows
	// invisible to the SQL-side diff, which leaves deleted files behind.
	scopePrefix := escapeLikePattern(cleanScope+string(filepath.Separator)) + "%"
	return fmt.Sprintf("(%s = ? OR %s LIKE ? ESCAPE '\\')", column, column), []any{cleanScope, scopePrefix}
}

func buildEntryDifferencePredicate(existingAlias string, stagedAlias string) string {
	left := strings.TrimSpace(existingAlias)
	right := strings.TrimSpace(stagedAlias)
	return fmt.Sprintf(
		"%s.root_id <> %s.root_id OR %s.parent_path <> %s.parent_path OR %s.name <> %s.name OR %s.normalized_name <> %s.normalized_name OR %s.name_key <> %s.name_key OR %s.normalized_path <> %s.normalized_path OR %s.pinyin_full <> %s.pinyin_full OR %s.pinyin_initials <> %s.pinyin_initials OR %s.extension <> %s.extension OR %s.is_dir <> %s.is_dir OR %s.mtime <> %s.mtime OR %s.size <> %s.size OR %s.updated_at <> %s.updated_at",
		left, right,
		left, right,
		left, right,
		left, right,
		left, right,
		left, right,
		left, right,
		left, right,
		left, right,
		left, right,
		left, right,
		left, right,
		left, right,
	)
}

func refreshRootBigramsTx(ctx context.Context, tx *sql.Tx, rootID string) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM entries_bigram
		WHERE entry_id IN (SELECT entry_id FROM entries WHERE root_id = ?)
	`, rootID); err != nil {
		return fmt.Errorf("clear root bigrams for %s: %w", rootID, err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT entry_id, path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
		       pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
		FROM entries
		WHERE root_id = ?
	`, rootID)
	if err != nil {
		return fmt.Errorf("load root entries for bigram refresh: %w", err)
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO entries_bigram (field, gram, entry_id)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare root bigram refresh insert: %w", err)
	}
	defer stmt.Close()

	for rows.Next() {
		row, err := scanStoredEntryRecord(rows)
		if err != nil {
			return err
		}
		if err := insertEntryBigramsTx(ctx, stmt, row); err != nil {
			return err
		}
	}
	return rows.Err()
}

func deleteEntrySearchArtifactsTx(ctx context.Context, tx *sql.Tx, row storedEntryRecord) error {
	syncer, err := newEntrySearchArtifactSyncTx(ctx, tx)
	if err != nil {
		return err
	}
	defer syncer.Close()
	return deleteEntrySearchArtifactsWithSyncTx(ctx, syncer, row)
}

func deleteEntrySearchArtifactsWithSyncTx(ctx context.Context, syncer *entrySearchArtifactSyncTx, row storedEntryRecord) error {
	if row.EntryID == 0 {
		return nil
	}
	if err := deleteEntryFTSWithSyncTx(ctx, syncer, row); err != nil {
		return err
	}
	if _, err := syncer.deleteBigramStmt.ExecContext(ctx, row.EntryID); err != nil {
		return fmt.Errorf("delete entry bigrams for %q: %w", row.Path, err)
	}
	return nil
}

func insertEntrySearchArtifactsTx(ctx context.Context, tx *sql.Tx, row storedEntryRecord) error {
	syncer, err := newEntrySearchArtifactSyncTx(ctx, tx)
	if err != nil {
		return err
	}
	defer syncer.Close()
	return insertEntrySearchArtifactsWithSyncTx(ctx, syncer, row)
}

func insertEntrySearchArtifactsWithSyncTx(ctx context.Context, syncer *entrySearchArtifactSyncTx, row storedEntryRecord) error {
	if row.EntryID == 0 {
		return nil
	}
	if err := insertEntryFTSWithSyncTx(ctx, syncer, row); err != nil {
		return err
	}
	return insertEntryBigramsTx(ctx, syncer.insertBigramStmt, row)
}

func deleteEntryFTSTx(ctx context.Context, tx *sql.Tx, row storedEntryRecord) error {
	syncer, err := newEntrySearchArtifactSyncTx(ctx, tx)
	if err != nil {
		return err
	}
	defer syncer.Close()
	return deleteEntryFTSWithSyncTx(ctx, syncer, row)
}

func deleteEntryFTSWithSyncTx(ctx context.Context, syncer *entrySearchArtifactSyncTx, row storedEntryRecord) error {
	commands := []struct {
		name  string
		stmt  *sql.Stmt
		value string
	}{
		{name: "entries_name_fts", stmt: syncer.deleteNameFTSStmt, value: row.NormalizedName},
		{name: "entries_path_fts", stmt: syncer.deletePathFTSStmt, value: row.NormalizedPath},
		{name: "entries_pinyin_full_fts", stmt: syncer.deletePinyinFullFTSStmt, value: row.PinyinFull},
		{name: "entries_initials_fts", stmt: syncer.deleteInitialsFTSStmt, value: row.PinyinInitials},
	}
	for _, command := range commands {
		if strings.TrimSpace(command.value) == "" {
			continue
		}
		if _, err := command.stmt.ExecContext(ctx, row.EntryID, command.value); err != nil {
			return fmt.Errorf("delete %s row for %q: %w", command.name, row.Path, err)
		}
	}
	return nil
}

func insertEntryFTSTx(ctx context.Context, tx *sql.Tx, row storedEntryRecord) error {
	syncer, err := newEntrySearchArtifactSyncTx(ctx, tx)
	if err != nil {
		return err
	}
	defer syncer.Close()
	return insertEntryFTSWithSyncTx(ctx, syncer, row)
}

func insertEntryFTSWithSyncTx(ctx context.Context, syncer *entrySearchArtifactSyncTx, row storedEntryRecord) error {
	commands := []struct {
		name  string
		stmt  *sql.Stmt
		value string
	}{
		{name: "entries_name_fts", stmt: syncer.insertNameFTSStmt, value: row.NormalizedName},
		{name: "entries_path_fts", stmt: syncer.insertPathFTSStmt, value: row.NormalizedPath},
		{name: "entries_pinyin_full_fts", stmt: syncer.insertPinyinFullFTSStmt, value: row.PinyinFull},
		{name: "entries_initials_fts", stmt: syncer.insertInitialsFTSStmt, value: row.PinyinInitials},
	}
	for _, command := range commands {
		if strings.TrimSpace(command.value) == "" {
			continue
		}
		if _, err := command.stmt.ExecContext(ctx, row.EntryID, command.value); err != nil {
			return fmt.Errorf("insert %s row for %q: %w", command.name, row.Path, err)
		}
	}
	return nil
}

func (d *FileSearchDB) rebuildFTSTables(ctx context.Context, optimize bool) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := rebuildFTSTablesTx(ctx, tx, optimize); err != nil {
		return err
	}

	return tx.Commit()
}

func rebuildFTSTablesTx(ctx context.Context, tx *sql.Tx, optimize bool) error {
	for _, tableName := range filesearchFTSTables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s(%s) VALUES('rebuild')`, tableName, tableName)); err != nil {
			return fmt.Errorf("rebuild %s: %w", tableName, err)
		}
	}
	if optimize {
		for _, tableName := range filesearchFTSTables {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s(%s) VALUES('optimize')`, tableName, tableName)); err != nil {
				return fmt.Errorf("optimize %s: %w", tableName, err)
			}
		}
	}
	return nil
}
