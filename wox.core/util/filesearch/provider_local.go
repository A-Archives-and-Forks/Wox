package filesearch

import (
	"context"
	"fmt"
	"sort"
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

	p.entries = cloneEntryRecords(entries)
	p.entriesByRoot = groupEntriesByRoot(p.entries)
	p.index = newQueryIndex(p.entries)
}

func (p *LocalIndexProvider) ReplaceRootEntries(rootID string, entries []EntryRecord) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	oldRootEntries := cloneEntryRecords(p.entriesByRoot[rootID])
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

	return cloneEntryRecords(p.entriesByRoot[rootID])
}

func (p *LocalIndexProvider) ApplyRootEntries(rootID string, entries []EntryRecord, delta EntryDeltaBatch) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.applyRootEntriesLocked(rootID, cloneEntryRecords(entries), delta)
}

func (p *LocalIndexProvider) applyRootEntriesLocked(rootID string, rootEntries []EntryRecord, delta EntryDeltaBatch) int {

	if p.entriesByRoot == nil {
		p.entriesByRoot = map[string][]EntryRecord{}
	}

	if len(rootEntries) == 0 {
		delete(p.entriesByRoot, rootID)
	} else {
		p.entriesByRoot[rootID] = rootEntries
	}

	p.entries = flattenEntriesByRoot(p.entriesByRoot)
	if p.index == nil {
		p.index = newQueryIndex(p.entries)
	} else {
		if shouldRebuildRootEntries(delta) {
			p.index.replaceRootEntries(rootID, rootEntries)
		} else {
			p.index.patchRootEntries(rootID, delta, rootEntries)
		}
	}

	return len(p.entries)
}

func (p *LocalIndexProvider) Search(ctx context.Context, query SearchQuery, limit int) ([]ProviderCandidate, error) {
	searchStartedAt := util.GetSystemTimestamp()
	query = normalizeSearchQuery(query)

	p.mu.RLock()
	entries := append([]EntryRecord(nil), p.entries...)
	index := p.index
	p.mu.RUnlock()

	var indexStats querySearchStats
	if index != nil && query.plan != nil {
		results, stats := index.searchWithStats(ctx, query, limit)
		indexStats = stats
		if len(results) > 0 || !shouldFallbackToLinearScan(query) {
			logLocalIndexSearch(ctx, query, searchModeIndexed, util.GetSystemTimestamp()-searchStartedAt, len(entries), indexStats, len(results))
			return convertResultsToCandidates(sortAndLimitResults(results, limit)), nil
		}
	}

	results := make([]SearchResult, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return convertResultsToCandidates(sortAndLimitResults(results, limit)), ctx.Err()
		default:
		}

		matched, score := matchSearchQuery(query, entry.Name, entry.Path, entry.PinyinFull, entry.PinyinInitials)
		if !matched {
			continue
		}

		results = append(results, SearchResult{
			Path:       entry.Path,
			Name:       entry.Name,
			ParentPath: entry.ParentPath,
			IsDir:      entry.IsDir,
			Score:      score,
		})
	}

	logLocalIndexSearch(ctx, query, searchModeLinearFallback, util.GetSystemTimestamp()-searchStartedAt, len(entries), indexStats, len(results))
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

func diffRootEntries(rootID string, oldEntries []EntryRecord, newEntries []EntryRecord) EntryDeltaBatch {
	oldByPath := make(map[string]EntryRecord, len(oldEntries))
	newByPath := make(map[string]EntryRecord, len(newEntries))

	for _, entry := range oldEntries {
		oldByPath[entry.NormalizedPath] = entry
	}
	for _, entry := range newEntries {
		newByPath[entry.NormalizedPath] = entry
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
	return a.Path == b.Path &&
		a.RootID == b.RootID &&
		a.ParentPath == b.ParentPath &&
		a.Name == b.Name &&
		a.NormalizedName == b.NormalizedName &&
		a.NormalizedPath == b.NormalizedPath &&
		a.PinyinFull == b.PinyinFull &&
		a.PinyinInitials == b.PinyinInitials &&
		a.IsDir == b.IsDir
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

func groupEntriesByRoot(entries []EntryRecord) map[string][]EntryRecord {
	grouped := make(map[string][]EntryRecord)
	for _, entry := range entries {
		grouped[entry.RootID] = append(grouped[entry.RootID], entry)
	}
	return grouped
}

func flattenEntriesByRoot(entriesByRoot map[string][]EntryRecord) []EntryRecord {
	if len(entriesByRoot) == 0 {
		return nil
	}

	rootIDs := make([]string, 0, len(entriesByRoot))
	total := 0
	for rootID, entries := range entriesByRoot {
		rootIDs = append(rootIDs, rootID)
		total += len(entries)
	}
	sort.Strings(rootIDs)

	flattened := make([]EntryRecord, 0, total)
	for _, rootID := range rootIDs {
		flattened = append(flattened, entriesByRoot[rootID]...)
	}

	return flattened
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
