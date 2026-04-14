package filesearch

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDirtyQueueFlushReadyKeepsDisjointSubtreesSeparate(t *testing.T) {
	queue := NewDirtyQueue(DirtyQueueConfig{
		DebounceWindow:               50 * time.Millisecond,
		SiblingMergeThreshold:        8,
		RootEscalationPathThreshold:  512,
		RootEscalationDirectoryRatio: 0.10,
	})

	firstFile := filepath.Join(string(filepath.Separator), "root", "a", "b", "c", "file.txt")
	secondFile := filepath.Join(string(filepath.Separator), "root", "a", "d", "e", "file.txt")

	queue.Push(DirtySignal{Kind: DirtySignalKindPath, RootID: "root-a", Path: firstFile, PathTypeKnown: true, PathIsDir: false, At: time.Unix(0, 0)})
	queue.Push(DirtySignal{Kind: DirtySignalKindPath, RootID: "root-a", Path: secondFile, PathTypeKnown: true, PathIsDir: false, At: time.Unix(0, 0)})

	batches := queue.FlushReady(time.Unix(0, int64(60*time.Millisecond)), map[string]int{"root-a": 100})
	if len(batches) != 1 {
		t.Fatalf("expected one root batch, got %d", len(batches))
	}
	if batches[0].Mode != ReconcileModeSubtree {
		t.Fatalf("expected subtree reconcile, got %s", batches[0].Mode)
	}
	if batches[0].RootID != "root-a" {
		t.Fatalf("expected root-a batch, got %q", batches[0].RootID)
	}
	if batches[0].DirtyPathCount != 2 {
		t.Fatalf("expected 2 dirty paths, got %d", batches[0].DirtyPathCount)
	}
	expectedFirst := filepath.Join(string(filepath.Separator), "root", "a", "b", "c")
	expectedSecond := filepath.Join(string(filepath.Separator), "root", "a", "d", "e")
	if len(batches[0].Paths) != 2 || batches[0].Paths[0] != expectedFirst || batches[0].Paths[1] != expectedSecond {
		t.Fatalf("unexpected subtree paths: %#v", batches[0].Paths)
	}
}

func TestDirtyQueueFlushReadyCollapsesManySiblingPathsToParent(t *testing.T) {
	queue := NewDirtyQueue(DirtyQueueConfig{
		DebounceWindow:               0,
		SiblingMergeThreshold:        8,
		RootEscalationPathThreshold:  512,
		RootEscalationDirectoryRatio: 0.10,
	})

	for i := 0; i < 8; i++ {
		queue.Push(DirtySignal{
			Kind:          DirtySignalKindPath,
			RootID:        "root-a",
			Path:          filepath.Join(string(filepath.Separator), "root", "a", "parent", "child-"+string(rune('0'+i)), "grand", "file.txt"),
			PathTypeKnown: true,
			PathIsDir:     false,
			At:            time.Unix(0, 0),
		})
	}

	batches := queue.FlushReady(time.Unix(1, 0), map[string]int{"root-a": 100})
	if len(batches) != 1 {
		t.Fatalf("expected one root batch, got %d", len(batches))
	}
	if batches[0].Mode != ReconcileModeSubtree {
		t.Fatalf("expected subtree reconcile, got %s", batches[0].Mode)
	}
	if batches[0].DirtyPathCount != 8 {
		t.Fatalf("expected 8 dirty paths, got %d", batches[0].DirtyPathCount)
	}
	expectedPath := filepath.Join(string(filepath.Separator), "root", "a", "parent")
	if len(batches[0].Paths) != 1 || batches[0].Paths[0] != expectedPath {
		t.Fatalf("expected sibling collapse to %s, got %#v", expectedPath, batches[0].Paths)
	}
}

