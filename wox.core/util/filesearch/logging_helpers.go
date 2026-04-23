package filesearch

import (
	"context"
	"fmt"
	"strings"
	"wox/util"
)

const (
	maxLoggedPaths                               = 8
	maxLoggedRoots                               = 5
	slowFilesearchProviderQueryThresholdMs int64 = 40
	slowFilesearchAggregationThresholdMs   int64 = 10
	slowFilesearchEngineQueryThresholdMs   int64 = 60
	slowFilesearchSearchOnceTimeoutMs      int64 = 200
)

func summarizeLogPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "<empty>"
	}
	return path
}

func summarizeLogPaths(paths []string) string {
	if len(paths) == 0 {
		return "[]"
	}

	limit := len(paths)
	if limit > maxLoggedPaths {
		limit = maxLoggedPaths
	}

	visible := make([]string, 0, limit)
	for _, path := range paths[:limit] {
		visible = append(visible, summarizeLogPath(path))
	}

	if len(paths) <= limit {
		return "[" + strings.Join(visible, ", ") + "]"
	}

	return fmt.Sprintf("[%s, ... +%d more]", strings.Join(visible, ", "), len(paths)-limit)
}

func summarizeDirtySignal(signal DirtySignal) string {
	return fmt.Sprintf(
		"kind=%s root=%s trace=%s path=%s path_is_dir=%t path_type_known=%t",
		signal.Kind,
		signal.RootID,
		strings.TrimSpace(signal.TraceID),
		summarizeLogPath(signal.Path),
		signal.PathIsDir,
		signal.PathTypeKnown,
	)
}

func contextWithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return ctx
	}
	if util.GetContextTraceId(ctx) == traceID {
		return ctx
	}
	return util.NewTraceContextWith(traceID)
}

func logProviderSearchResponse(ctx context.Context, query SearchQuery, providerName string, elapsedMs int64, aggregationElapsedMs int64, candidateCount int, resultCount int, changed bool, err error) {
	status := "ok"
	if err != nil {
		if errorsIsCanceled(err) {
			status = "canceled"
		} else {
			status = "error"
		}
	}

	msg := fmt.Sprintf(
		"filesearch provider query: provider=%s query=%q elapsed=%dms aggregate=%dms candidates=%d results=%d changed=%v status=%s",
		providerName,
		query.Raw,
		elapsedMs,
		aggregationElapsedMs,
		candidateCount,
		resultCount,
		changed,
		status,
	)
	if err != nil && !errorsIsCanceled(err) {
		msg += " error=" + err.Error()
	}

	if err != nil && !errorsIsCanceled(err) {
		util.GetLogger().Warn(ctx, msg)
		return
	}

	if elapsedMs >= slowFilesearchProviderQueryThresholdMs || aggregationElapsedMs >= slowFilesearchAggregationThresholdMs {
		util.GetLogger().Info(ctx, "filesearch slow provider query: "+msg)
		return
	}

	util.GetLogger().Debug(ctx, msg)
}

func logEngineSearchCompletion(ctx context.Context, query SearchQuery, elapsedMs int64, providerCount int, updateCount int, resultCount int) {
	msg := fmt.Sprintf(
		"filesearch engine query complete: query=%q elapsed=%dms providers=%d updates=%d results=%d",
		query.Raw,
		elapsedMs,
		providerCount,
		updateCount,
		resultCount,
	)
	if elapsedMs >= slowFilesearchEngineQueryThresholdMs {
		util.GetLogger().Info(ctx, "filesearch slow engine query: "+msg)
		return
	}

	util.GetLogger().Debug(ctx, msg)
}

func logSearchOnceWait(ctx context.Context, query SearchQuery, elapsedMs int64, timedOut bool, resultCount int) {
	msg := fmt.Sprintf(
		"filesearch search_once wait: query=%q elapsed=%dms timeout=%v results=%d",
		query.Raw,
		elapsedMs,
		timedOut,
		resultCount,
	)
	if timedOut {
		util.GetLogger().Info(ctx, "filesearch partial query return: "+msg)
		return
	}

	if elapsedMs >= slowFilesearchSearchOnceTimeoutMs {
		util.GetLogger().Info(ctx, "filesearch slow search_once wait: "+msg)
		return
	}

	util.GetLogger().Debug(ctx, msg)
}

