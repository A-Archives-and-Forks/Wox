package filesearch

import (
	"path/filepath"
	"sort"
	"strings"

	"wox/util"
)

func normalizeQuery(raw string) string {
	return strings.TrimSpace(raw)
}

func normalizePath(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if util.IsWindows() {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func buildSearchTerms(name string, path string, pinyinFull string, pinyinInitials string) []string {
	terms := []string{name, pinyinFull, pinyinInitials}
	return util.UniqueStrings(filterNonEmpty(terms))
}

func scoreSearchTerms(query string, terms []string) (bool, int64) {
	bestScore := int64(0)
	matched := false

	for _, term := range terms {
		isMatch, score := util.IsStringMatchScore(term, query, true)
		if !isMatch {
			continue
		}

		if !matched || score > bestScore {
			matched = true
			bestScore = score
		}
	}

	return matched, bestScore
}

func compareSearchResults(a SearchResult, b SearchResult) int {
	switch {
	case a.Score > b.Score:
		return -1
	case a.Score < b.Score:
		return 1
	case a.IsDir && !b.IsDir:
		return -1
	case !a.IsDir && b.IsDir:
		return 1
	case a.Name < b.Name:
		return -1
	case a.Name > b.Name:
		return 1
	case a.Path < b.Path:
		return -1
	case a.Path > b.Path:
		return 1
	default:
		return 0
	}
}

func sortAndLimitResults(results []SearchResult, limit int) []SearchResult {
	sort.Slice(results, func(i, j int) bool {
		return compareSearchResults(results[i], results[j]) < 0
	})

	if limit > 0 && len(results) > limit {
		return append([]SearchResult(nil), results[:limit]...)
	}

	return append([]SearchResult(nil), results...)
}

func filterNonEmpty(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}
