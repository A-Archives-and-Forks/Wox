package filesearch

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestJobExecutorOrderIsStable(t *testing.T) {
	rootOnePath := filepath.Join(t.TempDir(), "root-one")
	rootTwoPath := filepath.Join(t.TempDir(), "root-two")
	mustWriteTestFile(t, filepath.Join(rootOnePath, "alpha.txt"), "alpha")
	mustWriteTestFile(t, filepath.Join(rootOnePath, "beta.txt"), "beta")
	mustWriteTestFile(t, filepath.Join(rootOnePath, "gamma.txt"), "gamma")
	mustWriteTestFile(t, filepath.Join(rootTwoPath, "nested", "gamma.txt"), "gamma")

	rootOne := testRunPlannerRootRecord("root-one", rootOnePath)
	rootTwo := testRunPlannerRootRecord("root-two", rootTwoPath)
	plan := RunPlan{
		PlanID: "plan-order",
		RunID:  "run-order",
		Kind:   RunKindFull,
		RootPlans: []RootPlan{{
			RootID:   rootOne.ID,
			RootPath: rootOne.Path,
		}, {
			RootID:   rootTwo.ID,
			RootPath: rootTwo.Path,
		}},
		Jobs: []Job{{
			JobID:             "job-subtree",
			RootID:            rootTwo.ID,
			RootPath:          rootTwo.Path,
			ScopePath:         rootTwo.Path,
			Kind:              JobKindSubtree,
			PlannedScanUnits:  2,
			PlannedWriteUnits: 2,
			PlannedTotalUnits: 4,
			Status:            JobStatusPending,
			OrderIndex:        11,
		}, {
			JobID:                 "job-direct-files-0",
			RootID:                rootOne.ID,
			RootPath:              rootOne.Path,
			ScopePath:             rootOne.Path,
			Kind:                  JobKindDirectFiles,
			DirectFileChunkIndex:  0,
			DirectFileChunkOffset: 0,
			DirectFileChunkCount:  2,
			PlannedScanUnits:      3,
			PlannedWriteUnits:     3,
			PlannedTotalUnits:     6,
			Status:                JobStatusPending,
			OrderIndex:            3,
		}, {
			JobID:                 "job-direct-files-1",
			RootID:                rootOne.ID,
			RootPath:              rootOne.Path,
			ScopePath:             rootOne.Path,
			Kind:                  JobKindDirectFiles,
			DirectFileChunkIndex:  1,
			DirectFileChunkOffset: 2,
			DirectFileChunkCount:  1,
			PlannedScanUnits:      1,
			PlannedWriteUnits:     1,
			PlannedTotalUnits:     2,
			Status:                JobStatusPending,
			OrderIndex:            7,
		}, {
			JobID:             "job-finalize",
			RootID:            rootTwo.ID,
			RootPath:          rootTwo.Path,
			ScopePath:         rootTwo.Path,
			Kind:              JobKindFinalizeRoot,
			PlannedWriteUnits: 1,
			PlannedTotalUnits: 1,
			Status:            JobStatusPending,
			OrderIndex:        19,
		}},
		TotalWorkUnits: 13,
	}

	builder := NewSnapshotBuilder(newPolicyState(Policy{}))
	chunkZeroBatch, err := builder.BuildDirectFilesJobSnapshot(context.Background(), rootOne, plan.Jobs[1])
	if err != nil {
		t.Fatalf("build chunk zero snapshot: %v", err)
	}
	chunkOneBatch, err := builder.BuildDirectFilesJobSnapshot(context.Background(), rootOne, plan.Jobs[2])
	if err != nil {
		t.Fatalf("build chunk one snapshot: %v", err)
	}

	if got, want := snapshotEntryPaths(chunkZeroBatch), []string{
		rootOnePath,
		filepath.Join(rootOnePath, "alpha.txt"),
		filepath.Join(rootOnePath, "beta.txt"),
	}; !equalPaths(got, want) {
		t.Fatalf("unexpected chunk zero entry paths: got %v want %v", got, want)
	}
	if got, want := snapshotEntryPaths(chunkOneBatch), []string{
		filepath.Join(rootOnePath, "gamma.txt"),
	}; !equalPaths(got, want) {
		t.Fatalf("unexpected chunk one entry paths: got %v want %v", got, want)
	}
	if got, want := snapshotDirectoryPaths(chunkZeroBatch), []string{rootOnePath}; !equalPaths(got, want) {
		t.Fatalf("unexpected chunk zero directory paths: got %v want %v", got, want)
	}
	if got := len(chunkOneBatch.Directories); got != 0 {
		t.Fatalf("expected chunk one to skip directory ownership, got %d directories", got)
	}

	executor := NewJobExecutor(builder)
	completedOrder := make([]int, 0, len(plan.Jobs))
	snapshots := make([]StatusSnapshot, 0, len(plan.Jobs)*2+1)
	run, executedJobs, err := executor.ExecuteRun(context.Background(), plan, []RootRecord{rootOne, rootTwo}, func(snapshot StatusSnapshot, job Job) {
		snapshots = append(snapshots, snapshot)
		if job.Status == JobStatusCompleted {
			completedOrder = append(completedOrder, job.OrderIndex)
		}
	})
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}

	if got, want := run.Status, RunStatusCompleted; got != want {
		t.Fatalf("unexpected run status: got %s want %s", got, want)
	}

	gotOrder := completedOrder
	wantOrder := []int{11, 3, 7, 19}
	if len(gotOrder) != len(wantOrder) {
		t.Fatalf("unexpected completed job count: got %d want %d", len(gotOrder), len(wantOrder))
	}
	for index := range wantOrder {
		if gotOrder[index] != wantOrder[index] {
			t.Fatalf("unexpected completed order at %d: got %v want %v", index, gotOrder, wantOrder)
		}
		if executedJobs[index].OrderIndex != wantOrder[index] {
			t.Fatalf("unexpected stored order index at %d: got %d want %d", index, executedJobs[index].OrderIndex, wantOrder[index])
		}
		if executedJobs[index].Status != JobStatusCompleted {
			t.Fatalf("expected executed job %q to be completed, got %s", executedJobs[index].JobID, executedJobs[index].Status)
		}
	}

	sawFinalizing := false
	for _, snapshot := range snapshots {
		if snapshot.ActiveJobKind == JobKindFinalizeRoot && snapshot.ActiveRunStatus == RunStatusFinalizing && snapshot.ActiveStage == RunStageFinalizing {
			sawFinalizing = true
			break
		}
	}
	if !sawFinalizing {
		t.Fatal("expected finalize job snapshots to expose finalizing run state")
	}
	if len(snapshots) == 0 {
		t.Fatal("expected snapshots")
	}
	lastSnapshot := snapshots[len(snapshots)-1]
	if got, want := lastSnapshot.ActiveRunStatus, RunStatusCompleted; got != want {
		t.Fatalf("expected terminal completed snapshot, got %s want %s", got, want)
	}
}

