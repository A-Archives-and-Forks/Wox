package filesearch

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	CapturedAt         string                 `json:"captured_at"`
	Root               realIndexRootArtifact  `json:"root"`
	TotalElapsedMillis int64                  `json:"total_elapsed_millis"`
	StageMetrics       []realIndexStageMetric `json:"stage_metrics"`
	Transitions        []realIndexTransition  `json:"transitions"`
	SQLiteSnapshot     string                 `json:"sqlite_snapshot"`
	SQLiteTopRoots     string                 `json:"sqlite_top_roots"`
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
