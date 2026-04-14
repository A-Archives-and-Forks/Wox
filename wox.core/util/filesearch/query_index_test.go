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
