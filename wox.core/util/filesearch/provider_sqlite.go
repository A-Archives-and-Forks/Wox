package filesearch

import (
	"context"
	"fmt"
	"strings"
)

type SQLiteSearchProvider struct {
	db *FileSearchDB
}

func NewSQLiteSearchProvider(db *FileSearchDB) *SQLiteSearchProvider {
	return &SQLiteSearchProvider{db: db}
}

func (p *SQLiteSearchProvider) Name() string {
	return "sqlite-search"
}

func (p *SQLiteSearchProvider) Search(ctx context.Context, query SearchQuery, limit int) ([]ProviderCandidate, error) {
	query = normalizeSearchQuery(query)
	if p == nil || p.db == nil || strings.TrimSpace(query.Raw) == "" {
		return nil, nil
	}

	candidateLimit := defaultPreRerankLimit
	if query.plan != nil && query.plan.preRerankLimit > 0 {
		candidateLimit = query.plan.preRerankLimit
	}
	if limit > 0 && candidateLimit < limit {
		candidateLimit = limit
	}

	candidateIDs, err := p.collectCandidateIDs(ctx, query, candidateLimit)
	if err != nil {
		return nil, err
	}
	if len(candidateIDs) == 0 {
		return nil, nil
	}

	rows, err := p.listEntriesByIDs(ctx, candidateIDs)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		record := docRecord{
			Path:           row.Path,
			IsDir:          row.IsDir,
			NormalizedName: row.NormalizedName,
			PinyinFull:     row.PinyinFull,
			PinyinInitials: row.PinyinInitials,
		}
		matched, score := scoreDocAgainstQuery(query, record, 0)
		if !matched {
			continue
		}
		results = append(results, SearchResult{
			Path:       row.Path,
			Name:       row.Name,
			ParentPath: row.ParentPath,
			IsDir:      row.IsDir,
			Score:      score,
		})
	}

	return convertResultsToCandidates(sortAndLimitResults(results, limit)), nil
}

func (p *SQLiteSearchProvider) collectCandidateIDs(ctx context.Context, query SearchQuery, limit int) ([]int64, error) {
	if query.plan == nil {
		return nil, nil
	}

	plan := query.plan
	if plan.extensionOnly {
		return p.queryIDsByExtension(ctx, plan.extension, limit)
	}

	if query.wildcard != nil {
		return p.collectWildcardCandidateIDs(ctx, query, limit)
	}

	switch plan.shortQueryLength {
	case 1:
		return p.collectOneCharacterCandidateIDs(ctx, query, limit)
	case 2:
		return p.collectTwoCharacterCandidateIDs(ctx, query, limit)
	default:
		return p.collectGeneralCandidateIDs(ctx, query, limit)
	}
}

func (p *SQLiteSearchProvider) collectOneCharacterCandidateIDs(ctx context.Context, query SearchQuery, limit int) ([]int64, error) {
	plan := query.plan
	if plan == nil || len(plan.rawLettersDigits) != 1 || plan.pathLike {
		return nil, nil
	}

	prefix := plan.rawLettersDigits
	return p.queryIDs(ctx, `
		SELECT entry_id
		FROM entries
		WHERE name_key >= ? AND name_key < ?
		ORDER BY name_key ASC, entry_id ASC
		LIMIT ?
	`, prefix, nextPrefixUpperBound(prefix), limit)
}

func (p *SQLiteSearchProvider) collectTwoCharacterCandidateIDs(ctx context.Context, query SearchQuery, limit int) ([]int64, error) {
	plan := query.plan
	if plan == nil {
		return nil, nil
	}

	if plan.pathLike {
		return p.queryPathFallbackIDs(ctx, plan.pathQuery, limit)
	}

	if !plan.asciiLettersDigits || len(plan.rawLettersDigits) != 2 {
		return nil, nil
	}

	// Two-character substring matching produced very broad recall and forced the
	// indexer to maintain the expensive bigram side table. Tightening short
	// queries to the same indexed name-key prefix path keeps response time fast
	// while making the reduced recall explicit and predictable.
	prefix := plan.rawLettersDigits
	return p.queryIDs(ctx, `
		SELECT entry_id
		FROM entries
		WHERE name_key >= ? AND name_key < ?
		ORDER BY name_key ASC, entry_id ASC
		LIMIT ?
	`, prefix, nextPrefixUpperBound(prefix), limit)
}

