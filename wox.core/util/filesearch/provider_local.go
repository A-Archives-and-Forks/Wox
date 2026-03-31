package filesearch

import (
	"context"
	"sync"
)

type LocalIndexProvider struct {
	mu      sync.RWMutex
	entries []EntryRecord
}

func NewLocalIndexProvider() *LocalIndexProvider {
	return &LocalIndexProvider{}
}

func (p *LocalIndexProvider) Name() string {
	return "local-index"
}

func (p *LocalIndexProvider) ReplaceEntries(entries []EntryRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.entries = append([]EntryRecord(nil), entries...)
}

func (p *LocalIndexProvider) Search(ctx context.Context, query SearchQuery, limit int) ([]ProviderCandidate, error) {
	p.mu.RLock()
	entries := append([]EntryRecord(nil), p.entries...)
	p.mu.RUnlock()

	results := make([]SearchResult, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return convertResultsToCandidates(sortAndLimitResults(results, limit)), ctx.Err()
		default:
		}

		terms := buildSearchTerms(entry.Name, entry.Path, entry.PinyinFull, entry.PinyinInitials)
		matched, score := scoreSearchTerms(query.Raw, terms)
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

	return convertResultsToCandidates(sortAndLimitResults(results, limit)), nil
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
