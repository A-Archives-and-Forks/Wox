package filesearch

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestFileSearchDBInitCreatesDirectoriesTable(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)

	row := db.db.QueryRowContext(ctx, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = 'directories'
	`)

	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatalf("expected directories table to exist: %v", err)
	}
}

func TestFileSearchDBInitExtendsRootsTableWithFeedColumns(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)

	rows, err := db.db.QueryContext(ctx, `PRAGMA table_info(roots)`)
	if err != nil {
		t.Fatalf("query roots schema: %v", err)
	}
	defer rows.Close()

	columnNames := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan roots schema: %v", err)
		}
		columnNames[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate roots schema: %v", err)
	}

	requiredColumns := []string{
		"feed_type",
		"feed_cursor",
		"feed_state",
		"last_reconcile_at",
		"last_full_scan_at",
	}
	for _, columnName := range requiredColumns {
		if !columnNames[columnName] {
			t.Fatalf("expected roots column %q to exist", columnName)
		}
	}
}

func TestFileSearchDBInitCreatesSQLiteSearchSchema(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)

	rows, err := db.db.QueryContext(ctx, `PRAGMA table_info(entries)`)
	if err != nil {
		t.Fatalf("query entries schema: %v", err)
	}
	defer rows.Close()

	columnNames := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan entries schema: %v", err)
		}
		columnNames[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate entries schema: %v", err)
	}

	for _, columnName := range []string{"entry_id", "name_key", "extension"} {
		if !columnNames[columnName] {
			t.Fatalf("expected entries column %q to exist", columnName)
		}
	}

	for _, tableName := range []string{
		"entries_bigram",
		"entries_name_fts",
		"entries_path_fts",
		"entries_pinyin_full_fts",
		"entries_initials_fts",
	} {
		row := db.db.QueryRowContext(ctx, `
			SELECT name
			FROM sqlite_master
			WHERE name = ?
		`, tableName)

		var found string
		if err := row.Scan(&found); err != nil {
			t.Fatalf("expected sqlite search table %q to exist: %v", tableName, err)
		}
	}
}

func TestFileSearchDBDeleteRootRemovesDirectorySnapshots(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "root-delete",
		Path:      filepath.Join(t.TempDir(), "root-delete"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, "/tmp/root", root.ID, "/", root.UpdatedAt, true)
	if err != nil {
		t.Fatalf("insert directory snapshot: %v", err)
	}

	if err := db.DeleteRoot(ctx, root.ID); err != nil {
		t.Fatalf("delete root: %v", err)
	}

	row := db.db.QueryRowContext(ctx, `SELECT count(*) FROM directories WHERE root_id = ?`, root.ID)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count directory snapshots: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected directory snapshots to be deleted, got %d", count)
	}
}

func TestFileSearchDBReplaceRootSnapshotPreservesEntryIDForExistingPath(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootPath := filepath.Join(t.TempDir(), "root-entry-id-stability")
	root := RootRecord{
		ID:        "root-entry-id-stability",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	entryPath := filepath.Join(rootPath, "report.txt")
	entry := EntryRecord{
		Path:           entryPath,
		RootID:         root.ID,
		ParentPath:     rootPath,
		Name:           "report.txt",
		NormalizedName: "report.txt",
		NormalizedPath: entryPath,
		IsDir:          false,
		Mtime:          now,
		Size:           10,
		UpdatedAt:      now,
	}

	if err := db.ReplaceRootEntries(ctx, root, []EntryRecord{entry}, nil); err != nil {
		t.Fatalf("initial replace root entries: %v", err)
	}
	firstEntryID := queryEntryIDByPath(t, db, ctx, entryPath)

	entry.Size = 20
	entry.UpdatedAt = now + 1
	if err := db.ReplaceRootEntries(ctx, root, []EntryRecord{entry}, nil); err != nil {
		t.Fatalf("second replace root entries: %v", err)
	}
	secondEntryID := queryEntryIDByPath(t, db, ctx, entryPath)

	if firstEntryID != secondEntryID {
		t.Fatalf("expected entry_id to stay stable across upsert, got %d then %d", firstEntryID, secondEntryID)
	}
}

func TestFileSearchDBListDirectoriesByRootStartsEmpty(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "root-empty",
		Path:      filepath.Join(t.TempDir(), "root-empty"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	directories, err := db.ListDirectoriesByRoot(ctx, root.ID)
	if err != nil {
		t.Fatalf("list directories: %v", err)
	}
	if len(directories) != 0 {
		t.Fatalf("expected no directories, got %d", len(directories))
	}
}

func TestFileSearchDBListDirectoriesByRootRoundTripsRows(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "root-roundtrip",
		Path:      filepath.Join(t.TempDir(), "root-roundtrip"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	otherRoot := RootRecord{
		ID:        "root-other",
		Path:      filepath.Join(t.TempDir(), "root-other"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, otherRoot)

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?),
		       (?, ?, ?, ?, ?),
		       (?, ?, ?, ?, ?)
	`, "/root/b", root.ID, "/root", int64(200), true,
		"/root/a", root.ID, "/root", int64(100), false,
		"/other/z", otherRoot.ID, "/other", int64(300), true,
	)
	if err != nil {
		t.Fatalf("insert directory snapshots: %v", err)
	}

	directories, err := db.ListDirectoriesByRoot(ctx, root.ID)
	if err != nil {
		t.Fatalf("list directories: %v", err)
	}
	if len(directories) != 2 {
		t.Fatalf("expected 2 directories, got %d", len(directories))
	}

	if directories[0].Path != "/root/a" || directories[1].Path != "/root/b" {
		t.Fatalf("expected path-ascending order, got %q then %q", directories[0].Path, directories[1].Path)
	}
	if directories[0].LastScanTime != 100 || directories[1].LastScanTime != 200 {
		t.Fatalf("expected last scan times to round-trip, got %d and %d", directories[0].LastScanTime, directories[1].LastScanTime)
	}
	if directories[0].Exists != false || directories[1].Exists != true {
		t.Fatalf("expected exists flags to round-trip, got %t and %t", directories[0].Exists, directories[1].Exists)
	}
}

