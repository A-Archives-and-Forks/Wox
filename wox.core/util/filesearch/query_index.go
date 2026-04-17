package filesearch

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"wox/util"
)

type DocID uint32

type queryIndex struct {
	shards map[string]*rootShard
}

type querySearchStats struct {
	ShardCount          int
	CandidateCount      int
	RerankCount         int
	ResultCount         int
	NameRecallCount     int
	PathRecallCount     int
	PinyinFullRecall    int
	PinyinInitialRecall int
	ExtensionRecall     int
	TrimmedShardCount   int
}

type shardSearchStats struct {
	CandidateCount      int
	RerankCount         int
	ResultCount         int
	NameRecallCount     int
	PathRecallCount     int
	PinyinFullRecall    int
	PinyinInitialRecall int
	ExtensionRecall     int
	Trimmed             bool
}

type shardSearchOutcome struct {
	results []SearchResult
	stats   shardSearchStats
}

type rootShard struct {
	rootID                 string
	nextDocID              DocID
	docTable               map[DocID]docRecord
	pathToDocID            map[string]DocID
	freedDocIDs            []DocID
	extensionIndex         map[string][]DocID
	namePrefixIndex        map[string][]DocID
	nameBigramIndex        map[string][]DocID
	nameTrigramIndex       map[string][]DocID
	pathSegmentIndex       map[string][]DocID
	pathTrigramIndex       map[string][]DocID
	pinyinFullBigramIndex  map[string][]DocID
	pinyinFullTrigramIndex map[string][]DocID
	initialsTrie           *initialsTrieNode
}

type docRecord struct {
	ID             DocID
	Path           string
	Name           string
	ParentPath     string
	DirectoryPath  string
	IsDir          bool
	NormalizedName string
	NormalizedPath string
	PinyinFull     string
	PinyinInitials string
}

type initialsTrieNode struct {
	children map[rune]*initialsTrieNode
	docIDs   []DocID
}

type queryPlan struct {
	raw                   string
	rawLower              string
	rawLettersDigits      string
	pathLike              bool
	asciiLettersDigits    bool
	extension             string
	extensionOnly         bool
	pathQuery             string
	pathSegments          []string
	wildcardLiterals      []string
	nameTerm              string
	perClauseLimit        int
	postIntersectionLimit int
	preRerankLimit        int
	shortQueryLength      int
}

type queryMatchHint uint8

const (
	matchHintName queryMatchHint = 1 << iota
	matchHintPath
	matchHintPinyinFull
	matchHintPinyinInitial
	matchHintExtension
)

const (
	defaultPerClauseLimit        = 20000
	defaultPostIntersectionLimit = 10000
	defaultPreRerankLimit        = 4000
)

func newQueryIndex(entries []EntryRecord) *queryIndex {
	shards := map[string]*rootShard{}
	grouped := map[string][]EntryRecord{}
	for _, entry := range entries {
		grouped[entry.RootID] = append(grouped[entry.RootID], entry)
	}

	for rootID, rootEntries := range grouped {
		shards[rootID] = buildRootShard(rootID, rootEntries)
	}

	return &queryIndex{shards: shards}
}

func (i *queryIndex) replaceRootEntries(rootID string, entries []EntryRecord) {
	if i == nil {
		return
	}
	if i.shards == nil {
		i.shards = map[string]*rootShard{}
	}
	if len(entries) == 0 {
		delete(i.shards, rootID)
		return
	}

	i.shards[rootID] = buildRootShard(rootID, entries)
}

func (i *queryIndex) patchRootEntries(rootID string, diff EntryDeltaBatch, entries []EntryRecord) {
	if i == nil {
		return
	}
	if i.shards == nil {
		i.shards = map[string]*rootShard{}
	}
	if len(entries) == 0 {
		delete(i.shards, rootID)
		return
	}

	shard := i.shards[rootID]
	if shard == nil {
		i.shards[rootID] = buildRootShard(rootID, entries)
		return
	}

	shard.applyDiff(diff)
}

