package filesearch

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"wox/util"
)

const (
	actualIndexCaptureEnv      = "WOX_CAPTURE_FILESEARCH_REAL_INDEX"
	actualIndexArtifactPathEnv = "WOX_FILESEARCH_REAL_INDEX_ARTIFACT_PATH"
	actualIndexRootPath        = `C:\dev`
	actualIndexTimeout         = 30 * time.Minute
)

var (
	actualIndexCaptureFlag      = flag.Bool("filesearch-real-index", false, "capture a real filesearch index baseline for C:\\dev")
	actualIndexArtifactPathFlag = flag.String("filesearch-real-index-artifact", "", "write the real filesearch index baseline artifact to this path")
)

type realIndexStageMetric struct {
	Stage           string `json:"stage"`
	ElapsedMillis   int64  `json:"elapsed_millis"`
	TransitionCount int    `json:"transition_count"`
}

type realIndexTransition struct {
	OffsetMillis       int64  `json:"offset_millis"`
	Stage              string `json:"stage"`
	RunStatus          string `json:"run_status"`
	ActiveRootPath     string `json:"active_root_path"`
	ActiveScopePath    string `json:"active_scope_path"`
	RunProgressCurrent int64  `json:"run_progress_current"`
	RunProgressTotal   int64  `json:"run_progress_total"`
}

type realIndexRootArtifact struct {
	Path            string `json:"path"`
	Status          string `json:"status"`
	LastFullScanAt  int64  `json:"last_full_scan_at"`
	ProgressCurrent int64  `json:"progress_current"`
	ProgressTotal   int64  `json:"progress_total"`
	DirectoryCount  int    `json:"directory_count"`
	EntryCount      int    `json:"entry_count"`
}

type realIndexArtifact struct {
	CapturedAt         string                  `json:"captured_at"`
	Root               realIndexRootArtifact   `json:"root"`
	TotalElapsedMillis int64                   `json:"total_elapsed_millis"`
	StageMetrics       []realIndexStageMetric  `json:"stage_metrics"`
	Transitions        []realIndexTransition   `json:"transitions"`
	SQLiteSnapshot     string                  `json:"sqlite_snapshot"`
	SQLiteTopRoots     string                  `json:"sqlite_top_roots"`
	ExecutionStats     realIndexExecutionStats `json:"execution_stats"`
}

type realIndexExecutionStats struct {
	JobCount                   int                  `json:"job_count"`
	SubtreeApplyTotalP50Millis int64                `json:"subtree_apply_total_p50_millis"`
	SubtreeApplyTotalP95Millis int64                `json:"subtree_apply_total_p95_millis"`
	ApplySnapshotP50Millis     int64                `json:"apply_snapshot_p50_millis"`
	ApplySnapshotP95Millis     int64                `json:"apply_snapshot_p95_millis"`
	SlowestScopes              []realIndexSlowScope `json:"slowest_scopes"`
}

type realIndexSlowScope struct {
	Scope         string `json:"scope"`
	ElapsedMillis int64  `json:"elapsed_millis"`
}

type realIndexTimelineEvent struct {
	recordedAt         time.Time
	stage              RunStage
	runStatus          RunStatus
	activeRootPath     string
	activeScopePath    string
	runProgressCurrent int64
	runProgressTotal   int64
}

var (
	realIndexApplySnapshotPattern = regexp.MustCompile(`phase=apply_snapshot elapsed=(\d+)ms .* scope=(.+) units=\d+$`)
	realIndexSubtreeApplyPattern  = regexp.MustCompile(`operation=subtree_apply_total scope=.* elapsed=(\d+)ms work_count=\d+$`)
)