func TestFileSearchDBReplaceSubtreeSnapshotReplacesOnlyScopedPaths(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootBase := filepath.Join(t.TempDir(), "root-3")
	scopePath := filepath.Join(rootBase, "a")
	oldEntryPath := filepath.Join(scopePath, "old.txt")
	newEntryPath := filepath.Join(scopePath, "new.txt")
	siblingEntryPath := filepath.Join(rootBase, "b", "keep.txt")
	adjacentDirectoryPath := filepath.Join(rootBase, "ab")
	outsideDirectoryPath := filepath.Join(rootBase, "c")
	root := RootRecord{
		ID:        "root-subtree",
		Path:      rootBase,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db,
		EntryRecord{
			Path:           oldEntryPath,
			RootID:         root.ID,
			ParentPath:     scopePath,
			Name:           "old.txt",
			NormalizedName: "old.txt",
			NormalizedPath: "old.txt",
			IsDir:          false,
			Mtime:          int64(10),
			Size:           int64(1),
			UpdatedAt:      now,
		},
		EntryRecord{
			Path:           siblingEntryPath,
			RootID:         root.ID,
			ParentPath:     filepath.Join(rootBase, "b"),
			Name:           "keep.txt",
			NormalizedName: "keep.txt",
			NormalizedPath: "keep.txt",
			IsDir:          false,
			Mtime:          int64(20),
			Size:           int64(2),
			UpdatedAt:      now,
		},
	)

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?),
		       (?, ?, ?, ?, ?),
		       (?, ?, ?, ?, ?)
	`, adjacentDirectoryPath, root.ID, rootBase, now, true,
		outsideDirectoryPath, root.ID, rootBase, now, true,
		scopePath, root.ID, rootBase, now, false,
	)
	if err != nil {
		t.Fatalf("insert directory snapshots: %v", err)
	}

	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: scopePath,
		Directories: []DirectoryRecord{
			{
				Path:         scopePath,
				RootID:       root.ID,
				ParentPath:   rootBase,
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           newEntryPath,
				RootID:         root.ID,
				ParentPath:     scopePath,
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				PinyinFull:     "",
				PinyinInitials: "",
				IsDir:          false,
				Mtime:          int64(30),
				Size:           int64(3),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err != nil {
		t.Fatalf("replace subtree snapshot: %v", err)
	}

	entries, err := db.ListEntries(ctx)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}

	seen := map[string]EntryRecord{}
	for _, entry := range entries {
		if entry.RootID == root.ID {
			seen[entry.Path] = entry
		}
	}

	if _, ok := seen[oldEntryPath]; ok {
		t.Fatalf("expected scoped entry to be removed")
	}
	if _, ok := seen[newEntryPath]; !ok {
		t.Fatalf("expected new scoped entry to exist")
	}
	if _, ok := seen[siblingEntryPath]; !ok {
		t.Fatalf("expected sibling entry to remain")
	}

	directories, err := db.ListDirectoriesByRoot(ctx, root.ID)
	if err != nil {
		t.Fatalf("list directories: %v", err)
	}
	if len(directories) != 3 {
		t.Fatalf("expected three directories, got %d", len(directories))
	}

	directorySeen := map[string]bool{}
	for _, directory := range directories {
		directorySeen[directory.Path] = directory.Exists
	}

	if !directorySeen[scopePath] {
		t.Fatalf("expected scoped directory to be replaced")
	}
	if !directorySeen[adjacentDirectoryPath] {
		t.Fatalf("expected prefix-adjacent sibling directory to remain")
	}
	if !directorySeen[outsideDirectoryPath] {
		t.Fatalf("expected outside-scope directory to remain")
	}
}

func TestFileSearchDBReplaceSubtreeSnapshotsIsAtomicAcrossBatches(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootBase := filepath.Join(t.TempDir(), "root-atomic")
	scopeA := filepath.Join(rootBase, "a")
	scopeB := filepath.Join(rootBase, "b")
	oldA := filepath.Join(scopeA, "old-a.txt")
	oldB := filepath.Join(scopeB, "old-b.txt")
	newA := filepath.Join(scopeA, "new-a.txt")

	root := RootRecord{
		ID:        "root-atomic",
		Path:      rootBase,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db,
		EntryRecord{
			Path:           oldA,
			RootID:         root.ID,
			ParentPath:     scopeA,
			Name:           "old-a.txt",
			NormalizedName: "old-a.txt",
			NormalizedPath: "old-a.txt",
			IsDir:          false,
			Mtime:          int64(1),
			Size:           int64(1),
			UpdatedAt:      now,
		},
		EntryRecord{
			Path:           oldB,
			RootID:         root.ID,
			ParentPath:     scopeB,
			Name:           "old-b.txt",
			NormalizedName: "old-b.txt",
			NormalizedPath: "old-b.txt",
			IsDir:          false,
			Mtime:          int64(1),
			Size:           int64(1),
			UpdatedAt:      now,
		},
	)

	err := db.ReplaceSubtreeSnapshots(ctx, []SubtreeSnapshotBatch{
		{
			RootID:    root.ID,
			ScopePath: scopeA,
			Directories: []DirectoryRecord{{
				Path:         scopeA,
				RootID:       root.ID,
				ParentPath:   rootBase,
				LastScanTime: now,
				Exists:       true,
			}},
			Entries: []EntryRecord{{
				Path:           newA,
				RootID:         root.ID,
				ParentPath:     scopeA,
				Name:           "new-a.txt",
				NormalizedName: "new-a.txt",
				NormalizedPath: "new-a.txt",
				IsDir:          false,
				Mtime:          now,
				Size:           2,
				UpdatedAt:      now,
			}},
		},
		{
			RootID:    root.ID,
			ScopePath: filepath.Join(t.TempDir(), "outside-root"),
		},
	})
	if err == nil {
		t.Fatalf("expected multi-batch replace to fail for out-of-root scope")
	}

	entries, err := db.ListEntries(ctx)
	if err != nil {
		t.Fatalf("list entries after failed atomic replace: %v", err)
	}
	paths := []string{entries[0].Path, entries[1].Path}
	if !reflect.DeepEqual(paths, []string{oldA, oldB}) && !reflect.DeepEqual(paths, []string{oldB, oldA}) {
		t.Fatalf("expected failed atomic replace to leave old entries intact, got %#v", paths)
	}
}

func TestFileSearchDBReplaceSubtreeSnapshotsTombstonesMissingDirectories(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootBase := filepath.Join(t.TempDir(), "root-tombstone")
	scopePath := filepath.Join(rootBase, "scope")
	removedPath := filepath.Join(scopePath, "removed")
	keptPath := filepath.Join(scopePath, "kept")

	root := RootRecord{
		ID:        "root-tombstone",
		Path:      rootBase,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?),
		       (?, ?, ?, ?, ?)
	`, removedPath, root.ID, scopePath, now, true,
		keptPath, root.ID, scopePath, now, true,
	)
	if err != nil {
		t.Fatalf("seed tombstone directories: %v", err)
	}

	if err := db.ReplaceSubtreeSnapshots(ctx, []SubtreeSnapshotBatch{{
		RootID:    root.ID,
		ScopePath: scopePath,
		Directories: []DirectoryRecord{
			{
				Path:         scopePath,
				RootID:       root.ID,
				ParentPath:   rootBase,
				LastScanTime: now,
				Exists:       true,
			},
			{
				Path:         keptPath,
				RootID:       root.ID,
				ParentPath:   scopePath,
				LastScanTime: now,
				Exists:       true,
			},
		},
	}}); err != nil {
		t.Fatalf("replace subtree snapshots with tombstone cleanup: %v", err)
	}

	directories, err := db.ListDirectoriesByRoot(ctx, root.ID)
	if err != nil {
		t.Fatalf("list directories after tombstone replace: %v", err)
	}

	foundRemoved := false
	for _, directory := range directories {
		if directory.Path != removedPath {
			continue
		}
		foundRemoved = true
		if directory.Exists {
			t.Fatalf("expected missing directory %q to be tombstoned", removedPath)
		}
	}
	if !foundRemoved {
		t.Fatalf("expected missing directory %q to remain as tombstone", removedPath)
	}
}