func buildRootShard(rootID string, entries []EntryRecord) *rootShard {
	shard := &rootShard{
		rootID:                 rootID,
		nextDocID:              1,
		docTable:               map[DocID]docRecord{},
		pathToDocID:            map[string]DocID{},
		extensionIndex:         map[string][]DocID{},
		namePrefixIndex:        map[string][]DocID{},
		nameBigramIndex:        map[string][]DocID{},
		nameTrigramIndex:       map[string][]DocID{},
		pathSegmentIndex:       map[string][]DocID{},
		pathTrigramIndex:       map[string][]DocID{},
		pinyinFullBigramIndex:  map[string][]DocID{},
		pinyinFullTrigramIndex: map[string][]DocID{},
		initialsTrie:           newInitialsTrieNode(),
	}

	for _, entry := range entries {
		docID := shard.allocateDocID()
		record := buildDocRecord(docID, entry)

		shard.docTable[docID] = record
		shard.pathToDocID[record.NormalizedPath] = docID
		shard.indexDoc(record)
	}

	return shard
}

func (s *rootShard) allocateDocID() DocID {
	if n := len(s.freedDocIDs); n > 0 {
		docID := s.freedDocIDs[n-1]
		s.freedDocIDs = s.freedDocIDs[:n-1]
		return docID
	}

	docID := s.nextDocID
	s.nextDocID++
	return docID
}

func (s *rootShard) indexDoc(record docRecord) {
	insertDocID(s.extensionIndex, normalizeExtension(filepath.Ext(record.Name)), record.ID)

	namePrefixes := buildNamePrefixes(record.NormalizedName)
	for _, prefix := range namePrefixes {
		insertDocID(s.namePrefixIndex, prefix, record.ID)
	}

	for _, gram := range uniqueNgrams(record.NormalizedName, 2) {
		insertDocID(s.nameBigramIndex, gram, record.ID)
	}
	for _, gram := range uniqueNgrams(record.NormalizedName, 3) {
		insertDocID(s.nameTrigramIndex, gram, record.ID)
	}

	directoryPath := record.directoryPath()
	for _, segment := range uniqueDirectorySegments(directoryPath) {
		insertDocID(s.pathSegmentIndex, segment, record.ID)
	}
	for _, gram := range uniqueNgrams(directoryPath, 3) {
		insertDocID(s.pathTrigramIndex, gram, record.ID)
	}

	for _, gram := range uniqueNgrams(record.PinyinFull, 2) {
		insertDocID(s.pinyinFullBigramIndex, gram, record.ID)
	}
	for _, gram := range uniqueNgrams(record.PinyinFull, 3) {
		insertDocID(s.pinyinFullTrigramIndex, gram, record.ID)
	}

	s.initialsTrie.insert(record.PinyinInitials, record.ID)
}

func (s *rootShard) applyDiff(diff EntryDeltaBatch) {
	for _, removed := range diff.Removed {
		s.removeEntry(removed)
	}
	for _, updated := range diff.Updated {
		s.updateEntry(updated.Old, updated.New)
	}
	for _, added := range diff.Added {
		s.addEntry(added)
	}
}

func (s *rootShard) addEntry(entry EntryRecord) {
	docID := s.allocateDocID()
	record := buildDocRecord(docID, entry)
	s.docTable[docID] = record
	s.pathToDocID[record.NormalizedPath] = docID
	s.indexDoc(record)
}

func (s *rootShard) updateEntry(oldEntry EntryRecord, newEntry EntryRecord) {
	oldKey := normalizeEntryPathKey(oldEntry)
	docID, ok := s.pathToDocID[oldKey]
	if !ok {
		s.addEntry(newEntry)
		return
	}

	oldRecord, ok := s.docTable[docID]
	if !ok {
		s.addEntry(newEntry)
		return
	}

	s.deindexDoc(oldRecord)
	delete(s.pathToDocID, oldKey)

	newRecord := buildDocRecord(docID, newEntry)
	s.docTable[docID] = newRecord
	s.pathToDocID[newRecord.NormalizedPath] = docID
	s.indexDoc(newRecord)
}

func (s *rootShard) removeEntry(entry EntryRecord) {
	pathKey := normalizeEntryPathKey(entry)
	docID, ok := s.pathToDocID[pathKey]
	if !ok {
		return
	}

	record, ok := s.docTable[docID]
	if !ok {
		delete(s.pathToDocID, pathKey)
		return
	}

	s.deindexDoc(record)
	delete(s.docTable, docID)
	delete(s.pathToDocID, pathKey)
	s.freedDocIDs = append(s.freedDocIDs, docID)
}