func (p *SQLiteSearchProvider) collectGeneralCandidateIDs(ctx context.Context, query SearchQuery, limit int) ([]int64, error) {
	plan := query.plan
	if plan == nil {
		return nil, nil
	}

	if plan.pathLike {
		return p.queryPathFTSIDs(ctx, plan, limit)
	}

	ids := make([]int64, 0, limit)
	nameIDs, err := p.queryFTSLikeIDs(ctx, "entries_name_fts", "normalized_name", "%"+escapeLikePattern(plan.nameTerm)+"%", limit)
	if err != nil {
		return nil, err
	}
	ids = append(ids, nameIDs...)

	if plan.asciiLettersDigits && len(plan.rawLettersDigits) >= 3 {
		pinyinFullIDs, err := p.queryFTSLikeIDs(ctx, "entries_pinyin_full_fts", "pinyin_full", "%"+escapeLikePattern(plan.rawLettersDigits)+"%", limit)
		if err != nil {
			return nil, err
		}
		ids = append(ids, pinyinFullIDs...)

		initialsIDs, err := p.queryFTSMatchIDs(ctx, "entries_initials_fts", plan.rawLettersDigits+"*", limit)
		if err != nil {
			return nil, err
		}
		ids = append(ids, initialsIDs...)
	}

	if plan.extension != "" {
		extensionIDs, err := p.queryIDsByExtension(ctx, plan.extension, limit)
		if err != nil {
			return nil, err
		}
		ids = append(ids, extensionIDs...)
	}

	return trimCandidateIDs(ids, limit), nil
}

func (p *SQLiteSearchProvider) collectWildcardCandidateIDs(ctx context.Context, query SearchQuery, limit int) ([]int64, error) {
	plan := query.plan
	if plan == nil {
		return nil, nil
	}

	literal := longestLiteralFromWildcard(query.Raw)
	if utf8LenString(literal) < 3 {
		if plan.pathLike {
			return p.queryPathFallbackIDs(ctx, plan.pathQuery, limit)
		}
		return p.queryNameFallbackIDs(ctx, plan.rawLower, limit)
	}

	pattern := normalizeIndexText(strings.ReplaceAll(query.Raw, "\\", "/"))
	targetTable := "entries_name_fts"
	targetColumn := "normalized_name"
	if plan.pathLike {
		targetTable = "entries_path_fts"
		targetColumn = "normalized_path"
		pattern = normalizePathQuery(query.Raw)
	}

	return p.queryIDs(ctx, fmt.Sprintf(`
		SELECT rowid
		FROM %s
		WHERE %s GLOB ?
		LIMIT ?
	`, targetTable, targetColumn), pattern, limit)
}

func (p *SQLiteSearchProvider) queryPathFTSIDs(ctx context.Context, plan *queryPlan, limit int) ([]int64, error) {
	if plan == nil {
		return nil, nil
	}

	segments := plan.pathSegments
	if len(segments) == 0 && strings.TrimSpace(plan.pathQuery) != "" {
		segments = []string{plan.pathQuery}
	}
	if len(segments) == 0 {
		return nil, nil
	}

	var intersected []int64
	for _, segment := range segments {
		if strings.TrimSpace(segment) == "" {
			continue
		}

		var ids []int64
		var err error
		if utf8LenString(segment) >= 3 {
			ids, err = p.queryFTSLikeIDs(ctx, "entries_path_fts", "normalized_path", "%"+escapeLikePattern(segment)+"%", plan.perClauseLimit)
		} else {
			ids, err = p.queryPathFallbackIDs(ctx, segment, plan.perClauseLimit)
		}
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, nil
		}

		if intersected == nil {
			intersected = ids
			continue
		}
		intersected = intersectInt64(intersected, ids, limit)
		if len(intersected) == 0 {
			return nil, nil
		}
	}

	if len(plan.pathQuery) >= 3 {
		fullPathIDs, err := p.queryFTSLikeIDs(ctx, "entries_path_fts", "normalized_path", "%"+escapeLikePattern(plan.pathQuery)+"%", limit)
		if err != nil {
			return nil, err
		}
		intersected = append(intersected, fullPathIDs...)
	}

	return trimCandidateIDs(intersected, limit), nil
}

