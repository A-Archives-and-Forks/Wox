package filesearch

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"wox/util"
)

// util.GetLocation() is process-global, so do not use this helper from parallel benchmarks.
func openBenchmarkFileSearchDB(b *testing.B) (*FileSearchDB, context.Context) {
	b.Helper()

	dataDir := b.TempDir()
	b.Setenv(util.TestWoxDataDirEnv, dataDir)
	b.Setenv(util.TestUserDataDirEnv, filepath.Join(dataDir, "user"))

	if err := util.GetLocation().Init(); err != nil {
		b.Fatalf("init location: %v", err)
	}

	ctx := context.Background()
	db, err := NewFileSearchDB(ctx)
	if err != nil {
		b.Fatalf("open filesearch db: %v", err)
	}

	b.Cleanup(func() {
		_ = db.Close()
	})

	return db, ctx
}

func mustInsertBenchmarkRoot(b *testing.B, ctx context.Context, db *FileSearchDB, root RootRecord) {
	b.Helper()

	if err := db.UpsertRoot(ctx, root); err != nil {
		b.Fatalf("upsert root: %v", err)
	}
}

func benchmarkBatch(rootID, scopePath string, dirCount, fileCount int) SubtreeSnapshotBatch {
	directories := make([]DirectoryRecord, 0, dirCount)
	entries := make([]EntryRecord, 0, fileCount)

	for i := 0; i < dirCount; i++ {
		dirPath := filepath.Join(scopePath, fmt.Sprintf("dir-%04d", i))
		directories = append(directories, DirectoryRecord{
			Path:         dirPath,
			RootID:       rootID,
			ParentPath:   scopePath,
			LastScanTime: int64(i + 1),
			Exists:       true,
		})
	}

	for i := 0; i < fileCount; i++ {
		filePath := filepath.Join(scopePath, fmt.Sprintf("file-%04d.txt", i))
		entries = append(entries, EntryRecord{
			Path:           filePath,
			RootID:         rootID,
			ParentPath:     scopePath,
			Name:           filepath.Base(filePath),
			NormalizedName: filepath.Base(filePath),
			NormalizedPath: filePath,
			IsDir:          false,
			Mtime:          int64(i + 1),
			Size:           128,
			UpdatedAt:      int64(i + 1),
		})
	}

	return SubtreeSnapshotBatch{
		RootID:      rootID,
		ScopePath:   scopePath,
		Directories: directories,
		Entries:     entries,
	}
}

func BenchmarkFileSearchDBReplaceSubtreeSnapshot(b *testing.B) {
	db, ctx := openBenchmarkFileSearchDB(b)
	rootPath := filepath.Join(b.TempDir(), "bench-root")
	root := RootRecord{
		ID:        "bench-root",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: 1,
		UpdatedAt: 1,
	}
	mustInsertBenchmarkRoot(b, ctx, db, root)

	b.ReportAllocs()

	b.Run("small-subtree", func(b *testing.B) {
		batch := benchmarkBatch(root.ID, filepath.Join(rootPath, "small"), 64, 256)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := db.ReplaceSubtreeSnapshot(context.Background(), batch); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("large-subtree", func(b *testing.B) {
		batch := benchmarkBatch(root.ID, filepath.Join(rootPath, "large"), 1024, 4096)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := db.ReplaceSubtreeSnapshot(context.Background(), batch); err != nil {
				b.Fatal(err)
			}
		}
	})
}