func (s *rootShard) deindexDoc(record docRecord) {
	removeDocID(s.extensionIndex, normalizeExtension(filepath.Ext(record.Name)), record.ID)

	for _, prefix := range buildNamePrefixes(record.NormalizedName) {
		removeDocID(s.namePrefixIndex, prefix, record.ID)
	}
	for _, gram := range uniqueNgrams(record.NormalizedName, 2) {
		removeDocID(s.nameBigramIndex, gram, record.ID)
	}
	for _, gram := range uniqueNgrams(record.NormalizedName, 3) {
		removeDocID(s.nameTrigramIndex, gram, record.ID)
	}

	directoryPath := record.directoryPath()
	for _, segment := range uniqueDirectorySegments(directoryPath) {
		removeDocID(s.pathSegmentIndex, segment, record.ID)
	}
	for _, gram := range uniqueNgrams(directoryPath, 3) {
		removeDocID(s.pathTrigramIndex, gram, record.ID)
	}

	for _, gram := range uniqueNgrams(record.PinyinFull, 2) {
		removeDocID(s.pinyinFullBigramIndex, gram, record.ID)
	}
	for _, gram := range uniqueNgrams(record.PinyinFull, 3) {
		removeDocID(s.pinyinFullTrigramIndex, gram, record.ID)
	}

	s.initialsTrie.remove(record.PinyinInitials, record.ID)
}

func (record docRecord) directoryPath() string {
	return record.DirectoryPath
}

func (i *queryIndex) search(ctx context.Context, query SearchQuery, limit int) []SearchResult {
	results, _ := i.searchWithStats(ctx, query, limit)
	return results
}

func (i *queryIndex) searchWithStats(ctx context.Context, query SearchQuery, limit int) ([]SearchResult, querySearchStats) {
	if i == nil || len(i.shards) == 0 {
		return nil, querySearchStats{}
	}

	shards := make([]*rootShard, 0, len(i.shards))
	for _, shard := range i.shards {
		shards = append(shards, shard)
	}

	stats := querySearchStats{ShardCount: len(shards)}
	if len(shards) == 1 {
		shardResults, shardStats := shards[0].searchWithStats(ctx, query)
		stats.CandidateCount = shardStats.CandidateCount
		stats.RerankCount = shardStats.RerankCount
		stats.NameRecallCount = shardStats.NameRecallCount
		stats.PathRecallCount = shardStats.PathRecallCount
		stats.PinyinFullRecall = shardStats.PinyinFullRecall
		stats.PinyinInitialRecall = shardStats.PinyinInitialRecall
		stats.ExtensionRecall = shardStats.ExtensionRecall
		if shardStats.Trimmed {
			stats.TrimmedShardCount = 1
		}

		final := sortAndLimitResults(shardResults, limit)
		stats.ResultCount = len(final)
		return final, stats
	}

	// Search root shards in parallel because short queries such as "se" can fan out
	// into thousands of candidates. The previous serial loop stacked each shard's
	// rerank cost linearly even though shard lookups are independent and read-only.
	outcomes := make(chan shardSearchOutcome, len(shards))
	var wg sync.WaitGroup
	for _, shard := range shards {
		wg.Add(1)
		go func(shard *rootShard) {
			defer wg.Done()
			shardResults, shardStats := shard.searchWithStats(ctx, query)
			outcomes <- shardSearchOutcome{results: shardResults, stats: shardStats}
		}(shard)
	}

	go func() {
		wg.Wait()
		close(outcomes)
	}()

	results := make([]SearchResult, 0, limit)
	for outcome := range outcomes {
		stats.CandidateCount += outcome.stats.CandidateCount
		stats.RerankCount += outcome.stats.RerankCount
		stats.NameRecallCount += outcome.stats.NameRecallCount
		stats.PathRecallCount += outcome.stats.PathRecallCount
		stats.PinyinFullRecall += outcome.stats.PinyinFullRecall
		stats.PinyinInitialRecall += outcome.stats.PinyinInitialRecall
		stats.ExtensionRecall += outcome.stats.ExtensionRecall
		if outcome.stats.Trimmed {
			stats.TrimmedShardCount++
		}
		results = append(results, outcome.results...)
	}

	final := sortAndLimitResults(results, limit)
	stats.ResultCount = len(final)
	return final, stats
}

func (s *rootShard) search(ctx context.Context, query SearchQuery) []SearchResult {
	results, _ := s.searchWithStats(ctx, query)
	return results
}