func TestJobExecutorProgressNeverDecreasesAcrossRoots(t *testing.T) {
	rootOnePath := filepath.Join(t.TempDir(), "root-one")
	rootTwoPath := filepath.Join(t.TempDir(), "root-two")
	mustWriteTestFile(t, filepath.Join(rootOnePath, "alpha.txt"), "alpha")
	mustWriteTestFile(t, filepath.Join(rootTwoPath, "beta.txt"), "beta")

	rootOne := testRunPlannerRootRecord("root-one", rootOnePath)
	rootTwo := testRunPlannerRootRecord("root-two", rootTwoPath)
	plan := RunPlan{
		PlanID: "plan-progress",
		RunID:  "run-progress",
		Kind:   RunKindFull,
		RootPlans: []RootPlan{{
			RootID:   rootOne.ID,
			RootPath: rootOne.Path,
		}, {
			RootID:   rootTwo.ID,
			RootPath: rootTwo.Path,
		}},
		Jobs: []Job{
			testFinalizeJob(rootOne, 0),
			testFinalizeJob(rootOne, 1),
			testFinalizeJob(rootTwo, 2),
			testFinalizeJob(rootTwo, 3),
		},
		TotalWorkUnits: 4,
	}

	executor := NewJobExecutor(nil)
	progresses := make([]int64, 0, 8)
	_, _, err := executor.ExecuteRun(context.Background(), plan, []RootRecord{rootOne, rootTwo}, func(snapshot StatusSnapshot, _ Job) {
		progresses = append(progresses, snapshot.ProgressCurrent)
	})
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}

	if len(progresses) == 0 {
		t.Fatal("expected progress snapshots")
	}
	for index := 1; index < len(progresses); index++ {
		if progresses[index] < progresses[index-1] {
			t.Fatalf("global progress decreased at snapshot %d: got %d after %d", index, progresses[index], progresses[index-1])
		}
	}
	for _, snapshot := range progressesForCompatibility(t, plan, []RootRecord{rootOne, rootTwo}) {
		if snapshot.ActiveRunStatus == RunStatusCompleted {
			continue
		}
		if got, want := snapshot.ActiveProgressTotal, int64(1); got != want {
			t.Fatalf("expected active progress to stay root-scoped for finalize jobs: got %d want %d", got, want)
		}
		if snapshot.ActiveProgressCurrent > snapshot.ActiveProgressTotal {
			t.Fatalf("active progress overflowed its scoped total: got %d/%d", snapshot.ActiveProgressCurrent, snapshot.ActiveProgressTotal)
		}
		if snapshot.RunProgressTotal != plan.TotalWorkUnits {
			t.Fatalf("unexpected run progress total: got %d want %d", snapshot.RunProgressTotal, plan.TotalWorkUnits)
		}
	}
}

