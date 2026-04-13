package filesearch

import (
	"context"
	"fmt"
	"strings"
	"wox/util"
)

const maxLoggedPaths = 8

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
