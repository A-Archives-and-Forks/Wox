package filesearch

import (
	"context"
	"fmt"
	"sync"

	"wox/util"
)

type LocalIndexProvider struct {
	mu            sync.RWMutex
	entries       []EntryRecord
	entriesByRoot map[string][]EntryRecord
	index         *queryIndex
	// Test hook to coordinate ReplaceRootEntries snapshot/apply ordering.
	beforeReplaceRootEntriesApply func(rootID string, delta EntryDeltaBatch, entries []EntryRecord)
}

const (
	rootPatchRebuildThreshold             = 64
	rootPatchRebuildMinimumAffected       = 16
	rootPatchRebuildRatioThreshold        = 0.50
	slowLocalIndexSearchThresholdMs int64 = 100
)

func NewLocalIndexProvider() *LocalIndexProvider {
	return &LocalIndexProvider{}
}

func (p *LocalIndexProvider) Name() string {
	return "local-index"
}

func (p *LocalIndexProvider) ReplaceEntries(entries []EntryRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Keep only the query index in steady state because duplicating the persisted
	// EntryRecord slices in memory made file search scale poorly once many roots
	// were indexed. Rebuilding the index directly preserves search behavior while
	// removing the largest avoidable mirror from the hot path.
	p.entries = nil
	p.entriesByRoot = nil
	p.index = newQueryIndex(cloneEntryRecords(entries))
}

func (p *LocalIndexProvider) ReplaceRootEntries(rootID string, entries []EntryRecord) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	oldRootEntries := p.snapshotRootEntriesLocked(rootID)
	rootEntries := cloneEntryRecords(entries)
	delta := diffRootEntries(rootID, oldRootEntries, rootEntries)
	if p.beforeReplaceRootEntriesApply != nil {
		p.beforeReplaceRootEntriesApply(rootID, delta, cloneEntryRecords(rootEntries))
	}

	return p.applyRootEntriesLocked(rootID, rootEntries, delta)
}

func (p *LocalIndexProvider) SnapshotRootEntries(rootID string) []EntryRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.snapshotRootEntriesLocked(rootID)
}

func (p *LocalIndexProvider) ApplyRootEntries(rootID string, entries []EntryRecord, delta EntryDeltaBatch) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.applyRootEntriesLocked(rootID, cloneEntryRecords(entries), delta)
}

func (p *LocalIndexProvider) applyRootEntriesLocked(rootID string, rootEntries []EntryRecord, delta EntryDeltaBatch) int {
	if p.index == nil {
		p.index = newQueryIndex(rootEntries)
	} else {
		if shouldRebuildRootEntries(delta) {
			p.index.replaceRootEntries(rootID, rootEntries)
		} else {
			p.index.patchRootEntries(rootID, delta, rootEntries)
		}
	}

	p.entries = nil
	p.entriesByRoot = nil
	return p.index.docCount()
}

func (p *LocalIndexProvider) Search(ctx context.Context, query SearchQuery, limit int) ([]ProviderCandidate, error) {
	searchStartedAt := util.GetSystemTimestamp()
	query = normalizeSearchQuery(query)

	p.mu.RLock()
	index := p.index
	p.mu.RUnlock()

	entryCount := 0
	if index != nil {
		entryCount = index.docCount()
	}

	var indexStats querySearchStats
	if index != nil && query.plan != nil {
		results, stats := index.searchWithStats(ctx, query, limit)
		indexStats = stats
		if len(results) > 0 || !shouldFallbackToLinearScan(query) {
			logLocalIndexSearch(ctx, query, searchModeIndexed, util.GetSystemTimestamp()-searchStartedAt, entryCount, indexStats, len(results))
			return convertResultsToCandidates(sortAndLimitResults(results, limit)), nil
		}
	}

	// Fall back to live doc records because the old code scanned a duplicated
	// EntryRecord mirror. Using the index-backed records keeps path/short-query
	// fallback correct after steady-state mirrors are removed.
	results, err := searchLinearFromIndex(ctx, index, query, limit)
	if err != nil {
		return convertResultsToCandidates(sortAndLimitResults(results, limit)), err
	}

	logLocalIndexSearch(ctx, query, searchModeLinearFallback, util.GetSystemTimestamp()-searchStartedAt, entryCount, indexStats, len(results))
	return convertResultsToCandidates(sortAndLimitResults(results, limit)), nil
}

func shouldFallbackToLinearScan(query SearchQuery) bool {
	if query.plan == nil {
		return true
	}

	if query.wildcard != nil || query.plan.extensionOnly {
		return false
	}

	return query.plan.pathLike || (query.plan.shortQueryLength > 0 && query.plan.shortQueryLength <= 4)
}

func convertResultsToCandidates(results []SearchResult) []ProviderCandidate {
	candidates := make([]ProviderCandidate, 0, len(results))
	for _, result := range results {
		candidates = append(candidates, ProviderCandidate{
			Path:       result.Path,
			Name:       result.Name,
			ParentPath: result.ParentPath,
			IsDir:      result.IsDir,
			Score:      result.Score,
		})
	}
	return candidates
}

func cloneEntryRecords(entries []EntryRecord) []EntryRecord {
	return append([]EntryRecord(nil), entries...)
}

func (p *LocalIndexProvider) snapshotRootEntriesLocked(rootID string) []EntryRecord {
	if p == nil || p.index == nil {
		return nil
	}
	return p.index.snapshotRootEntries(rootID)
}