func (s *rootShard) searchWithStats(ctx context.Context, query SearchQuery) ([]SearchResult, shardSearchStats) {
	plan := query.plan
	if s == nil || plan == nil {
		return nil, shardSearchStats{}
	}

	candidateHints, candidateIDs, stats := s.recallCandidates(plan)
	if len(candidateIDs) == 0 {
		return nil, stats
	}

	stats.CandidateCount = len(candidateIDs)
	stats.RerankCount = len(candidateIDs)
	results := make([]SearchResult, 0, len(candidateIDs))
	for _, docID := range candidateIDs {
		select {
		case <-ctx.Done():
			stats.ResultCount = len(results)
			return results, stats
		default:
		}

		record, ok := s.docTable[docID]
		if !ok {
			continue
		}

		matched, score := scoreDocAgainstQuery(query, record, candidateHints[docID])
		if !matched {
			continue
		}

		results = append(results, SearchResult{
			Path:       record.Path,
			Name:       record.Name,
			ParentPath: record.ParentPath,
			IsDir:      record.IsDir,
			Score:      score,
		})
	}

	stats.ResultCount = len(results)
	return results, stats
}

func (s *rootShard) recallCandidates(plan *queryPlan) (map[DocID]queryMatchHint, []DocID, shardSearchStats) {
	stats := shardSearchStats{}
	if plan == nil {
		return nil, nil, stats
	}

	var textSources []recallSource
	if plan.pathLike {
		if source := s.recallPath(plan); len(source.docIDs) > 0 {
			textSources = append(textSources, source)
			stats.PathRecallCount += len(source.docIDs)
		}
	} else {
		if source := s.recallName(plan); len(source.docIDs) > 0 {
			textSources = append(textSources, source)
			stats.NameRecallCount += len(source.docIDs)
		}
		if plan.asciiLettersDigits {
			if source := s.recallPinyinFull(plan); len(source.docIDs) > 0 {
				textSources = append(textSources, source)
				stats.PinyinFullRecall += len(source.docIDs)
			}
			if source := s.recallPinyinInitials(plan); len(source.docIDs) > 0 {
				textSources = append(textSources, source)
				stats.PinyinInitialRecall += len(source.docIDs)
			}
		}
	}

	var candidateHints map[DocID]queryMatchHint
	var candidateIDs []DocID
	if len(textSources) > 0 {
		candidateHints, candidateIDs = mergeRecallSources(textSources, plan.postIntersectionLimit)
	} else if plan.extension != "" {
		extDocIDs := s.extensionIndex[plan.extension]
		candidateHints, candidateIDs = docIDsToHints(extDocIDs, matchHintExtension, 0)
		stats.ExtensionRecall += len(candidateIDs)
	}

	if len(candidateIDs) == 0 {
		return nil, nil, stats
	}

	if plan.extension != "" && !(plan.extensionOnly && len(textSources) == 0) {
		filteredHints, filteredIDs := filterCandidateSetByPosting(candidateHints, candidateIDs, s.extensionIndex[plan.extension], matchHintExtension)
		candidateHints, candidateIDs = filteredHints, filteredIDs
		if len(s.extensionIndex[plan.extension]) > 0 {
			stats.ExtensionRecall += len(s.extensionIndex[plan.extension])
		}
	}

	if len(candidateIDs) == 0 {
		return nil, nil, stats
	}

	if plan.preRerankLimit > 0 && len(candidateIDs) > plan.preRerankLimit && !plan.extensionOnly {
		candidateIDs = candidateIDs[:plan.preRerankLimit]
		trimmed := make(map[DocID]queryMatchHint, len(candidateIDs))
		for _, docID := range candidateIDs {
			trimmed[docID] = candidateHints[docID]
		}
		candidateHints = trimmed
		stats.Trimmed = true
	}

	return candidateHints, candidateIDs, stats
}

type recallSource struct {
	docIDs []DocID
	hint   queryMatchHint
}

func mergeRecallSources(sources []recallSource, limit int) (map[DocID]queryMatchHint, []DocID) {
	// Recall sources for a single token family intentionally merge with OR semantics.
	// Multi-term AND semantics are handled before this by intersecting postings inside each source.
	candidateHints := map[DocID]queryMatchHint{}
	candidateIDs := make([]DocID, 0)
	for _, source := range sources {
		for _, docID := range source.docIDs {
			if _, exists := candidateHints[docID]; !exists {
				if limit > 0 && len(candidateIDs) >= limit {
					continue
				}
				candidateIDs = append(candidateIDs, docID)
			}
			candidateHints[docID] |= source.hint
		}
	}

	sort.Slice(candidateIDs, func(i, j int) bool {
		return candidateIDs[i] < candidateIDs[j]
	})

	return candidateHints, candidateIDs
}