func TestFileSearchDBDeleteDirectoryTombstonesRemovesExistsFalseRows(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootBase := filepath.Join(t.TempDir(), "root-4")
	livePath := filepath.Join(rootBase, "live")
	tombstonePath := filepath.Join(rootBase, "tombstone")
	otherRootBase := filepath.Join(t.TempDir(), "root-4-other")
	otherTombstonePath := filepath.Join(otherRootBase, "tombstone")
	root := RootRecord{
		ID:        "root-tombstone",
		Path:      filepath.Join(t.TempDir(), "root-tombstone"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?),
		       (?, ?, ?, ?, ?)
	`, livePath, root.ID, rootBase, now, true,
		tombstonePath, root.ID, rootBase, now, false,
	)
	if err != nil {
		t.Fatalf("insert directory snapshots: %v", err)
	}

	otherRoot := RootRecord{
		ID:        "root-tombstone-other",
		Path:      filepath.Join(t.TempDir(), "root-tombstone-other"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, otherRoot)

	_, err = db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, otherTombstonePath, otherRoot.ID, otherRootBase, now, false)
	if err != nil {
		t.Fatalf("insert other root directory snapshot: %v", err)
	}

	if err := db.DeleteDirectoryTombstones(ctx, root.ID); err != nil {
		t.Fatalf("delete directory tombstones: %v", err)
	}

	directories, err := db.ListDirectoriesByRoot(ctx, root.ID)
	if err != nil {
		t.Fatalf("list directories: %v", err)
	}
	if len(directories) != 1 {
		t.Fatalf("expected one live directory, got %d", len(directories))
	}
	if directories[0].Path != livePath {
		t.Fatalf("expected live directory to remain, got %q", directories[0].Path)
	}
	if !directories[0].Exists {
		t.Fatalf("expected remaining directory to be live")
	}

	otherDirectories, err := db.ListDirectoriesByRoot(ctx, otherRoot.ID)
	if err != nil {
		t.Fatalf("list other root directories: %v", err)
	}
	if len(otherDirectories) != 1 {
		t.Fatalf("expected other root tombstone to remain, got %d", len(otherDirectories))
	}
	if otherDirectories[0].Path != otherTombstonePath || otherDirectories[0].Exists {
		t.Fatalf("expected other root tombstone row to remain unchanged")
	}
}

func TestFileSearchDBReplaceSubtreeSnapshotRejectsMismatchedRootIDAndLeavesDBUnchanged(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootBase := filepath.Join(t.TempDir(), "root-validation-mismatch")
	scopePath := filepath.Join(rootBase, "scope")
	existingEntryPath := filepath.Join(scopePath, "existing.txt")
	existingDirectoryPath := filepath.Join(scopePath, "existing-dir")
	root := RootRecord{
		ID:        "root-validation-mismatch",
		Path:      filepath.Join(t.TempDir(), "root-validation-mismatch"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db, EntryRecord{
		Path:           existingEntryPath,
		RootID:         root.ID,
		ParentPath:     scopePath,
		Name:           "existing.txt",
		NormalizedName: "existing.txt",
		NormalizedPath: "existing.txt",
		IsDir:          false,
		Mtime:          int64(10),
		Size:           int64(1),
		UpdatedAt:      now,
	})

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, existingDirectoryPath, root.ID, scopePath, now, true)
	if err != nil {
		t.Fatalf("insert existing directory snapshot: %v", err)
	}

	beforeEntries, beforeDirectories := snapshotRootState(t, db, ctx, root.ID)

	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: scopePath,
		Directories: []DirectoryRecord{
			{
				Path:         filepath.Join(scopePath, "new-dir"),
				RootID:       "different-root",
				ParentPath:   scopePath,
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           filepath.Join(scopePath, "new.txt"),
				RootID:         root.ID,
				ParentPath:     scopePath,
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				IsDir:          false,
				Mtime:          int64(20),
				Size:           int64(2),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err == nil {
		t.Fatalf("expected mismatched root id to be rejected")
	}

	afterEntries, afterDirectories := snapshotRootState(t, db, ctx, root.ID)
	assertRootStateEqual(t, beforeEntries, afterEntries, beforeDirectories, afterDirectories)
}

func TestFileSearchDBReplaceSubtreeSnapshotRejectsOutOfScopePathAndLeavesDBUnchanged(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootBase := filepath.Join(t.TempDir(), "root-validation-out-of-scope")
	scopePath := filepath.Join(rootBase, "scope")
	existingEntryPath := filepath.Join(scopePath, "existing.txt")
	existingDirectoryPath := filepath.Join(scopePath, "existing-dir")
	root := RootRecord{
		ID:        "root-validation-out-of-scope",
		Path:      filepath.Join(t.TempDir(), "root-validation-out-of-scope"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db, EntryRecord{
		Path:           existingEntryPath,
		RootID:         root.ID,
		ParentPath:     scopePath,
		Name:           "existing.txt",
		NormalizedName: "existing.txt",
		NormalizedPath: "existing.txt",
		IsDir:          false,
		Mtime:          int64(10),
		Size:           int64(1),
		UpdatedAt:      now,
	})

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, existingDirectoryPath, root.ID, scopePath, now, true)
	if err != nil {
		t.Fatalf("insert existing directory snapshot: %v", err)
	}

	beforeEntries, beforeDirectories := snapshotRootState(t, db, ctx, root.ID)

	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: scopePath,
		Directories: []DirectoryRecord{
			{
				Path:         filepath.Join(rootBase, "other", "new-dir"),
				RootID:       root.ID,
				ParentPath:   filepath.Join(rootBase, "other"),
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           filepath.Join(scopePath, "new.txt"),
				RootID:         root.ID,
				ParentPath:     scopePath,
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				IsDir:          false,
				Mtime:          int64(20),
				Size:           int64(2),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err == nil {
		t.Fatalf("expected out-of-scope path to be rejected")
	}

	afterEntries, afterDirectories := snapshotRootState(t, db, ctx, root.ID)
	assertRootStateEqual(t, beforeEntries, afterEntries, beforeDirectories, afterDirectories)
}

func TestFileSearchDBReplaceSubtreeSnapshotRejectsEmptyScopePathAndLeavesDBUnchanged(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "root-validation-empty-scope",
		Path:      filepath.Join(t.TempDir(), "root-validation-empty-scope"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db, EntryRecord{
		Path:           filepath.Join(root.Path, "existing.txt"),
		RootID:         root.ID,
		ParentPath:     root.Path,
		Name:           "existing.txt",
		NormalizedName: "existing.txt",
		NormalizedPath: "existing.txt",
		IsDir:          false,
		Mtime:          int64(10),
		Size:           int64(1),
		UpdatedAt:      now,
	})

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, filepath.Join(root.Path, "existing-dir"), root.ID, root.Path, now, true)
	if err != nil {
		t.Fatalf("insert existing directory snapshot: %v", err)
	}

	beforeEntries, beforeDirectories := snapshotRootState(t, db, ctx, root.ID)

	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: "",
		Directories: []DirectoryRecord{
			{
				Path:         filepath.Join(root.Path, "new-dir"),
				RootID:       root.ID,
				ParentPath:   root.Path,
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           filepath.Join(root.Path, "new.txt"),
				RootID:         root.ID,
				ParentPath:     root.Path,
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				IsDir:          false,
				Mtime:          int64(20),
				Size:           int64(2),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err == nil {
		t.Fatalf("expected empty scope path to be rejected")
	}

	afterEntries, afterDirectories := snapshotRootState(t, db, ctx, root.ID)
	assertRootStateEqual(t, beforeEntries, afterEntries, beforeDirectories, afterDirectories)
}

func TestFileSearchDBReplaceSubtreeSnapshotRejectsMismatchedEntryRootIDAndLeavesDBUnchanged(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	rootBase := filepath.Join(t.TempDir(), "root-validation-entry-root")
	scopePath := filepath.Join(rootBase, "scope")
	existingEntryPath := filepath.Join(scopePath, "existing.txt")
	existingDirectoryPath := filepath.Join(scopePath, "existing-dir")
	root := RootRecord{
		ID:        "root-validation-entry-root",
		Path:      filepath.Join(t.TempDir(), "root-validation-entry-root"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db, EntryRecord{
		Path:           existingEntryPath,
		RootID:         root.ID,
		ParentPath:     scopePath,
		Name:           "existing.txt",
		NormalizedName: "existing.txt",
		NormalizedPath: "existing.txt",
		IsDir:          false,
		Mtime:          int64(10),
		Size:           int64(1),
		UpdatedAt:      now,
	})

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, existingDirectoryPath, root.ID, scopePath, now, true)
	if err != nil {
		t.Fatalf("insert existing directory snapshot: %v", err)
	}

	beforeEntries, beforeDirectories := snapshotRootState(t, db, ctx, root.ID)

	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: scopePath,
		Directories: []DirectoryRecord{
			{
				Path:         filepath.Join(scopePath, "new-dir"),
				RootID:       root.ID,
				ParentPath:   scopePath,
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           filepath.Join(scopePath, "new.txt"),
				RootID:         "different-root",
				ParentPath:     scopePath,
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				IsDir:          false,
				Mtime:          int64(20),
				Size:           int64(2),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err == nil {
		t.Fatalf("expected mismatched entry root id to be rejected")
	}

	afterEntries, afterDirectories := snapshotRootState(t, db, ctx, root.ID)
	assertRootStateEqual(t, beforeEntries, afterEntries, beforeDirectories, afterDirectories)
}

func TestFileSearchDBReplaceSubtreeSnapshotRejectsRelativeScopePathAndLeavesDBUnchanged(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "root-validation-relative-scope",
		Path:      filepath.Join(t.TempDir(), "root-validation-relative-scope"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db, EntryRecord{
		Path:           filepath.Join(root.Path, "existing.txt"),
		RootID:         root.ID,
		ParentPath:     root.Path,
		Name:           "existing.txt",
		NormalizedName: "existing.txt",
		NormalizedPath: "existing.txt",
		IsDir:          false,
		Mtime:          int64(10),
		Size:           int64(1),
		UpdatedAt:      now,
	})

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, filepath.Join(root.Path, "existing-dir"), root.ID, root.Path, now, true)
	if err != nil {
		t.Fatalf("insert existing directory snapshot: %v", err)
	}

	beforeEntries, beforeDirectories := snapshotRootState(t, db, ctx, root.ID)

	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: filepath.Join("relative", "scope"),
		Directories: []DirectoryRecord{
			{
				Path:         filepath.Join(root.Path, "new-dir"),
				RootID:       root.ID,
				ParentPath:   root.Path,
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           filepath.Join(root.Path, "new.txt"),
				RootID:         root.ID,
				ParentPath:     root.Path,
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				IsDir:          false,
				Mtime:          int64(20),
				Size:           int64(2),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err == nil {
		t.Fatalf("expected relative scope path to be rejected")
	}

	afterEntries, afterDirectories := snapshotRootState(t, db, ctx, root.ID)
	assertRootStateEqual(t, beforeEntries, afterEntries, beforeDirectories, afterDirectories)
}

func TestFileSearchDBReplaceSubtreeSnapshotRejectsMismatchedDirectoryParentPathAndLeavesDBUnchanged(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "root-validation-directory-parent",
		Path:      filepath.Join(t.TempDir(), "root-validation-directory-parent"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db, EntryRecord{
		Path:           filepath.Join(root.Path, "existing.txt"),
		RootID:         root.ID,
		ParentPath:     root.Path,
		Name:           "existing.txt",
		NormalizedName: "existing.txt",
		NormalizedPath: "existing.txt",
		IsDir:          false,
		Mtime:          int64(10),
		Size:           int64(1),
		UpdatedAt:      now,
	})

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, filepath.Join(root.Path, "existing-dir"), root.ID, root.Path, now, true)
	if err != nil {
		t.Fatalf("insert existing directory snapshot: %v", err)
	}

	beforeEntries, beforeDirectories := snapshotRootState(t, db, ctx, root.ID)

	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: root.Path,
		Directories: []DirectoryRecord{
			{
				Path:         filepath.Join(root.Path, "new-dir"),
				RootID:       root.ID,
				ParentPath:   filepath.Join(root.Path, "wrong-parent"),
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           filepath.Join(root.Path, "new.txt"),
				RootID:         root.ID,
				ParentPath:     root.Path,
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				IsDir:          false,
				Mtime:          int64(20),
				Size:           int64(2),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err == nil {
		t.Fatalf("expected mismatched directory parent path to be rejected")
	}

	afterEntries, afterDirectories := snapshotRootState(t, db, ctx, root.ID)
	assertRootStateEqual(t, beforeEntries, afterEntries, beforeDirectories, afterDirectories)
}

func TestFileSearchDBReplaceSubtreeSnapshotRejectsMismatchedEntryParentPathAndLeavesDBUnchanged(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "root-validation-entry-parent",
		Path:      filepath.Join(t.TempDir(), "root-validation-entry-parent"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db, EntryRecord{
		Path:           filepath.Join(root.Path, "existing.txt"),
		RootID:         root.ID,
		ParentPath:     root.Path,
		Name:           "existing.txt",
		NormalizedName: "existing.txt",
		NormalizedPath: "existing.txt",
		IsDir:          false,
		Mtime:          int64(10),
		Size:           int64(1),
		UpdatedAt:      now,
	})

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, filepath.Join(root.Path, "existing-dir"), root.ID, root.Path, now, true)
	if err != nil {
		t.Fatalf("insert existing directory snapshot: %v", err)
	}

	beforeEntries, beforeDirectories := snapshotRootState(t, db, ctx, root.ID)

	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: root.Path,
		Directories: []DirectoryRecord{
			{
				Path:         filepath.Join(root.Path, "new-dir"),
				RootID:       root.ID,
				ParentPath:   root.Path,
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           filepath.Join(root.Path, "new.txt"),
				RootID:         root.ID,
				ParentPath:     filepath.Join(root.Path, "wrong-parent"),
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				IsDir:          false,
				Mtime:          int64(20),
				Size:           int64(2),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err == nil {
		t.Fatalf("expected mismatched entry parent path to be rejected")
	}

	afterEntries, afterDirectories := snapshotRootState(t, db, ctx, root.ID)
	assertRootStateEqual(t, beforeEntries, afterEntries, beforeDirectories, afterDirectories)
}

func TestFileSearchDBReplaceSubtreeSnapshotRejectsRootScopeMismatchAndLeavesDBUnchanged(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "root-validation-root-scope",
		Path:      filepath.Join(t.TempDir(), "root-validation-root-scope"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	otherRoot := RootRecord{
		ID:        "root-validation-root-scope-other",
		Path:      filepath.Join(t.TempDir(), "root-validation-root-scope-other"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, otherRoot)

	mustInsertEntrySnapshots(t, ctx, db, EntryRecord{
		Path:           filepath.Join(root.Path, "existing.txt"),
		RootID:         root.ID,
		ParentPath:     root.Path,
		Name:           "existing.txt",
		NormalizedName: "existing.txt",
		NormalizedPath: "existing.txt",
		IsDir:          false,
		Mtime:          int64(10),
		Size:           int64(1),
		UpdatedAt:      now,
	})

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, filepath.Join(root.Path, "existing-dir"), root.ID, root.Path, now, true)
	if err != nil {
		t.Fatalf("insert existing directory snapshot: %v", err)
	}

	beforeEntries, beforeDirectories := snapshotRootState(t, db, ctx, root.ID)

	batch := SubtreeSnapshotBatch{
		RootID:    root.ID,
		ScopePath: otherRoot.Path,
		Directories: []DirectoryRecord{
			{
				Path:         filepath.Join(otherRoot.Path, "new-dir"),
				RootID:       root.ID,
				ParentPath:   otherRoot.Path,
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           filepath.Join(otherRoot.Path, "new.txt"),
				RootID:         root.ID,
				ParentPath:     otherRoot.Path,
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				IsDir:          false,
				Mtime:          int64(20),
				Size:           int64(2),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err == nil {
		t.Fatalf("expected root/scope mismatch to be rejected")
	}

	afterEntries, afterDirectories := snapshotRootState(t, db, ctx, root.ID)
	assertRootStateEqual(t, beforeEntries, afterEntries, beforeDirectories, afterDirectories)
}

func TestFileSearchDBReplaceSubtreeSnapshotRejectsMissingRootAndLeavesDBUnchanged(t *testing.T) {
	db, ctx := openTestFileSearchDB(t)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "root-validation-missing-root",
		Path:      filepath.Join(t.TempDir(), "root-validation-missing-root"),
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, ctx, db, root)

	mustInsertEntrySnapshots(t, ctx, db, EntryRecord{
		Path:           filepath.Join(root.Path, "existing.txt"),
		RootID:         root.ID,
		ParentPath:     root.Path,
		Name:           "existing.txt",
		NormalizedName: "existing.txt",
		NormalizedPath: "existing.txt",
		IsDir:          false,
		Mtime:          int64(10),
		Size:           int64(1),
		UpdatedAt:      now,
	})

	_, err := db.db.ExecContext(ctx, `
		INSERT INTO directories (path, root_id, parent_path, last_scan_time, "exists")
		VALUES (?, ?, ?, ?, ?)
	`, filepath.Join(root.Path, "existing-dir"), root.ID, root.Path, now, true)
	if err != nil {
		t.Fatalf("insert existing directory snapshot: %v", err)
	}

	beforeEntries, beforeDirectories := snapshotRootState(t, db, ctx, root.ID)

	batch := SubtreeSnapshotBatch{
		RootID:    "missing-root",
		ScopePath: filepath.Join(root.Path, "scope"),
		Directories: []DirectoryRecord{
			{
				Path:         filepath.Join(root.Path, "scope", "new-dir"),
				RootID:       "missing-root",
				ParentPath:   filepath.Join(root.Path, "scope"),
				LastScanTime: now,
				Exists:       true,
			},
		},
		Entries: []EntryRecord{
			{
				Path:           filepath.Join(root.Path, "scope", "new.txt"),
				RootID:         "missing-root",
				ParentPath:     filepath.Join(root.Path, "scope"),
				Name:           "new.txt",
				NormalizedName: "new.txt",
				NormalizedPath: "new.txt",
				IsDir:          false,
				Mtime:          int64(20),
				Size:           int64(2),
				UpdatedAt:      now,
			},
		},
	}

	if err := db.ReplaceSubtreeSnapshot(ctx, batch); err == nil {
		t.Fatalf("expected missing root to be rejected")
	}

	afterEntries, afterDirectories := snapshotRootState(t, db, ctx, root.ID)
	assertRootStateEqual(t, beforeEntries, afterEntries, beforeDirectories, afterDirectories)
}

func snapshotRootState(t *testing.T, db *FileSearchDB, ctx context.Context, rootID string) (map[string]EntryRecord, map[string]DirectoryRecord) {
	t.Helper()

	entries, err := db.ListEntries(ctx)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}

	entryState := map[string]EntryRecord{}
	for _, entry := range entries {
		if entry.RootID == rootID {
			entryState[entry.Path] = entry
		}
	}

	directories, err := db.ListDirectoriesByRoot(ctx, rootID)
	if err != nil {
		t.Fatalf("list directories: %v", err)
	}

	directoryState := map[string]DirectoryRecord{}
	for _, directory := range directories {
		directoryState[directory.Path] = directory
	}

	return entryState, directoryState
}

func assertRootStateEqual(
	t *testing.T,
	beforeEntries map[string]EntryRecord,
	afterEntries map[string]EntryRecord,
	beforeDirectories map[string]DirectoryRecord,
	afterDirectories map[string]DirectoryRecord,
) {
	t.Helper()

	if !reflect.DeepEqual(beforeEntries, afterEntries) {
		t.Fatalf("expected entries to remain unchanged\nbefore: %#v\nafter: %#v", beforeEntries, afterEntries)
	}
	if !reflect.DeepEqual(beforeDirectories, afterDirectories) {
		t.Fatalf("expected directories to remain unchanged\nbefore: %#v\nafter: %#v", beforeDirectories, afterDirectories)
	}
}

func queryEntryIDByPath(t *testing.T, db *FileSearchDB, ctx context.Context, entryPath string) int64 {
	t.Helper()

	row := db.db.QueryRowContext(ctx, `SELECT entry_id FROM entries WHERE path = ?`, entryPath)
	var entryID int64
	if err := row.Scan(&entryID); err != nil {
		t.Fatalf("query entry_id for %q: %v", entryPath, err)
	}
	return entryID
}