func TestDirtyQueueFlushReadyEscalatesLargeBatchToRoot(t *testing.T) {
	t.Run("path-threshold", func(t *testing.T) {
		queue := NewDirtyQueue(DirtyQueueConfig{
			DebounceWindow:               0,
			SiblingMergeThreshold:        99,
			RootEscalationPathThreshold:  10,
			RootEscalationDirectoryRatio: 0.10,
		})

		for i := 0; i < 11; i++ {
			queue.Push(DirtySignal{
				Kind:          DirtySignalKindPath,
				RootID:        "root-a",
				Path:          filepath.Join(string(filepath.Separator), "root", "a", "dir-"+string(rune('a'+i)), "grand", "file.txt"),
				PathTypeKnown: true,
				PathIsDir:     false,
				At:            time.Unix(0, 0),
			})
		}

		batches := queue.FlushReady(time.Unix(1, 0), map[string]int{"root-a": 100})
		if len(batches) != 1 {
			t.Fatalf("expected one root batch, got %d", len(batches))
		}
		if batches[0].Mode != ReconcileModeRoot {
			t.Fatalf("expected root reconcile, got %s", batches[0].Mode)
		}
		if batches[0].DirtyPathCount != 11 {
			t.Fatalf("expected 11 dirty paths, got %d", batches[0].DirtyPathCount)
		}
	})

	t.Run("disabled-thresholds", func(t *testing.T) {
		queue := NewDirtyQueue(DirtyQueueConfig{
			DebounceWindow:               0,
			SiblingMergeThreshold:        99,
			RootEscalationPathThreshold:  0,
			RootEscalationDirectoryRatio: 0,
		})

		for i := 0; i < 4; i++ {
			queue.Push(DirtySignal{
				Kind:          DirtySignalKindPath,
				RootID:        "root-a",
				Path:          filepath.Join(string(filepath.Separator), "root", "a", "dir-"+string(rune('a'+i)), "grand", "file.txt"),
				PathTypeKnown: true,
				PathIsDir:     false,
				At:            time.Unix(0, 0),
			})
		}

		batches := queue.FlushReady(time.Unix(1, 0), map[string]int{"root-a": 10})
		if len(batches) != 1 {
			t.Fatalf("expected one root batch, got %d", len(batches))
		}
		if batches[0].Mode != ReconcileModeSubtree {
			t.Fatalf("expected subtree reconcile with thresholds disabled, got %s", batches[0].Mode)
		}
		if batches[0].DirtyPathCount != 4 {
			t.Fatalf("expected 4 dirty paths, got %d", batches[0].DirtyPathCount)
		}
	})

	t.Run("directory-ratio", func(t *testing.T) {
		queue := NewDirtyQueue(DirtyQueueConfig{
			DebounceWindow:               0,
			SiblingMergeThreshold:        99,
			RootEscalationPathThreshold:  512,
			RootEscalationDirectoryRatio: 0.25,
		})

		for i := 0; i < 4; i++ {
			queue.Push(DirtySignal{
				Kind:          DirtySignalKindPath,
				RootID:        "root-a",
				Path:          filepath.Join(string(filepath.Separator), "root", "a", "dir-"+string(rune('a'+i)), "grand", "file.txt"),
				PathTypeKnown: true,
				PathIsDir:     false,
				At:            time.Unix(0, 0),
			})
		}

		batches := queue.FlushReady(time.Unix(1, 0), map[string]int{"root-a": 10})
		if len(batches) != 1 {
			t.Fatalf("expected one root batch, got %d", len(batches))
		}
		if batches[0].Mode != ReconcileModeRoot {
			t.Fatalf("expected root reconcile, got %s", batches[0].Mode)
		}
		if batches[0].DirtyPathCount != 4 {
			t.Fatalf("expected 4 dirty paths, got %d", batches[0].DirtyPathCount)
		}
	})

	t.Run("root-signal", func(t *testing.T) {
		queue := NewDirtyQueue(DirtyQueueConfig{
			DebounceWindow:               0,
			SiblingMergeThreshold:        8,
			RootEscalationPathThreshold:  512,
			RootEscalationDirectoryRatio: 0.10,
		})

		queue.Push(DirtySignal{
			Kind:   DirtySignalKindRoot,
			RootID: "root-a",
			At:     time.Unix(0, 0),
		})

		batches := queue.FlushReady(time.Unix(1, 0), map[string]int{"root-a": 10})
		if len(batches) != 1 {
			t.Fatalf("expected one root batch, got %d", len(batches))
		}
		if batches[0].Mode != ReconcileModeRoot {
			t.Fatalf("expected root reconcile, got %s", batches[0].Mode)
		}
		if batches[0].DirtyPathCount != 1 {
			t.Fatalf("expected 1 dirty path, got %d", batches[0].DirtyPathCount)
		}
		if len(batches[0].Paths) != 0 {
			t.Fatalf("expected empty root path list, got %#v", batches[0].Paths)
		}
	})
}

func TestDirtyQueueFlushReadyPreservesAbsolutePathVolumeRoot(t *testing.T) {
	queue := NewDirtyQueue(DirtyQueueConfig{
		DebounceWindow:               0,
		SiblingMergeThreshold:        8,
		RootEscalationPathThreshold:  512,
		RootEscalationDirectoryRatio: 0.10,
	})

	rootPath := filepath.Join(t.TempDir(), "root")
	filePath := filepath.Join(rootPath, "nested", "file.txt")
	expectedScope := filepath.Dir(filePath)

	queue.Push(DirtySignal{
		Kind:          DirtySignalKindPath,
		RootID:        "root-a",
		Path:          filePath,
		PathTypeKnown: true,
		PathIsDir:     false,
		At:            time.Unix(0, 0),
	})

	batches := queue.FlushReady(time.Unix(1, 0), map[string]int{"root-a": 100})
	if len(batches) != 1 {
		t.Fatalf("expected one root batch, got %d", len(batches))
	}
	if batches[0].Mode != ReconcileModeSubtree {
		t.Fatalf("expected subtree reconcile, got %s", batches[0].Mode)
	}
	if len(batches[0].Paths) != 1 || batches[0].Paths[0] != expectedScope {
		t.Fatalf("expected scope %q, got %#v", expectedScope, batches[0].Paths)
	}
}