func docIDsToHints(docIDs []DocID, hint queryMatchHint, limit int) (map[DocID]queryMatchHint, []DocID) {
	if limit > 0 && len(docIDs) > limit {
		docIDs = docIDs[:limit]
	}

	hints := make(map[DocID]queryMatchHint, len(docIDs))
	ids := make([]DocID, 0, len(docIDs))
	for _, docID := range docIDs {
		if _, exists := hints[docID]; exists {
			continue
		}
		hints[docID] = hint
		ids = append(ids, docID)
	}
	return hints, ids
}

func filterCandidateSetByPosting(hints map[DocID]queryMatchHint, candidateIDs []DocID, posting []DocID, hint queryMatchHint) (map[DocID]queryMatchHint, []DocID) {
	if len(candidateIDs) == 0 {
		return nil, nil
	}
	if len(posting) == 0 {
		return nil, nil
	}

	filteredIDs := intersectDocIDLists(candidateIDs, posting, 0)
	if len(filteredIDs) == 0 {
		return nil, nil
	}

	filteredHints := make(map[DocID]queryMatchHint, len(filteredIDs))
	for _, docID := range filteredIDs {
		filteredHints[docID] = hints[docID] | hint
	}

	return filteredHints, filteredIDs
}

func (s *rootShard) recallName(plan *queryPlan) recallSource {
	term := normalizeIndexText(plan.nameTerm)
	if term == "" {
		return recallSource{}
	}

	switch utf8Len(term) {
	case 1:
		return recallSource{docIDs: cloneDocIDs(s.namePrefixIndex[term], plan.perClauseLimit), hint: matchHintName}
	case 2:
		return recallSource{docIDs: cloneDocIDs(s.nameBigramIndex[term], plan.perClauseLimit), hint: matchHintName}
	default:
		trigrams := uniqueNgrams(term, 3)
		return recallSource{docIDs: s.intersectIndexTerms(s.nameTrigramIndex, trigrams, plan.perClauseLimit), hint: matchHintName}
	}
}

func (s *rootShard) recallPath(plan *queryPlan) recallSource {
	var sources [][]DocID
	for _, segment := range plan.pathSegments {
		posting := s.pathSegmentIndex[segment]
		if len(posting) == 0 {
			return recallSource{}
		}
		sources = append(sources, posting)
	}

	if utf8Len(plan.pathQuery) >= 3 {
		trigrams := uniqueNgrams(plan.pathQuery, 3)
		posting := s.intersectIndexTerms(s.pathTrigramIndex, trigrams, 0)
		if len(posting) > 0 {
			sources = append(sources, posting)
		}
	}

	if len(sources) == 0 {
		return recallSource{}
	}

	docIDs := intersectDocIDSlices(sources, plan.perClauseLimit)
	return recallSource{docIDs: docIDs, hint: matchHintPath}
}

func (s *rootShard) recallPinyinFull(plan *queryPlan) recallSource {
	term := normalizeIndexText(plan.rawLettersDigits)
	if term == "" {
		return recallSource{}
	}

	switch utf8Len(term) {
	case 1:
		return recallSource{}
	case 2:
		return recallSource{docIDs: cloneDocIDs(s.pinyinFullBigramIndex[term], plan.perClauseLimit), hint: matchHintPinyinFull}
	default:
		trigrams := uniqueNgrams(term, 3)
		return recallSource{docIDs: s.intersectIndexTerms(s.pinyinFullTrigramIndex, trigrams, plan.perClauseLimit), hint: matchHintPinyinFull}
	}
}

func (s *rootShard) recallPinyinInitials(plan *queryPlan) recallSource {
	term := normalizeIndexText(plan.rawLettersDigits)
	if utf8Len(term) < 2 {
		return recallSource{}
	}

	return recallSource{docIDs: s.initialsTrie.collectPrefix(term, plan.perClauseLimit), hint: matchHintPinyinInitial}
}

