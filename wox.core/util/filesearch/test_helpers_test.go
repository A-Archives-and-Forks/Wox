package filesearch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"wox/util"
)

func openTestFileSearchDB(t *testing.T) (*FileSearchDB, context.Context) {
	t.Helper()

	// This helper mutates process-global location and environment state.
	// Do not call it from tests that use t.Parallel().
	testRoot, err := os.MkdirTemp("", "wox-filesearch-test-")
	if err != nil {
		t.Fatalf("create test root: %v", err)
	}
	t.Setenv(util.TestWoxDataDirEnv, filepath.Join(testRoot, "wox"))
	t.Setenv(util.TestUserDataDirEnv, filepath.Join(testRoot, "user"))
	ctx := context.Background()

	if err := util.GetLocation().Init(); err != nil {
		t.Fatalf("init test location: %v", err)
	}

	db, err := NewFileSearchDB(ctx)
	if err != nil {
		t.Fatalf("open filesearch db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		_ = os.RemoveAll(testRoot)
	})

	return db, ctx
}

func mustInsertRoot(t *testing.T, ctx context.Context, db *FileSearchDB, root RootRecord) {
	t.Helper()

	if err := db.UpsertRoot(ctx, root); err != nil {
		t.Fatalf("insert root: %v", err)
	}
}