func (p *SQLiteSearchProvider) queryIDsByExtension(ctx context.Context, extension string, limit int) ([]int64, error) {
	if strings.TrimSpace(extension) == "" {
		return nil, nil
	}
	return p.queryIDs(ctx, `
		SELECT entry_id
		FROM entries
		WHERE extension = ?
		ORDER BY entry_id ASC
		LIMIT ?
	`, extension, limit)
}

func (p *SQLiteSearchProvider) queryNameFallbackIDs(ctx context.Context, term string, limit int) ([]int64, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, nil
	}
	return p.queryIDs(ctx, `
		SELECT entry_id
		FROM entries
		WHERE normalized_name LIKE ? ESCAPE '\'
		ORDER BY entry_id ASC
		LIMIT ?
	`, "%"+escapeLikePattern(term)+"%", limit)
}

func (p *SQLiteSearchProvider) queryPathFallbackIDs(ctx context.Context, term string, limit int) ([]int64, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, nil
	}
	return p.queryIDs(ctx, `
		SELECT entry_id
		FROM entries
		WHERE normalized_path LIKE ? ESCAPE '\'
		ORDER BY entry_id ASC
		LIMIT ?
	`, "%"+escapeLikePattern(term)+"%", limit)
}

func (p *SQLiteSearchProvider) queryFTSLikeIDs(ctx context.Context, tableName string, columnName string, pattern string, limit int) ([]int64, error) {
	return p.queryIDs(ctx, fmt.Sprintf(`
		SELECT rowid
		FROM %s
		WHERE %s LIKE ? ESCAPE '\'
		LIMIT ?
	`, tableName, columnName), pattern, limit)
}

func (p *SQLiteSearchProvider) queryFTSMatchIDs(ctx context.Context, tableName string, expression string, limit int) ([]int64, error) {
	return p.queryIDs(ctx, fmt.Sprintf(`
		SELECT rowid
		FROM %s
		WHERE %s MATCH ?
		LIMIT ?
	`, tableName, tableName), expression, limit)
}

func (p *SQLiteSearchProvider) queryIDs(ctx context.Context, query string, args ...any) ([]int64, error) {
	rows, err := p.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var entryID int64
		if err := rows.Scan(&entryID); err != nil {
			return nil, err
		}
		ids = append(ids, entryID)
	}
	return ids, rows.Err()
}

func (p *SQLiteSearchProvider) listEntriesByIDs(ctx context.Context, entryIDs []int64) ([]storedEntryRecord, error) {
	entryIDs = trimCandidateIDs(entryIDs, 0)
	if len(entryIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(entryIDs))
	args := make([]any, 0, len(entryIDs))
	for _, entryID := range entryIDs {
		placeholders = append(placeholders, "?")
		args = append(args, entryID)
	}

	rows, err := p.db.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT entry_id, path, root_id, parent_path, name, normalized_name, name_key, normalized_path,
		       pinyin_full, pinyin_initials, extension, is_dir, mtime, size, updated_at
		FROM entries
		WHERE entry_id IN (%s)
	`, strings.Join(placeholders, ", ")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var loaded []storedEntryRecord
	for rows.Next() {
		row, err := scanStoredEntryRecord(rows)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return loaded, nil
}

func intersectInt64(left []int64, right []int64, limit int) []int64 {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}

	rightSet := make(map[int64]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}

	intersection := make([]int64, 0, min(len(left), len(right)))
	for _, value := range left {
		if _, ok := rightSet[value]; !ok {
			continue
		}
		intersection = append(intersection, value)
		if limit > 0 && len(intersection) >= limit {
			break
		}
	}
	return intersection
}