func (s *rootShard) intersectIndexTerms(index map[string][]DocID, terms []string, limit int) []DocID {
	if len(terms) == 0 {
		return nil
	}

	sources := make([][]DocID, 0, len(terms))
	for _, term := range terms {
		posting := index[term]
		if len(posting) == 0 {
			return nil
		}
		sources = append(sources, posting)
	}

	return intersectDocIDSlices(sources, limit)
}

func intersectDocIDSlices(sources [][]DocID, limit int) []DocID {
	if len(sources) == 0 {
		return nil
	}

	sort.Slice(sources, func(i, j int) bool {
		return len(sources[i]) < len(sources[j])
	})

	result := append([]DocID(nil), sources[0]...)
	for _, source := range sources[1:] {
		result = intersectDocIDLists(result, source, limit)
		if len(result) == 0 {
			return nil
		}
	}

	if limit > 0 && len(result) > limit {
		return result[:limit]
	}

	return result
}

func intersectDocIDLists(a []DocID, b []DocID, limit int) []DocID {
	result := make([]DocID, 0, min(len(a), len(b)))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			result = append(result, a[i])
			if limit > 0 && len(result) >= limit {
				return result
			}
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return result
}

func cloneDocIDs(values []DocID, limit int) []DocID {
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	return append([]DocID(nil), values...)
}

func insertDocID(index map[string][]DocID, key string, docID DocID) {
	if key == "" {
		return
	}

	values := index[key]
	insertAt := sort.Search(len(values), func(i int) bool {
		return values[i] >= docID
	})
	if insertAt < len(values) && values[insertAt] == docID {
		return
	}

	values = append(values, 0)
	copy(values[insertAt+1:], values[insertAt:])
	values[insertAt] = docID
	index[key] = values
}

func removeDocID(index map[string][]DocID, key string, docID DocID) {
	if key == "" {
		return
	}

	values := index[key]
	if len(values) == 0 {
		return
	}

	removeAt := sort.Search(len(values), func(i int) bool {
		return values[i] >= docID
	})
	if removeAt >= len(values) || values[removeAt] != docID {
		return
	}

	values = append(values[:removeAt], values[removeAt+1:]...)
	if len(values) == 0 {
		delete(index, key)
		return
	}
	index[key] = values
}

func newInitialsTrieNode() *initialsTrieNode {
	return &initialsTrieNode{children: map[rune]*initialsTrieNode{}}
}

func (node *initialsTrieNode) insert(value string, docID DocID) {
	if node == nil || value == "" {
		return
	}

	current := node
	for _, r := range value {
		child, ok := current.children[r]
		if !ok {
			child = newInitialsTrieNode()
			current.children[r] = child
		}
		current = child
	}
	insertAt := sort.Search(len(current.docIDs), func(i int) bool {
		return current.docIDs[i] >= docID
	})
	if insertAt < len(current.docIDs) && current.docIDs[insertAt] == docID {
		return
	}
	current.docIDs = append(current.docIDs, 0)
	copy(current.docIDs[insertAt+1:], current.docIDs[insertAt:])
	current.docIDs[insertAt] = docID
}

func (node *initialsTrieNode) remove(value string, docID DocID) bool {
	if node == nil || value == "" {
		return false
	}

	return node.removeRunes([]rune(value), docID)
}

func (node *initialsTrieNode) removeRunes(value []rune, docID DocID) bool {
	if len(value) == 0 {
		removeAt := sort.Search(len(node.docIDs), func(i int) bool {
			return node.docIDs[i] >= docID
		})
		if removeAt < len(node.docIDs) && node.docIDs[removeAt] == docID {
			node.docIDs = append(node.docIDs[:removeAt], node.docIDs[removeAt+1:]...)
		}
		return len(node.docIDs) == 0 && len(node.children) == 0
	}

	child, ok := node.children[value[0]]
	if !ok {
		return false
	}
	if child.removeRunes(value[1:], docID) {
		delete(node.children, value[0])
	}
	return len(node.docIDs) == 0 && len(node.children) == 0
}

func (node *initialsTrieNode) collectPrefix(prefix string, limit int) []DocID {
	if node == nil || prefix == "" {
		return nil
	}

	current := node
	for _, r := range prefix {
		child, ok := current.children[r]
		if !ok {
			return nil
		}
		current = child
	}

	results := make([]DocID, 0)
	current.collect(&results, limit)
	return results
}

