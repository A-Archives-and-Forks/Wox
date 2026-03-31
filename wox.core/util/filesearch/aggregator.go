package filesearch

import "fmt"

type resultAggregator struct {
	limit        int
	byPath       map[string]SearchResult
	lastSnapshot string
}

func newResultAggregator(limit int) *resultAggregator {
	return &resultAggregator{
		limit:  limit,
		byPath: map[string]SearchResult{},
	}
}

func (a *resultAggregator) Add(candidates []ProviderCandidate) ([]SearchResult, bool) {
	changed := false
	for _, candidate := range candidates {
		key := normalizePath(candidate.Path)
		if _, exists := a.byPath[key]; exists {
			continue
		}

		a.byPath[key] = SearchResult{
			Path:       candidate.Path,
			Name:       candidate.Name,
			ParentPath: candidate.ParentPath,
			IsDir:      candidate.IsDir,
			Score:      candidate.Score,
		}
		changed = true
	}

	snapshot := a.snapshot()
	signature := buildSnapshotSignature(snapshot)
	if signature == a.lastSnapshot {
		return snapshot, false
	}

	a.lastSnapshot = signature
	return snapshot, changed
}

func (a *resultAggregator) snapshot() []SearchResult {
	results := make([]SearchResult, 0, len(a.byPath))
	for _, result := range a.byPath {
		results = append(results, result)
	}
	return sortAndLimitResults(results, a.limit)
}

func buildSnapshotSignature(results []SearchResult) string {
	signature := ""
	for _, result := range results {
		signature += fmt.Sprintf("%s:%d|", normalizePath(result.Path), result.Score)
	}
	return signature
}