func logLocalIndexSnapshot(ctx context.Context, stage string, snapshot queryIndexSnapshot, info bool) {
	summary := formatLocalIndexSnapshotSummary(stage, snapshot)
	topRoots := formatLocalIndexTopRoots(stage, snapshot)
	if info {
		util.GetLogger().Info(ctx, summary)
		if topRoots != "" {
			util.GetLogger().Info(ctx, topRoots)
		}
		return
	}

	util.GetLogger().Debug(ctx, summary)
	if topRoots != "" {
		util.GetLogger().Debug(ctx, topRoots)
	}
}

func logSQLiteIndexSnapshot(ctx context.Context, stage string, snapshot sqliteIndexSnapshot, info bool) {
	summary := formatSQLiteIndexSnapshotSummary(stage, snapshot)
	topRoots := formatSQLiteIndexTopRoots(stage, snapshot)
	if info {
		util.GetLogger().Info(ctx, summary)
		if topRoots != "" {
			util.GetLogger().Info(ctx, topRoots)
		}
		return
	}

	util.GetLogger().Debug(ctx, summary)
	if topRoots != "" {
		util.GetLogger().Debug(ctx, topRoots)
	}
}

func logFilesearchRunStage(ctx context.Context, kind RunKind, stage RunStage, root RootRecord, job Job, rootIndex int, rootTotal int, current int64, total int64) {
	msg := fmt.Sprintf(
		"filesearch run stage: kind=%s stage=%s root=%s root_path=%s root_index=%d/%d job=%s job_kind=%s scope=%s progress=%d/%d",
		kind,
		stage,
		root.ID,
		summarizeLogPath(root.Path),
		rootIndex,
		rootTotal,
		strings.TrimSpace(job.JobID),
		job.Kind,
		summarizeLogPath(job.ScopePath),
		current,
		total,
	)

	switch stage {
	case RunStagePlanning, RunStagePreScan, RunStageFinalizing:
		util.GetLogger().Info(ctx, msg)
	default:
		util.GetLogger().Debug(ctx, msg)
	}
}

func formatLocalIndexSnapshotSummary(stage string, snapshot queryIndexSnapshot) string {
	return fmt.Sprintf(
		"filesearch index snapshot: stage=%s roots=%d docs=%d live_doc_records=%d path_to_doc_keys=%d freed_doc_ids=%d extension_keys=%d extension_refs=%d name_prefix_keys=%d name_prefix_refs=%d name_bigram_keys=%d name_bigram_refs=%d name_trigram_keys=%d name_trigram_refs=%d path_segment_keys=%d path_segment_refs=%d path_trigram_keys=%d path_trigram_refs=%d pinyin_full_bigram_keys=%d pinyin_full_bigram_refs=%d pinyin_full_trigram_keys=%d pinyin_full_trigram_refs=%d pinyin_initial_trie_nodes=%d pinyin_initial_posting_refs=%d doc_bytes_est=%d posting_bytes_est=%d path_key_bytes_est=%d trie_bytes_est=%d total_bytes_est=%d",
		strings.TrimSpace(stage),
		snapshot.RootCount,
		snapshot.DocCount,
		snapshot.LiveDocRecords,
		snapshot.PathToDocKeyCount,
		snapshot.FreedDocIDCount,
		snapshot.Extension.PostingKeyCount,
		snapshot.Extension.PostingRefCount,
		snapshot.NamePrefix.PostingKeyCount,
		snapshot.NamePrefix.PostingRefCount,
		snapshot.NameBigram.PostingKeyCount,
		snapshot.NameBigram.PostingRefCount,
		snapshot.NameTrigram.PostingKeyCount,
		snapshot.NameTrigram.PostingRefCount,
		snapshot.PathSegment.PostingKeyCount,
		snapshot.PathSegment.PostingRefCount,
		snapshot.PathTrigram.PostingKeyCount,
		snapshot.PathTrigram.PostingRefCount,
		snapshot.PinyinFullBigram.PostingKeyCount,
		snapshot.PinyinFullBigram.PostingRefCount,
		snapshot.PinyinFullTrigram.PostingKeyCount,
		snapshot.PinyinFullTrigram.PostingRefCount,
		snapshot.PinyinInitials.NodeCount,
		snapshot.PinyinInitials.PostingRefCount,
		snapshot.DocBytesEstimate,
		snapshot.PostingBytesEstimate,
		snapshot.PathKeyBytesEstimate,
		snapshot.TrieBytesEstimate,
		snapshot.TotalBytesEstimate,
	)
}