func (node *initialsTrieNode) collect(results *[]DocID, limit int) {
	if node == nil {
		return
	}

	for _, docID := range node.docIDs {
		*results = append(*results, docID)
		if limit > 0 && len(*results) >= limit {
			return
		}
	}

	keys := make([]rune, 0, len(node.children))
	for key := range node.children {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	for _, key := range keys {
		node.children[key].collect(results, limit)
		if limit > 0 && len(*results) >= limit {
			return
		}
	}
}

func buildQueryPlan(query SearchQuery) *queryPlan {
	raw := normalizeQuery(query.Raw)
	if raw == "" {
		return nil
	}

	rawLower := normalizeIndexText(raw)
	lettersDigits := keepLettersAndDigits(rawLower)
	pathLike := strings.ContainsAny(raw, `/\`)
	pathQuery := normalizePathQuery(raw)
	pathSegments := splitDirectorySegments(pathQuery)

	nameTerm := rawLower
	wildcardLiterals := buildWildcardLiterals(raw)
	if query.wildcard != nil && len(wildcardLiterals) > 0 {
		nameTerm = longestString(wildcardLiterals)
	}

	extension := extractQueryExtension(raw)
	extensionOnly := extension != "" && !pathLike && strings.TrimSpace(strings.ReplaceAll(rawLower, "*", "")) == "."+extension

	plan := &queryPlan{
		raw:                   raw,
		rawLower:              rawLower,
		rawLettersDigits:      lettersDigits,
		pathLike:              pathLike,
		asciiLettersDigits:    isASCIIAlphaNumeric(lettersDigits) && lettersDigits != "",
		extension:             extension,
		extensionOnly:         extensionOnly,
		pathQuery:             pathQuery,
		pathSegments:          pathSegments,
		wildcardLiterals:      wildcardLiterals,
		nameTerm:              nameTerm,
		perClauseLimit:        defaultPerClauseLimit,
		postIntersectionLimit: defaultPostIntersectionLimit,
		preRerankLimit:        defaultPreRerankLimit,
		shortQueryLength:      utf8Len(rawLower),
	}

	if plan.shortQueryLength <= 2 {
		plan.preRerankLimit = min(defaultPreRerankLimit, 2000)
	}

	if plan.extensionOnly {
		plan.preRerankLimit = 0
	}

	return plan
}

func scoreDocAgainstQuery(query SearchQuery, record docRecord, hints queryMatchHint) (bool, int64) {
	if query.Raw == "" {
		return false, 0
	}

	if query.wildcard != nil {
		return query.wildcard.match(record.Name, record.Path)
	}

	plan := query.plan
	if plan == nil {
		return false, 0
	}

	var (
		matched   bool
		bestScore int64
	)

	updateBest := func(ok bool, score int64) {
		if !ok {
			return
		}
		if !matched || score > bestScore {
			matched = true
			bestScore = score
		}
	}

	nameScore := maybeScoreFuzzy(record.Name, plan.raw, true)
	updateBest(nameScore.matched, nameScore.score+4000)

	pathTarget := record.Path
	if !record.IsDir {
		pathTarget = record.ParentPath
	}
	pathQuery := plan.raw
	if plan.pathLike {
		pathTarget = record.directoryPath()
		pathQuery = plan.pathQuery
	}
	pathMatched, pathScore := scorePathMatch(pathTarget, pathQuery)
	updateBest(pathMatched, pathScore+1500)

	if hints&matchHintPinyinFull != 0 || hints&matchHintPinyinInitial != 0 || !plan.pathLike {
		fullScore := maybeScoreFuzzy(record.PinyinFull, plan.rawLettersDigits, false)
		updateBest(fullScore.matched, fullScore.score+2500)

		initialsScore := maybeScoreFuzzy(record.PinyinInitials, plan.rawLettersDigits, false)
		updateBest(initialsScore.matched, initialsScore.score+2500)
	}

	if !matched && plan.extensionOnly && normalizeExtension(filepath.Ext(record.Name)) == plan.extension {
		return true, 500
	}

	if matched && plan.extension != "" && normalizeExtension(filepath.Ext(record.Name)) == plan.extension {
		bestScore += 500
	}

	return matched, bestScore
}

type fuzzyScore struct {
	matched bool
	score   int64
}

func maybeScoreFuzzy(term string, query string, usePinyin bool) fuzzyScore {
	query = strings.TrimSpace(query)
	term = strings.TrimSpace(term)
	if query == "" || term == "" {
		return fuzzyScore{}
	}

	ok, score := util.IsStringMatchScore(term, query, usePinyin)
	return fuzzyScore{matched: ok, score: score}
}

func normalizeIndexText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToLower(filepath.ToSlash(value))
}

func buildDocRecord(docID DocID, entry EntryRecord) docRecord {
	directoryPath := normalizeIndexText(normalizePath(entry.ParentPath))
	if entry.IsDir {
		directoryPath = normalizeEntryPathKey(entry)
	}

	return docRecord{
		ID:             docID,
		Path:           entry.Path,
		Name:           entry.Name,
		ParentPath:     entry.ParentPath,
		DirectoryPath:  directoryPath,
		IsDir:          entry.IsDir,
		NormalizedName: normalizeIndexText(entry.NormalizedName),
		NormalizedPath: normalizeEntryPathKey(entry),
		PinyinFull:     normalizeIndexText(entry.PinyinFull),
		PinyinInitials: normalizeIndexText(entry.PinyinInitials),
	}
}

func normalizeEntryPathKey(entry EntryRecord) string {
	return normalizeIndexText(entry.NormalizedPath)
}

func normalizeExtension(ext string) string {
	ext = strings.TrimSpace(strings.ToLower(ext))
	ext = strings.TrimPrefix(ext, ".")
	return ext
}

func buildNamePrefixes(value string) []string {
	value = keepLettersAndDigits(value)
	if value == "" {
		return nil
	}

	runes := []rune(value)
	prefixes := make([]string, 0, min(2, len(runes)))
	for size := 1; size <= len(runes) && size <= 2; size++ {
		prefixes = append(prefixes, string(runes[:size]))
	}
	return prefixes
}

func uniqueNgrams(value string, size int) []string {
	value = strings.TrimSpace(value)
	if value == "" || size <= 0 {
		return nil
	}

	runes := []rune(value)
	if len(runes) < size {
		return nil
	}

	seen := map[string]struct{}{}
	grams := make([]string, 0, len(runes)-size+1)
	for i := 0; i+size <= len(runes); i++ {
		gram := string(runes[i : i+size])
		if _, ok := seen[gram]; ok {
			continue
		}
		seen[gram] = struct{}{}
		grams = append(grams, gram)
	}
	return grams
}

func uniqueDirectorySegments(path string) []string {
	parts := splitDirectorySegments(path)
	seen := map[string]struct{}{}
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		segments = append(segments, part)
	}
	return segments
}

func splitDirectorySegments(path string) []string {
	path = normalizePathQuery(path)
	if path == "" {
		return nil
	}

	rawSegments := strings.Split(path, "/")
	segments := make([]string, 0, len(rawSegments))
	for _, segment := range rawSegments {
		segment = strings.TrimSpace(segment)
		if segment == "" || strings.HasSuffix(segment, ":") {
			continue
		}
		segments = append(segments, segment)
	}
	return segments
}

func normalizePathQuery(value string) string {
	value = normalizeIndexText(value)
	value = strings.ReplaceAll(value, `\`, "/")
	return strings.Trim(value, "/")
}

func keepLettersAndDigits(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func buildWildcardLiterals(raw string) []string {
	if !strings.Contains(raw, "*") {
		return nil
	}

	parts := strings.Split(raw, "*")
	literals := make([]string, 0, len(parts))
	for _, part := range parts {
		part = keepLettersAndDigits(normalizeIndexText(part))
		if part == "" {
			continue
		}
		literals = append(literals, part)
	}
	return literals
}

func extractQueryExtension(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if strings.HasPrefix(raw, ".") && !strings.ContainsAny(raw[1:], `/\*`) {
		return normalizeExtension(raw)
	}

	if strings.Contains(raw, "*") {
		index := strings.LastIndex(raw, ".")
		if index >= 0 && index < len(raw)-1 && !strings.ContainsAny(raw[index+1:], `/*\`) {
			return normalizeExtension(raw[index:])
		}
	}

	return ""
}

func longestString(values []string) string {
	longest := ""
	for _, value := range values {
		if len(value) > len(longest) {
			longest = value
		}
	}
	return longest
}

func isASCIIAlphaNumeric(value string) bool {
	if value == "" {
		return false
	}

	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
}

func utf8Len(value string) int {
	return len([]rune(value))
}