func TestSummarizeRealIndexExecutionLog(t *testing.T) {
	logContent := strings.Join([]string{
		"2026-04-23 19:14:16.293 G0000008 [DBG] [Wox] filesearch sqlite maintenance: operation=subtree_apply_total scope=batches=1 elapsed=19ms work_count=1",
		"2026-04-23 19:14:16.293 G0000008 [DBG] [Wox] filesearch job phase: phase=apply_snapshot elapsed=19ms root=real-index-c-dev root_path=C:\\dev job=job-1 job_kind=subtree scope=C:\\dev\\scope-a units=64",
		"2026-04-23 19:14:16.309 G0000008 [DBG] [Wox] filesearch sqlite maintenance: operation=subtree_apply_total scope=batches=1 elapsed=15ms work_count=1",
		"2026-04-23 19:14:16.309 G0000008 [DBG] [Wox] filesearch job phase: phase=apply_snapshot elapsed=15ms root=real-index-c-dev root_path=C:\\dev job=job-2 job_kind=subtree scope=C:\\dev\\scope-b units=16",
		"2026-04-23 19:14:16.329 G0000008 [DBG] [Wox] filesearch sqlite maintenance: operation=subtree_apply_total scope=batches=1 elapsed=27ms work_count=1",
		"2026-04-23 19:14:16.329 G0000008 [DBG] [Wox] filesearch job phase: phase=apply_snapshot elapsed=27ms root=real-index-c-dev root_path=C:\\dev job=job-3 job_kind=subtree scope=C:\\dev\\scope-c units=18",
		"2026-04-23 19:14:16.349 G0000008 [DBG] [Wox] filesearch job phase: phase=apply_snapshot elapsed=20ms root=real-index-c-dev root_path=C:\\dev job=job-4 job_kind=subtree scope=C:\\dev\\scope-a units=36",
	}, "\n")

	stats := summarizeRealIndexExecutionLog(logContent)
	if stats.JobCount != 4 {
		t.Fatalf("expected 4 jobs, got %d", stats.JobCount)
	}
	if stats.SubtreeApplyTotalP50Millis != 19 {
		t.Fatalf("expected subtree apply p50 19ms, got %d", stats.SubtreeApplyTotalP50Millis)
	}
	if stats.SubtreeApplyTotalP95Millis != 27 {
		t.Fatalf("expected subtree apply p95 27ms, got %d", stats.SubtreeApplyTotalP95Millis)
	}
	if stats.ApplySnapshotP50Millis != 20 {
		t.Fatalf("expected apply snapshot p50 20ms, got %d", stats.ApplySnapshotP50Millis)
	}
	if stats.ApplySnapshotP95Millis != 27 {
		t.Fatalf("expected apply snapshot p95 27ms, got %d", stats.ApplySnapshotP95Millis)
	}
	if len(stats.SlowestScopes) != 3 {
		t.Fatalf("expected 3 unique slow scopes, got %d", len(stats.SlowestScopes))
	}
	if stats.SlowestScopes[0].Scope != `C:\dev\scope-c` || stats.SlowestScopes[0].ElapsedMillis != 27 {
		t.Fatalf("expected slowest scope C:\\dev\\scope-c at 27ms, got %#v", stats.SlowestScopes[0])
	}
	if stats.SlowestScopes[1].Scope != `C:\dev\scope-a` || stats.SlowestScopes[1].ElapsedMillis != 20 {
		t.Fatalf("expected second scope C:\\dev\\scope-a at 20ms, got %#v", stats.SlowestScopes[1])
	}
}