func formatLocalIndexTopRoots(stage string, snapshot queryIndexSnapshot) string {
	if len(snapshot.TopRoots) == 0 {
		return ""
	}

	limit := len(snapshot.TopRoots)
	if limit > maxLoggedRoots {
		limit = maxLoggedRoots
	}

	roots := make([]string, 0, limit)
	for _, root := range snapshot.TopRoots[:limit] {
		roots = append(roots, fmt.Sprintf(
			"%s(docs=%d,total_bytes_est=%d,path_keys=%d,freed_doc_ids=%d)",
			summarizeLogPath(root.RootID),
			root.DocCount,
			root.TotalBytesEstimate,
			root.PathToDocKeyCount,
			root.FreedDocIDCount,
		))
	}

	return fmt.Sprintf(
		"filesearch index snapshot roots: stage=%s top_roots=[%s]",
		strings.TrimSpace(stage),
		strings.Join(roots, ", "),
	)
}

func formatSQLiteIndexSnapshotSummary(stage string, snapshot sqliteIndexSnapshot) string {
	return fmt.Sprintf(
		"filesearch sqlite snapshot: stage=%s roots=%d entries=%d bigram_rows=%d name_fts_vocab=%d path_fts_vocab=%d pinyin_full_fts_vocab=%d initials_fts_vocab=%d fact_bytes_est=%d fts_source_bytes_est=%d bigram_bytes_est=%d total_bytes_est=%d db_main_file_bytes=%d db_wal_file_bytes=%d db_shm_file_bytes=%d db_total_file_bytes=%d",
		strings.TrimSpace(stage),
		snapshot.RootCount,
		snapshot.EntryCount,
		snapshot.BigramRowCount,
		snapshot.NameFTSVocab,
		snapshot.PathFTSVocab,
		snapshot.PinyinFullFTSVocab,
		snapshot.InitialsFTSVocab,
		snapshot.FactBytesEstimate,
		snapshot.FTSSourceBytesEstimate,
		snapshot.BigramBytesEstimate,
		snapshot.TotalBytesEstimate,
		snapshot.DBMainFileBytes,
		snapshot.DBWALFileBytes,
		snapshot.DBSHMFileBytes,
		snapshot.DBTotalFileBytes,
	)
}

func formatSQLiteIndexTopRoots(stage string, snapshot sqliteIndexSnapshot) string {
	if len(snapshot.TopRoots) == 0 {
		return ""
	}

	visible := make([]string, 0, min(len(snapshot.TopRoots), maxLoggedRoots))
	for _, root := range snapshot.TopRoots[:min(len(snapshot.TopRoots), maxLoggedRoots)] {
		visible = append(visible, fmt.Sprintf(
			"%s(docs=%d,bigram_rows=%d,total_bytes_est=%d,fact_bytes_est=%d,fts_source_bytes_est=%d,bigram_bytes_est=%d)",
			summarizeLogPath(root.Path),
			root.Docs,
			root.BigramRows,
			root.TotalBytesEstimate,
			root.FactBytesEstimate,
			root.FTSSourceBytesEstimate,
			root.BigramBytesEstimate,
		))
	}
	return fmt.Sprintf(
		"filesearch sqlite snapshot roots: stage=%s top_roots=[%s]",
		strings.TrimSpace(stage),
		strings.Join(visible, ", "),
	)
}
