package filesearch

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func makeProviderTestEntry(rootID string, fullPath string) EntryRecord {
	name := filepath.Base(fullPath)
	parentPath := filepath.Dir(fullPath)
	pinyinFull, pinyinInitials := buildPinyinFields(name)

	return EntryRecord{
		Path:           fullPath,
		RootID:         rootID,
		ParentPath:     parentPath,
		Name:           name,
		NormalizedName: normalizeIndexText(name),
		NormalizedPath: normalizePath(fullPath),
		PinyinFull:     pinyinFull,
		PinyinInitials: pinyinInitials,
	}
}

func TestDiffRootEntriesClassifiesAddRemoveAndUpdate(t *testing.T) {
	rootID := "root-provider-diff"

	unchangedOld := makeProviderTestEntry(rootID, filepath.Join("root", "keep.txt"))
	unchangedNew := unchangedOld
	unchangedNew.Mtime = 2
	unchangedNew.Size = 200
	unchangedNew.UpdatedAt = 300

	updatedOld := makeProviderTestEntry(rootID, filepath.Join("root", "rename.txt"))
	updatedNew := updatedOld
	updatedNew.PinyinFull = "renamed"
	updatedNew.PinyinInitials = "rmd"

	removed := makeProviderTestEntry(rootID, filepath.Join("root", "remove.txt"))
	added := makeProviderTestEntry(rootID, filepath.Join("root", "add.txt"))

	diff := diffRootEntries(
		rootID,
		[]EntryRecord{unchangedOld, updatedOld, removed},
		[]EntryRecord{unchangedNew, updatedNew, added},
	)

	if diff.RootID != rootID {
		t.Fatalf("expected diff root id %q, got %q", rootID, diff.RootID)
	}
	if len(diff.Added) != 1 || diff.Added[0].Path != added.Path {
		t.Fatalf("expected added entry %q, got %#v", added.Path, diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Path != removed.Path {
		t.Fatalf("expected removed entry %q, got %#v", removed.Path, diff.Removed)
	}
	if len(diff.Updated) != 1 {
		t.Fatalf("expected one updated entry, got %#v", diff.Updated)
	}
	if diff.Updated[0].Old.Path != updatedOld.Path || diff.Updated[0].New.PinyinFull != updatedNew.PinyinFull {
		t.Fatalf("expected updated entry %#v -> %#v, got %#v", updatedOld, updatedNew, diff.Updated)
	}
}

func TestLocalIndexProviderReplaceRootEntriesPatchesSmallRootDiff(t *testing.T) {
	rootID := "root-provider-patch"
	keep := makeProviderTestEntry(rootID, filepath.Join("root", "keep.txt"))
	removed := makeProviderTestEntry(rootID, filepath.Join("root", "removed.txt"))
	added := makeProviderTestEntry(rootID, filepath.Join("root", "added.txt"))

	provider := NewLocalIndexProvider()
	provider.ReplaceEntries([]EntryRecord{keep, removed})

	shard := provider.index.shards[rootID]
	keepPathKey := normalizeIndexText(keep.NormalizedPath)
	removedPathKey := normalizeIndexText(removed.NormalizedPath)
	addedPathKey := normalizeIndexText(added.NormalizedPath)
	keepDocIDBefore := shard.pathToDocID[keepPathKey]
	removedDocIDBefore := shard.pathToDocID[removedPathKey]

	provider.ReplaceRootEntries(rootID, []EntryRecord{added, keep})

	shard = provider.index.shards[rootID]
	keepDocIDAfter := shard.pathToDocID[keepPathKey]
	addedDocID := shard.pathToDocID[addedPathKey]

	if keepDocIDAfter != keepDocIDBefore {
		t.Fatalf("expected unchanged entry docID %d to be preserved, got %d", keepDocIDBefore, keepDocIDAfter)
	}
	if addedDocID != removedDocIDBefore {
		t.Fatalf("expected added entry to reuse removed docID %d, got %d", removedDocIDBefore, addedDocID)
	}
	if _, ok := shard.pathToDocID[removedPathKey]; ok {
		t.Fatalf("expected removed entry %q to be absent from path index", removed.Path)
	}

	results, err := provider.Search(context.Background(), SearchQuery{Raw: "added"}, 10)
	if err != nil {
		t.Fatalf("search for added entry: %v", err)
	}
	if len(results) != 1 || results[0].Path != added.Path {
		t.Fatalf("expected added entry %q in search results, got %#v", added.Path, results)
	}
}

func TestLocalIndexProviderApplyRootEntriesUsesExplicitDelta(t *testing.T) {
	rootID := "root-provider-explicit-delta"
	keep := makeProviderTestEntry(rootID, filepath.Join("root", "keep.txt"))
	removed := makeProviderTestEntry(rootID, filepath.Join("root", "removed.txt"))
	added := makeProviderTestEntry(rootID, filepath.Join("root", "added.txt"))

	provider := NewLocalIndexProvider()
	provider.ReplaceEntries([]EntryRecord{keep, removed})

	shard := provider.index.shards[rootID]
	keepPathKey := normalizeIndexText(keep.NormalizedPath)
	removedPathKey := normalizeIndexText(removed.NormalizedPath)
	addedPathKey := normalizeIndexText(added.NormalizedPath)
	keepDocIDBefore := shard.pathToDocID[keepPathKey]
	removedDocIDBefore := shard.pathToDocID[removedPathKey]

	delta := diffRootEntries(rootID, []EntryRecord{keep, removed}, []EntryRecord{added, keep})
	provider.ApplyRootEntries(rootID, []EntryRecord{added, keep}, delta)

	shard = provider.index.shards[rootID]
	if shard.pathToDocID[keepPathKey] != keepDocIDBefore {
		t.Fatalf("expected unchanged entry docID %d to be preserved after explicit delta apply, got %d", keepDocIDBefore, shard.pathToDocID[keepPathKey])
	}
	if shard.pathToDocID[addedPathKey] != removedDocIDBefore {
		t.Fatalf("expected added entry to reuse removed docID %d after explicit delta apply, got %d", removedDocIDBefore, shard.pathToDocID[addedPathKey])
	}
}

func TestLocalIndexProviderReplaceRootEntriesFallbacksToRebuildForLargeDiff(t *testing.T) {
	rootID := "root-provider-rebuild"
	keep := makeProviderTestEntry(rootID, filepath.Join("root", "keep.txt"))

	oldEntries := []EntryRecord{keep}
	for index := 0; index < 70; index++ {
		oldEntries = append(oldEntries, makeProviderTestEntry(rootID, filepath.Join("root", "old", fmt.Sprintf("old-%02d.txt", index))))
	}

	newEntries := make([]EntryRecord, 0, len(oldEntries))
	for index := 0; index < 70; index++ {
		newEntries = append(newEntries, makeProviderTestEntry(rootID, filepath.Join("root", "new", fmt.Sprintf("added-%02d.txt", index))))
	}
	newEntries = append(newEntries, keep)

	provider := NewLocalIndexProvider()
	provider.ReplaceEntries(oldEntries)

	keepPathKey := normalizeIndexText(keep.NormalizedPath)
	keepDocIDBefore := provider.index.shards[rootID].pathToDocID[keepPathKey]

	provider.ReplaceRootEntries(rootID, newEntries)

	keepDocIDAfter := provider.index.shards[rootID].pathToDocID[keepPathKey]
	if keepDocIDAfter == keepDocIDBefore {
		t.Fatalf("expected large diff to rebuild shard and reassign keep docID, still %d", keepDocIDAfter)
	}
}

func TestLocalIndexProviderReplaceRootEntriesFallbacksToRebuildForHighChangeRatio(t *testing.T) {
	rootID := "root-provider-rebuild-ratio"
	keep := makeProviderTestEntry(rootID, filepath.Join("root", "keep.txt"))

	oldEntries := []EntryRecord{keep}
	for index := 0; index < 10; index++ {
		oldEntries = append(oldEntries, makeProviderTestEntry(rootID, filepath.Join("root", "old", fmt.Sprintf("old-%02d.txt", index))))
	}

	newEntries := make([]EntryRecord, 0, len(oldEntries))
	for index := 0; index < 10; index++ {
		newEntries = append(newEntries, makeProviderTestEntry(rootID, filepath.Join("root", "new", fmt.Sprintf("new-%02d.txt", index))))
	}
	newEntries = append(newEntries, keep)

	provider := NewLocalIndexProvider()
	provider.ReplaceEntries(oldEntries)

	keepPathKey := normalizeIndexText(keep.NormalizedPath)
	keepDocIDBefore := provider.index.shards[rootID].pathToDocID[keepPathKey]

	provider.ReplaceRootEntries(rootID, newEntries)

	keepDocIDAfter := provider.index.shards[rootID].pathToDocID[keepPathKey]
	if keepDocIDAfter == keepDocIDBefore {
		t.Fatalf("expected high-ratio diff to rebuild shard and reassign keep docID, still %d", keepDocIDAfter)
	}
}

func TestLocalIndexProviderReplaceRootEntriesKeepsSnapshotAndApplyAtomic(t *testing.T) {
	rootID := "root-provider-atomic"
	keep := makeProviderTestEntry(rootID, filepath.Join("root", "keep.txt"))
	removed := makeProviderTestEntry(rootID, filepath.Join("root", "removed.txt"))
	first := makeProviderTestEntry(rootID, filepath.Join("root", "first.txt"))
	second := makeProviderTestEntry(rootID, filepath.Join("root", "second.txt"))

	provider := NewLocalIndexProvider()
	provider.ReplaceEntries([]EntryRecord{keep, removed})

	replaceBlocked := make(chan struct{})
	releaseReplace := make(chan struct{})
	provider.beforeReplaceRootEntriesApply = func(hookRootID string, _ EntryDeltaBatch, _ []EntryRecord) {
		if hookRootID != rootID {
			return
		}
		select {
		case <-replaceBlocked:
		default:
			close(replaceBlocked)
		}
		<-releaseReplace
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		provider.ReplaceRootEntries(rootID, []EntryRecord{keep, first})
	}()

	select {
	case <-replaceBlocked:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first ReplaceRootEntries call to block before apply")
	}

	go func() {
		defer wg.Done()
		provider.ReplaceRootEntries(rootID, []EntryRecord{keep, second})
	}()

	close(releaseReplace)
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for concurrent ReplaceRootEntries calls")
	}

	rootEntries := provider.SnapshotRootEntries(rootID)
	if len(rootEntries) != 2 {
		t.Fatalf("expected two root entries after serialized replaces, got %#v", rootEntries)
	}

	results, err := provider.Search(context.Background(), SearchQuery{Raw: "second"}, 10)
	if err != nil {
		t.Fatalf("search for latest root entry: %v", err)
	}
	if len(results) != 1 || results[0].Path != second.Path {
		t.Fatalf("expected latest root entry %q, got %#v", second.Path, results)
	}

	results, err = provider.Search(context.Background(), SearchQuery{Raw: "first"}, 10)
	if err != nil {
		t.Fatalf("search for stale root entry: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected stale root entry %q to be absent, got %#v", first.Path, results)
	}
}
