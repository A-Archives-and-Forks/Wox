package filesearch

import (
	"context"
	"path/filepath"
	"testing"
)

func TestQueryIndexSearchSupportsExtensionPathAndPinyinRecall(t *testing.T) {
	rootID := "root-query-index"
	txtEntry := makeProviderTestEntry(rootID, filepath.Join("docs", "report.txt"))
	pathEntry := makeProviderTestEntry(rootID, filepath.Join("workspace", "src", "plugin", "main.go"))
	pinyinEntry := makeProviderTestEntry(rootID, filepath.Join("workspace", "总结报告.md"))

	index := newQueryIndex([]EntryRecord{txtEntry, pathEntry, pinyinEntry})

	results, _ := index.searchWithStats(context.Background(), normalizeSearchQuery(SearchQuery{Raw: "*.txt"}), 10)
	if len(results) != 1 || results[0].Path != txtEntry.Path {
		t.Fatalf("expected extension recall to return %q, got %#v", txtEntry.Path, results)
	}

	results, _ = index.searchWithStats(context.Background(), normalizeSearchQuery(SearchQuery{Raw: "src/plugin"}), 10)
	if len(results) != 1 || results[0].Path != pathEntry.Path {
		t.Fatalf("expected path recall to return %q, got %#v", pathEntry.Path, results)
	}

	results, _ = index.searchWithStats(context.Background(), normalizeSearchQuery(SearchQuery{Raw: "zjbg"}), 10)
	if len(results) != 1 || results[0].Path != pinyinEntry.Path {
		t.Fatalf("expected pinyin recall to return %q, got %#v", pinyinEntry.Path, results)
	}
}

func TestQueryIndexSearchStatsCountsMixedExtensionRecallOnce(t *testing.T) {
	rootID := "root-query-index-stats"
	first := makeProviderTestEntry(rootID, filepath.Join("docs", "report-alpha.txt"))
	second := makeProviderTestEntry(rootID, filepath.Join("docs", "report-beta.txt"))
	other := makeProviderTestEntry(rootID, filepath.Join("docs", "report-beta.md"))

	index := newQueryIndex([]EntryRecord{first, second, other})

	_, stats := index.searchWithStats(context.Background(), normalizeSearchQuery(SearchQuery{Raw: "report*.txt"}), 10)
	if stats.ExtensionRecall != 2 {
		t.Fatalf("expected extension recall count 2 for mixed name+extension query, got %#v", stats)
	}
}

func TestBuildDocRecordSkipsRedundantAsciiPinyinPayload(t *testing.T) {
	entry := makeProviderTestEntry("root-query-index-doc", filepath.Join("docs", "readme123"))
	record := buildDocRecord(1, entry)

	if record.PinyinFull != "" || record.PinyinInitials != "" {
		t.Fatalf("expected redundant ASCII pinyin payload to be dropped, got full=%q initials=%q", record.PinyinFull, record.PinyinInitials)
	}
}

func TestBuildDocRecordKeepsCollapsedAsciiPinyinPayloadWhenNameDiffers(t *testing.T) {
	entry := makeProviderTestEntry("root-query-index-doc", filepath.Join("docs", "foo-bar"))
	record := buildDocRecord(1, entry)

	if record.PinyinFull == "" || record.PinyinInitials == "" {
		t.Fatalf("expected collapsed ASCII pinyin payload to remain for %q, got %#v", entry.Name, record)
	}
}

func TestQueryIndexSnapshotReportsDocAndRootDistribution(t *testing.T) {
	rootHeavy := "root-query-index-heavy"
	rootLight := "root-query-index-light"
	index := newQueryIndex([]EntryRecord{
		makeProviderTestEntry(rootHeavy, filepath.Join("docs", "alpha-report.txt")),
		makeProviderTestEntry(rootHeavy, filepath.Join("docs", "beta-report.txt")),
		makeProviderTestEntry(rootLight, filepath.Join("workspace", "总结报告.md")),
	})

	snapshot := index.snapshot()
	if snapshot.RootCount != 2 {
		t.Fatalf("expected two roots in snapshot, got %#v", snapshot)
	}
	if snapshot.DocCount != 3 || snapshot.LiveDocRecords != 3 {
		t.Fatalf("expected snapshot doc counts to be 3, got %#v", snapshot)
	}
	if snapshot.PathToDocKeyCount != 3 {
		t.Fatalf("expected three path-to-doc keys, got %#v", snapshot)
	}
	if len(snapshot.TopRoots) != 2 || snapshot.TopRoots[0].RootID != rootHeavy {
		t.Fatalf("expected heavier root %q to lead top roots, got %#v", rootHeavy, snapshot.TopRoots)
	}
	if snapshot.NameBigram.PostingKeyCount == 0 || snapshot.NameBigram.PostingRefCount == 0 {
		t.Fatalf("expected name bigram distribution to be populated, got %#v", snapshot.NameBigram)
	}
	if snapshot.TotalBytesEstimate == 0 {
		t.Fatalf("expected non-zero estimated bytes, got %#v", snapshot)
	}
}
