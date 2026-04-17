package filesearch

import (
	"context"
	"fmt"
	"strings"
	"wox/util"
)

const (
	maxLoggedPaths                          = 8
	slowFilesearchProviderQueryThresholdMs  int64 = 40
	slowFilesearchAggregationThresholdMs    int64 = 10
	slowFilesearchEngineQueryThresholdMs    int64 = 60
	slowFilesearchSearchOnceTimeoutMs       int64 = 200
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