func (p *LocalIndexProvider) snapshot() queryIndexSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p == nil || p.index == nil {
		return queryIndexSnapshot{}
	}
	return p.index.snapshot()
}

func diffRootEntries(rootID string, oldEntries []EntryRecord, newEntries []EntryRecord) EntryDeltaBatch {
	oldByPath := make(map[string]EntryRecord, len(oldEntries))
	newByPath := make(map[string]EntryRecord, len(newEntries))

	for _, entry := range oldEntries {
		// Root snapshots materialize normalized paths with slash separators, while
		// live scanner entries still carry platform-native separators on Windows.
		// Normalizing both sides to the same query-index key avoids false
		// delete+add diffs that would rebuild or reshuffle stable doc IDs.
		oldByPath[normalizeEntryPathKey(entry)] = entry
	}
	for _, entry := range newEntries {
		newByPath[normalizeEntryPathKey(entry)] = entry
	}

	diff := EntryDeltaBatch{
		RootID:        rootID,
		PreviousCount: len(oldEntries),
		NextCount:     len(newEntries),
	}
	for normalizedPath, oldEntry := range oldByPath {
		newEntry, ok := newByPath[normalizedPath]
		if !ok {
			diff.Removed = append(diff.Removed, oldEntry)
			continue
		}
		if !sameQueryIndexFields(oldEntry, newEntry) {
			diff.Updated = append(diff.Updated, EntryUpdate{Old: oldEntry, New: newEntry})
		}
	}

	for normalizedPath, newEntry := range newByPath {
		if _, ok := oldByPath[normalizedPath]; ok {
			continue
		}
		diff.Added = append(diff.Added, newEntry)
	}

	return diff
}

func sameQueryIndexFields(a EntryRecord, b EntryRecord) bool {
	aPinyinFull, aPinyinInitials := comparableEntryPinyin(a)
	bPinyinFull, bPinyinInitials := comparableEntryPinyin(b)

	return a.Path == b.Path &&
		a.RootID == b.RootID &&
		a.ParentPath == b.ParentPath &&
		a.Name == b.Name &&
		a.NormalizedName == b.NormalizedName &&
		a.NormalizedPath == b.NormalizedPath &&
		aPinyinFull == bPinyinFull &&
		aPinyinInitials == bPinyinInitials &&
		a.IsDir == b.IsDir
}

func comparableEntryPinyin(entry EntryRecord) (string, string) {
	normalizedName := normalizeIndexText(entry.NormalizedName)
	pinyinFull := normalizeIndexText(entry.PinyinFull)
	pinyinInitials := normalizeIndexText(entry.PinyinInitials)
	if shouldDropRedundantPinyinPayload(normalizedName, pinyinFull, pinyinInitials) {
		return "", ""
	}
	return pinyinFull, pinyinInitials
}

func shouldRebuildRootEntries(diff EntryDeltaBatch) bool {
	if diff.ForceRebuild {
		return true
	}

	affected := len(diff.Added) + len(diff.Updated) + len(diff.Removed)
	if affected >= rootPatchRebuildThreshold {
		return true
	}

	baseCount := diff.PreviousCount
	if diff.NextCount > baseCount {
		baseCount = diff.NextCount
	}
	if baseCount == 0 || affected < rootPatchRebuildMinimumAffected {
		return false
	}

	return float64(affected)/float64(baseCount) >= rootPatchRebuildRatioThreshold
}

type localIndexSearchMode string

const (
	searchModeIndexed        localIndexSearchMode = "indexed"
	searchModeLinearFallback localIndexSearchMode = "linear-fallback"
)

func logLocalIndexSearch(ctx context.Context, query SearchQuery, mode localIndexSearchMode, elapsedMs int64, entryCount int, indexStats querySearchStats, resultCount int) {
	if query.Raw == "" {
		return
	}

	msg := fmt.Sprintf(
		"filesearch local query: mode=%s query=%q elapsed=%dms entries=%d shards=%d candidates=%d rerank=%d results=%d name_recall=%d path_recall=%d pinyin_full_recall=%d pinyin_initial_recall=%d extension_recall=%d trimmed_shards=%d",
		mode,
		query.Raw,
		elapsedMs,
		entryCount,
		indexStats.ShardCount,
		indexStats.CandidateCount,
		indexStats.RerankCount,
		resultCount,
		indexStats.NameRecallCount,
		indexStats.PathRecallCount,
		indexStats.PinyinFullRecall,
		indexStats.PinyinInitialRecall,
		indexStats.ExtensionRecall,
		indexStats.TrimmedShardCount,
	)

	if elapsedMs >= slowLocalIndexSearchThresholdMs {
		util.GetLogger().Info(ctx, "filesearch slow query: "+msg)
		return
	}

	util.GetLogger().Debug(ctx, msg)
}

func searchLinearFromIndex(ctx context.Context, index *queryIndex, query SearchQuery, limit int) ([]SearchResult, error) {
	if index == nil {
		return nil, nil
	}

	results := make([]SearchResult, 0)
	for _, record := range index.docRecords() {
		select {
		case <-ctx.Done():
			return sortAndLimitResults(results, limit), ctx.Err()
		default:
		}

		name := record.name()
		parentPath := record.parentPath()
		matched, score := matchSearchQuery(query, name, record.Path, record.PinyinFull, record.PinyinInitials)
		if !matched {
			continue
		}

		results = append(results, SearchResult{
			Path:       record.Path,
			Name:       name,
			ParentPath: parentPath,
			IsDir:      record.IsDir,
			Score:      score,
		})
	}

	return sortAndLimitResults(results, limit), nil
}
