package filesearch

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSnapshotBuilderExcludesDynamicChildEntriesAndTraversal(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "root-snapshot-exclusion")
	regularPath := filepath.Join(rootPath, "regular")
	dynamicPath := filepath.Join(rootPath, "workspace", "target")
	dynamicFilePath := filepath.Join(dynamicPath, "owned.txt")
	regularFilePath := filepath.Join(regularPath, "kept.txt")

	mustWriteTestFile(t, dynamicFilePath, "owned")
	mustWriteTestFile(t, regularFilePath, "kept")

	root := RootRecord{ID: "root-snapshot-parent", Path: rootPath, Kind: RootKindUser}
	builder := NewSnapshotBuilder(nil)
	builder.SetRootExclusions(map[string][]string{
		root.ID: []string{dynamicPath},
	})

	batch, err := builder.BuildSubtreeSnapshot(context.Background(), root, rootPath)
	if err != nil {
		t.Fatalf("build excluded subtree snapshot: %v", err)
	}

	seenEntries := map[string]bool{}
	for _, entry := range batch.Entries {
		seenEntries[entry.Path] = true
	}
	if !seenEntries[rootPath] || !seenEntries[regularPath] || !seenEntries[regularFilePath] {
		t.Fatalf("expected non-excluded paths to remain indexed, got %#v", seenEntries)
	}
	if seenEntries[dynamicPath] || seenEntries[dynamicFilePath] {
		t.Fatalf("expected dynamic child directory and descendants to be excluded, got %#v", seenEntries)
	}

	for _, directory := range batch.Directories {
		if directory.Path == dynamicPath {
			t.Fatalf("expected dynamic child directory row to be excluded")
		}
	}
}

func TestSnapshotBuilderExcludesDynamicChildFromDirectFiles(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "root-direct-exclusion")
	dynamicPath := filepath.Join(rootPath, "target")
	dynamicFilePath := filepath.Join(dynamicPath, "owned.txt")
	directFilePath := filepath.Join(rootPath, "kept.txt")

	mustWriteTestFile(t, dynamicFilePath, "owned")
	mustWriteTestFile(t, directFilePath, "kept")

	root := RootRecord{ID: "root-direct-parent", Path: rootPath, Kind: RootKindUser}
	builder := NewSnapshotBuilder(nil)
	builder.SetRootExclusions(map[string][]string{
		root.ID: []string{dynamicPath},
	})

	batch, err := builder.BuildDirectFilesJobSnapshot(context.Background(), root, Job{
		RootID:    root.ID,
		RootPath:  root.Path,
		ScopePath: root.Path,
		Kind:      JobKindDirectFiles,
	})
	if err != nil {
		t.Fatalf("build direct-files snapshot: %v", err)
	}

	seenEntries := map[string]bool{}
	for _, entry := range batch.Entries {
		seenEntries[entry.Path] = true
	}
	if !seenEntries[rootPath] || !seenEntries[directFilePath] {
		t.Fatalf("expected direct file scope and file, got %#v", seenEntries)
	}
	if seenEntries[dynamicPath] || seenEntries[dynamicFilePath] {
		t.Fatalf("expected dynamic child to be excluded from direct files snapshot, got %#v", seenEntries)
	}
}