func TestCaptureFileSearchRealIndexForWindowsDevRoot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("real index capture only runs on Windows")
	}
	if !shouldCaptureRealIndex() {
		t.Skip("set WOX_CAPTURE_FILESEARCH_REAL_INDEX=1 or pass -args -filesearch-real-index to capture the real C:\\dev indexing baseline")
	}

	rootInfo, err := os.Stat(actualIndexRootPath)
	if err != nil {
		t.Skipf("skip real index capture because %q is unavailable: %v", actualIndexRootPath, err)
	}
	if !rootInfo.IsDir() {
		t.Skipf("skip real index capture because %q is not a directory", actualIndexRootPath)
	}

	db, baseCtx := openTestFileSearchDB(t)
	scanCtx, cancel := context.WithTimeout(baseCtx, actualIndexTimeout)
	defer cancel()
	t.Logf("real index test log directory: %s", util.GetLocation().GetLogDirectory())

	rootPath := filepath.Clean(actualIndexRootPath)
	now := time.Now().UnixMilli()
	root := RootRecord{
		ID:        "real-index-c-dev",
		Path:      rootPath,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mustInsertRoot(t, scanCtx, db, root)

	scanner := NewScanner(db, nil)
	engine := &Engine{db: db, scanner: scanner}

	var (
		timelineMu sync.Mutex
		timeline   []realIndexTimelineEvent
		lastKey    string
	)

	// Temp-dir fixtures keep full-scan tests deterministic, but they do not
	// expose the planner churn, SQLite write volume, and stage balance of a real
	// workstation-sized tree. This opt-in test records those production-shaped
	// costs without forcing CI or routine local runs to index C:\dev every time.
	scanner.SetStateChangeHandler(func(changeCtx context.Context) {
		status, err := engine.GetStatus(changeCtx)
		if err != nil {
			t.Fatalf("get status during real index capture: %v", err)
		}

		event := realIndexTimelineEvent{
			recordedAt:         time.Now(),
			stage:              status.ActiveStage,
			runStatus:          status.ActiveRunStatus,
			activeRootPath:     status.ActiveRootPath,
			activeScopePath:    status.ActiveScopePath,
			runProgressCurrent: status.RunProgressCurrent,
			runProgressTotal:   status.RunProgressTotal,
		}

		key := fmt.Sprintf(
			"%s|%s|%s|%s",
			event.stage,
			event.runStatus,
			strings.TrimSpace(event.activeRootPath),
			strings.TrimSpace(event.activeScopePath),
		)

		timelineMu.Lock()
		if key == lastKey {
			timelineMu.Unlock()
			return
		}
		lastKey = key
		timeline = append(timeline, event)
		timelineMu.Unlock()
	})

	scanStartedAt := time.Now()
	scanner.scanAllRoots(scanCtx)
	scanFinishedAt := time.Now()

	rootAfter, err := db.FindRootByID(scanCtx, root.ID)
	if err != nil {
		t.Fatalf("find root after real index capture: %v", err)
	}
	if rootAfter == nil {
		t.Fatalf("expected captured root %q to remain persisted", root.ID)
	}

	directoryCount, err := db.CountDirectoriesByRoot(scanCtx, root.ID)
	if err != nil {
		t.Fatalf("count directories after real index capture: %v", err)
	}
	entries, err := db.ListEntriesByRoot(scanCtx, root.ID)
	if err != nil {
		t.Fatalf("list entries after real index capture: %v", err)
	}

	sqliteSnapshot, err := db.SearchIndexSnapshot(scanCtx)
	if err != nil {
		t.Fatalf("capture sqlite snapshot after real index capture: %v", err)
	}
	executionStats := loadRealIndexExecutionStats(t)

	timelineMu.Lock()
	artifact := realIndexArtifact{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Root: realIndexRootArtifact{
			Path:            rootAfter.Path,
			Status:          string(rootAfter.Status),
			LastFullScanAt:  rootAfter.LastFullScanAt,
			ProgressCurrent: rootAfter.ProgressCurrent,
			ProgressTotal:   rootAfter.ProgressTotal,
			DirectoryCount:  directoryCount,
			EntryCount:      len(entries),
		},
		TotalElapsedMillis: scanFinishedAt.Sub(scanStartedAt).Milliseconds(),
		StageMetrics:       buildRealIndexStageMetrics(timeline, scanFinishedAt),
		Transitions:        buildRealIndexTransitions(timeline, scanStartedAt),
		SQLiteSnapshot:     formatSQLiteIndexSnapshotSummary("actual_root", sqliteSnapshot),
		SQLiteTopRoots:     formatSQLiteIndexTopRoots("actual_root", sqliteSnapshot),
		ExecutionStats:     executionStats,
	}
	timelineMu.Unlock()

	writeRealIndexArtifact(t, artifact)
}

func shouldCaptureRealIndex() bool {
	if *actualIndexCaptureFlag {
		return true
	}

	// WSL-launched Windows `go.exe` runs the test binary on the Windows side, but
	// the shell-to-process environment bridge can drop opt-in test variables.
	// Accepting a `go test -args` flag keeps the real-index capture runnable from
	// either shell without weakening the default skip guard for CI.
	return strings.TrimSpace(os.Getenv(actualIndexCaptureEnv)) == "1"
}

func loadRealIndexExecutionStats(t *testing.T) realIndexExecutionStats {
	t.Helper()

	logPath := filepath.Join(util.GetLocation().GetLogDirectory(), "log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read real index log %q: %v", logPath, err)
	}

	// The first version of this real-root baseline only emitted stage timelines
	// and SQLite snapshots, which still left the real hotspot hidden inside the
	// giant debug log. Parsing the existing log file here keeps the feature
	// test-only while turning one long trace into a compact performance summary.
	return summarizeRealIndexExecutionLog(string(content))
}

func buildRealIndexStageMetrics(timeline []realIndexTimelineEvent, scanFinishedAt time.Time) []realIndexStageMetric {
	if len(timeline) == 0 {
		return nil
	}

	stageTotals := map[string]int64{}
	stageTransitions := map[string]int{}
	stageOrder := []string{}

	for index, event := range timeline {
		stage := string(event.stage)
		if strings.TrimSpace(stage) == "" {
			continue
		}
		if _, seen := stageTransitions[stage]; !seen {
			stageOrder = append(stageOrder, stage)
		}
		stageTransitions[stage]++

		nextAt := scanFinishedAt
		if index+1 < len(timeline) {
			nextAt = timeline[index+1].recordedAt
		}
		stageTotals[stage] += nextAt.Sub(event.recordedAt).Milliseconds()
	}

	metrics := make([]realIndexStageMetric, 0, len(stageOrder))
	for _, stage := range stageOrder {
		metrics = append(metrics, realIndexStageMetric{
			Stage:           stage,
			ElapsedMillis:   stageTotals[stage],
			TransitionCount: stageTransitions[stage],
		})
	}

	return metrics
}

