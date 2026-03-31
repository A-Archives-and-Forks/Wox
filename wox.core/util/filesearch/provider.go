package filesearch

import "context"

type SearchProvider interface {
	Name() string
	Search(ctx context.Context, query SearchQuery, limit int) ([]ProviderCandidate, error)
}