func TestJobExecutorNinetyNinePercentMeansSmallKnownRemainder(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "root")
	mustWriteTestFile(t, filepath.Join(rootPath, "alpha.txt"), "alpha")
	root := testRunPlannerRootRecord("root", rootPath)

	jobs := make([]Job, 0, 200)
	for index := 0; index < 200; index++ {
		jobs = append(jobs, testFinalizeJob(root, index))
	}

	plan := RunPlan{
		PlanID: "plan-ninety-nine",
		RunID:  "run-ninety-nine",
		Kind:   RunKindFull,
		RootPlans: []RootPlan{{
			RootID:   root.ID,
			RootPath: root.Path,
		}},
		Jobs:           jobs,
		TotalWorkUnits: int64(len(jobs)),
	}

	executor := NewJobExecutor(nil)
	sawNinetyNine := false
	_, _, err := executor.ExecuteRun(context.Background(), plan, []RootRecord{root}, func(snapshot StatusSnapshot, _ Job) {
		if snapshot.ProgressTotal == 0 {
			return
		}
		percent := (snapshot.ProgressCurrent * 100) / snapshot.ProgressTotal
		if percent != 99 {
			return
		}

		sawNinetyNine = true
		remaining := snapshot.RunProgressTotal - snapshot.RunProgressCurrent
		if remaining*100 > snapshot.RunProgressTotal {
			t.Fatalf("99%% reported too early: remaining=%d total=%d current=%d/%d", remaining, snapshot.RunProgressTotal, snapshot.ProgressCurrent, snapshot.ProgressTotal)
		}
	})
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if !sawNinetyNine {
		t.Fatal("expected executor to report 99% before completion")
	}
}

func testFinalizeJob(root RootRecord, orderIndex int) Job {
	return Job{
		JobID:             fmt.Sprintf("%s-job-%03d", root.ID, orderIndex),
		RootID:            root.ID,
		RootPath:          root.Path,
		ScopePath:         root.Path,
		Kind:              JobKindFinalizeRoot,
		PlannedWriteUnits: 1,
		PlannedTotalUnits: 1,
		Status:            JobStatusPending,
		OrderIndex:        orderIndex,
	}
}

func snapshotEntryPaths(batch SubtreeSnapshotBatch) []string {
	paths := make([]string, 0, len(batch.Entries))
	for _, entry := range batch.Entries {
		paths = append(paths, entry.Path)
	}
	return paths
}

func snapshotDirectoryPaths(batch SubtreeSnapshotBatch) []string {
	paths := make([]string, 0, len(batch.Directories))
	for _, directory := range batch.Directories {
		paths = append(paths, directory.Path)
	}
	return paths
}

func equalPaths(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func progressesForCompatibility(t *testing.T, plan RunPlan, roots []RootRecord) []StatusSnapshot {
	t.Helper()

	snapshots := make([]StatusSnapshot, 0, len(plan.Jobs)*2+1)
	_, _, err := NewJobExecutor(nil).ExecuteRun(context.Background(), plan, roots, func(snapshot StatusSnapshot, _ Job) {
		snapshots = append(snapshots, snapshot)
	})
	if err != nil {
		t.Fatalf("execute compatibility run: %v", err)
	}
	return snapshots
}