func buildRealIndexTransitions(timeline []realIndexTimelineEvent, scanStartedAt time.Time) []realIndexTransition {
	if len(timeline) == 0 {
		return nil
	}

	transitions := make([]realIndexTransition, 0, len(timeline))
	for _, event := range timeline {
		transitions = append(transitions, realIndexTransition{
			OffsetMillis:       event.recordedAt.Sub(scanStartedAt).Milliseconds(),
			Stage:              string(event.stage),
			RunStatus:          string(event.runStatus),
			ActiveRootPath:     event.activeRootPath,
			ActiveScopePath:    event.activeScopePath,
			RunProgressCurrent: event.runProgressCurrent,
			RunProgressTotal:   event.runProgressTotal,
		})
	}
	return transitions
}

func writeRealIndexArtifact(t *testing.T, artifact realIndexArtifact) {
	t.Helper()

	payload, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatalf("marshal real index artifact: %v", err)
	}

	// The artifact is machine-specific, so only write a file when the caller asks
	// for one. Logging the JSON by default keeps the test useful for quick local
	// measurements without leaving workstation-specific output in the repo tree.
	artifactPath := realIndexArtifactPath()
	if artifactPath != "" {
		if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
			t.Fatalf("create real index artifact directory: %v", err)
		}
		if err := os.WriteFile(artifactPath, payload, 0o644); err != nil {
			t.Fatalf("write real index artifact %q: %v", artifactPath, err)
		}
		t.Logf("real index artifact written to %s", artifactPath)
	}

	t.Log(string(payload))
}

func realIndexArtifactPath() string {
	if path := strings.TrimSpace(*actualIndexArtifactPathFlag); path != "" {
		return path
	}
	return strings.TrimSpace(os.Getenv(actualIndexArtifactPathEnv))
}

func summarizeRealIndexExecutionLog(logContent string) realIndexExecutionStats {
	lines := strings.Split(logContent, "\n")
	subtreeApplyTotals := make([]int64, 0)
	applySnapshots := make([]int64, 0)
	slowestScopeByPath := map[string]int64{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if matches := realIndexSubtreeApplyPattern.FindStringSubmatch(line); len(matches) == 2 {
			subtreeApplyTotals = append(subtreeApplyTotals, mustParseRealIndexMillis(matches[1]))
		}

		if matches := realIndexApplySnapshotPattern.FindStringSubmatch(line); len(matches) == 3 {
			elapsed := mustParseRealIndexMillis(matches[1])
			scope := strings.TrimSpace(matches[2])
			applySnapshots = append(applySnapshots, elapsed)
			if elapsed > slowestScopeByPath[scope] {
				slowestScopeByPath[scope] = elapsed
			}
		}
	}

	stats := realIndexExecutionStats{
		JobCount:                   len(applySnapshots),
		SubtreeApplyTotalP50Millis: percentileMillis(subtreeApplyTotals, 0.50),
		SubtreeApplyTotalP95Millis: percentileMillis(subtreeApplyTotals, 0.95),
		ApplySnapshotP50Millis:     percentileMillis(applySnapshots, 0.50),
		ApplySnapshotP95Millis:     percentileMillis(applySnapshots, 0.95),
	}

	slowestScopes := make([]realIndexSlowScope, 0, len(slowestScopeByPath))
	for scope, elapsed := range slowestScopeByPath {
		slowestScopes = append(slowestScopes, realIndexSlowScope{
			Scope:         scope,
			ElapsedMillis: elapsed,
		})
	}

	sort.Slice(slowestScopes, func(left int, right int) bool {
		if slowestScopes[left].ElapsedMillis == slowestScopes[right].ElapsedMillis {
			return slowestScopes[left].Scope < slowestScopes[right].Scope
		}
		return slowestScopes[left].ElapsedMillis > slowestScopes[right].ElapsedMillis
	})
	if len(slowestScopes) > 20 {
		slowestScopes = slowestScopes[:20]
	}
	stats.SlowestScopes = slowestScopes

	return stats
}

func mustParseRealIndexMillis(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		panic(fmt.Sprintf("parse real index millis %q: %v", value, err))
	}
	return parsed
}

func percentileMillis(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(left int, right int) bool {
		return sorted[left] < sorted[right]
	})

	index := int(math.Round(percentile * float64(len(sorted)-1)))
	return sorted[index]
}
