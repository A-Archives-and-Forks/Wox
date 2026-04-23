package filesearch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRunPlanSealFreezesWorkload(t *testing.T) {
	scopeTree := ScopeNode{
		ScopePath:           "/root/subtree",
		ScopeKind:           ScopeKindSubtree,
		ParentScopePath:     "/root",
		DirectoryCount:      2,
		FileCount:           4,
		IndexableEntryCount: 5,
		SkippedCount:        1,
		PlannedScanUnits:    9,
		PlannedWriteUnits:   7,
		SplitRequired:       true,
		Children: []ScopeNode{{
			ScopePath:           "/root/subtree/leaf",
			ScopeKind:           ScopeKindDirectFiles,
			ParentScopePath:     "/root/subtree",
			DirectoryCount:      0,
			FileCount:           3,
			IndexableEntryCount: 3,
			PlannedScanUnits:    3,
			PlannedWriteUnits:   3,
		}},
	}

	rootPlans := []RootPlan{{
		RootID:    "root-1",
		RootPath:  "/root",
		Strategy:  RootPlanStrategySegmented,
		ScopeTree: &scopeTree,
		Totals: PlanTotals{
			DirectoryCount:      2,
			FileCount:           4,
			IndexableEntryCount: 5,
			SkippedCount:        1,
			PlannedScanUnits:    9,
			PlannedWriteUnits:   7,
		},
		Jobs: []JobRef{{
			JobID:      "job-1",
			OrderIndex: 0,
		}},
		SplitPolicyVersion: 1,
	}}

	jobs := []Job{{
		JobID:             "job-1",
		RootID:            "root-1",
		RootPath:          "/root",
		ScopePath:         "/root/subtree",
		Kind:              JobKindSubtree,
		PlannedScanUnits:  9,
		PlannedWriteUnits: 7,
		PlannedTotalUnits: 16,
		Status:            JobStatusPending,
		OrderIndex:        0,
	}, {
		JobID:             "job-2",
		RootID:            "root-1",
		RootPath:          "/root",
		ScopePath:         "/root",
		Kind:              JobKindFinalizeRoot,
		PlannedScanUnits:  0,
		PlannedWriteUnits: 1,
		PlannedTotalUnits: 1,
		Status:            JobStatusPending,
		OrderIndex:        1,
	}}

	plan := RunPlan{
		PlanID:         "plan-1",
		RunID:          "run-1",
		Kind:           RunKindFull,
		RootPlans:      rootPlans,
		Jobs:           jobs,
		TotalWorkUnits: 17,
		PlanningTotals: PlanTotals{
			DirectoryCount:      2,
			FileCount:           4,
			IndexableEntryCount: 5,
			SkippedCount:        1,
			PlannedScanUnits:    9,
			PlannedWriteUnits:   7,
		},
		PreScanTotals: PlanTotals{
			DirectoryCount:      2,
			FileCount:           4,
			IndexableEntryCount: 5,
			SkippedCount:        1,
			PlannedScanUnits:    9,
			PlannedWriteUnits:   7,
		},
	}

	sealed := plan.Seal()

	// Mutate the original planning buffers after sealing to prove the returned
	// plan owns its own workload copies. One logical root can now fan out into
	// several jobs, so sharing buffers would make later planner writes corrupt
	// the active run.
	rootPlans[0].RootPath = "/mutated-root"
	rootPlans[0].Totals.PlannedWriteUnits = 999
	rootPlans[0].Jobs[0].JobID = "mutated-job-ref"
	rootPlans[0].ScopeTree.ScopePath = "/mutated-scope"
	rootPlans[0].ScopeTree.Children[0].ScopePath = "/mutated-child"
	jobs[0].ScopePath = "/mutated-job-scope"
	jobs[0].PlannedTotalUnits = 999
	plan.TotalWorkUnits = 999
	plan.PlanningTotals.FileCount = 999
	plan.PreScanTotals.FileCount = 999
	plan.RootPlans[0].RootPath = "/mutated-plan-root"
	plan.RootPlans[0].Totals.DirectoryCount = 999
	plan.RootPlans[0].ScopeTree.ScopePath = "/mutated-plan-scope"
	plan.RootPlans[0].ScopeTree.Children[0].ScopePath = "/mutated-plan-child"
	plan.Jobs[0].ScopePath = "/mutated-plan-job-scope"
	plan.Jobs[0].PlannedTotalUnits = 888

	if got, want := sealed.TotalWorkUnits, int64(17); got != want {
		t.Fatalf("sealed total work units changed: got %d want %d", got, want)
	}
	if got, want := sealed.PlanningTotals.FileCount, int64(4); got != want {
		t.Fatalf("sealed planning totals changed: got %d want %d", got, want)
	}
	if got, want := sealed.PreScanTotals.FileCount, int64(4); got != want {
		t.Fatalf("sealed pre-scan totals changed: got %d want %d", got, want)
	}
	if got, want := sealed.RootPlans[0].RootPath, "/root"; got != want {
		t.Fatalf("sealed root path changed: got %q want %q", got, want)
	}
	if got, want := sealed.RootPlans[0].Totals.PlannedWriteUnits, int64(7); got != want {
		t.Fatalf("sealed root totals changed: got %d want %d", got, want)
	}
	if got, want := sealed.RootPlans[0].Jobs[0].JobID, "job-1"; got != want {
		t.Fatalf("sealed root job refs changed: got %q want %q", got, want)
	}
	if got, want := sealed.RootPlans[0].ScopeTree.ScopePath, "/root/subtree"; got != want {
		t.Fatalf("sealed scope tree root changed: got %q want %q", got, want)
	}
	if got, want := sealed.RootPlans[0].ScopeTree.Children[0].ScopePath, "/root/subtree/leaf"; got != want {
		t.Fatalf("sealed scope tree child changed: got %q want %q", got, want)
	}
	if got, want := sealed.Jobs[0].ScopePath, "/root/subtree"; got != want {
		t.Fatalf("sealed job scope changed: got %q want %q", got, want)
	}
	if got, want := sealed.Jobs[0].PlannedTotalUnits, int64(16); got != want {
		t.Fatalf("sealed job totals changed: got %d want %d", got, want)
	}
}

func TestRunPlannerBuildsSingleRootPlan(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "single-root")
	mustWriteTestFile(t, filepath.Join(rootPath, "nested", "alpha.txt"), "alpha")
	mustWriteTestFile(t, filepath.Join(rootPath, "nested", "beta.txt"), "beta")

	planner := &RunPlanner{
		policy: newPolicyState(Policy{}),
		budget: splitBudget{
			LeafEntryBudget:     16,
			LeafWriteBudget:     16,
			LeafMemoryBudget:    1 << 20,
			DirectFileBatchSize: 16,
		},
	}

	plan, err := planner.PlanFullRun(context.Background(), []RootRecord{testRunPlannerRootRecord("root-single", rootPath)})
	if err != nil {
		t.Fatalf("plan full run: %v", err)
	}

	if got, want := plan.Kind, RunKindFull; got != want {
		t.Fatalf("unexpected run kind: got %s want %s", got, want)
	}
	if got, want := len(plan.RootPlans), 1; got != want {
		t.Fatalf("unexpected root plan count: got %d want %d", got, want)
	}
	rootPlan := plan.RootPlans[0]
	if got, want := rootPlan.Strategy, RootPlanStrategySingle; got != want {
		t.Fatalf("unexpected root strategy: got %s want %s", got, want)
	}
	if rootPlan.ScopeTree == nil {
		t.Fatal("expected sealed scope tree")
	}
	if got, want := rootPlan.Totals.DirectoryCount, int64(2); got != want {
		t.Fatalf("unexpected directory count: got %d want %d", got, want)
	}
	if got, want := rootPlan.Totals.FileCount, int64(2); got != want {
		t.Fatalf("unexpected file count: got %d want %d", got, want)
	}
	if got, want := rootPlan.Totals.IndexableEntryCount, int64(4); got != want {
		t.Fatalf("unexpected indexable entry count: got %d want %d", got, want)
	}
	if got, want := len(plan.Jobs), 2; got != want {
		t.Fatalf("unexpected job count: got %d want %d", got, want)
	}
	if got, want := plan.Jobs[0].Kind, JobKindSubtree; got != want {
		t.Fatalf("unexpected first job kind: got %s want %s", got, want)
	}
	if got, want := plan.Jobs[0].ScopePath, rootPath; got != want {
		t.Fatalf("unexpected first job scope: got %q want %q", got, want)
	}
	if got, want := plan.Jobs[1].Kind, JobKindFinalizeRoot; got != want {
		t.Fatalf("unexpected finalize job kind: got %s want %s", got, want)
	}
	if got, want := len(rootPlan.Jobs), 2; got != want {
		t.Fatalf("unexpected root job refs: got %d want %d", got, want)
	}
	if planner.planningRootBuffers != nil {
		t.Fatal("expected planner buffers to be released after sealing")
	}
}

func TestRunPlannerSplitsLargeRootIntoLeafJobs(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "segmented-root")
	mustWriteTestFile(t, filepath.Join(rootPath, "huge", "leaf-a", "a-1.txt"), "a1")
	mustWriteTestFile(t, filepath.Join(rootPath, "huge", "leaf-a", "a-2.txt"), "a2")
	mustWriteTestFile(t, filepath.Join(rootPath, "huge", "leaf-b", "b-1.txt"), "b1")
	mustWriteTestFile(t, filepath.Join(rootPath, "huge", "leaf-b", "b-2.txt"), "b2")

	planner := &RunPlanner{
		policy: newPolicyState(Policy{}),
		budget: splitBudget{
			LeafEntryBudget:     3,
			LeafWriteBudget:     3,
			LeafMemoryBudget:    1 << 20,
			DirectFileBatchSize: 3,
		},
	}

	plan, err := planner.PlanFullRun(context.Background(), []RootRecord{testRunPlannerRootRecord("root-segmented", rootPath)})
	if err != nil {
		t.Fatalf("plan full run: %v", err)
	}

	rootPlan := plan.RootPlans[0]
	if got, want := rootPlan.Strategy, RootPlanStrategySegmented; got != want {
		t.Fatalf("unexpected root strategy: got %s want %s", got, want)
	}
	if rootPlan.ScopeTree == nil {
		t.Fatal("expected sealed scope tree")
	}
	if got, want := rootPlan.Totals.DirectoryCount, int64(4); got != want {
		t.Fatalf("unexpected directory count: got %d want %d", got, want)
	}
	if got, want := rootPlan.Totals.FileCount, int64(4); got != want {
		t.Fatalf("unexpected file count: got %d want %d", got, want)
	}
	if got, want := rootPlan.Totals.IndexableEntryCount, int64(8); got != want {
		t.Fatalf("unexpected indexable entry count: got %d want %d", got, want)
	}
	if len(plan.Jobs) < 4 {
		t.Fatalf("expected multiple leaf jobs plus finalize, got %d jobs", len(plan.Jobs))
	}
	if plan.Jobs[len(plan.Jobs)-1].Kind != JobKindFinalizeRoot {
		t.Fatalf("expected final job to finalize root, got %s", plan.Jobs[len(plan.Jobs)-1].Kind)
	}

	subtreeScopeA := filepath.Join(rootPath, "huge", "leaf-a")
	subtreeScopeB := filepath.Join(rootPath, "huge", "leaf-b")
	sawLeafA := false
	sawLeafB := false
	for _, job := range plan.Jobs[:len(plan.Jobs)-1] {
		if job.PlannedTotalUnits > 6 {
			t.Fatalf("expected split leaf job to stay bounded, got planned total %d for %s", job.PlannedTotalUnits, job.ScopePath)
		}
		if job.Kind != JobKindDirectFiles && job.Kind != JobKindSubtree {
			t.Fatalf("unexpected leaf job kind: %s", job.Kind)
		}
		if job.ScopePath == subtreeScopeA {
			sawLeafA = true
		}
		if job.ScopePath == subtreeScopeB {
			sawLeafB = true
		}
	}
	if !sawLeafA || !sawLeafB {
		t.Fatalf("expected split plan to create leaf subtree jobs for %q and %q", subtreeScopeA, subtreeScopeB)
	}
	if planner.planningRootBuffers != nil {
		t.Fatal("expected planner buffers to be released after sealing")
	}
}

func TestRunPlannerKeepsWideDirectFilesInOneJob(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "wide-root")
	for i := 0; i < 5; i++ {
		mustWriteTestFile(t, filepath.Join(rootPath, filepath.Base(rootPath)+"-"+time.Unix(int64(i+1), 0).Format("150405")+".txt"), "wide")
	}

	planner := &RunPlanner{
		policy: newPolicyState(Policy{}),
		budget: splitBudget{
			LeafEntryBudget:     3,
			LeafWriteBudget:     3,
			LeafMemoryBudget:    1 << 20,
			DirectFileBatchSize: 2,
		},
	}

	plan, err := planner.PlanFullRun(context.Background(), []RootRecord{testRunPlannerRootRecord("root-wide", rootPath)})
	if err != nil {
		t.Fatalf("plan full run: %v", err)
	}

	rootPlan := plan.RootPlans[0]
	if got, want := rootPlan.Strategy, RootPlanStrategySingle; got != want {
		t.Fatalf("unexpected root strategy: got %s want %s", got, want)
	}
	if got, want := rootPlan.Totals.DirectoryCount, int64(1); got != want {
		t.Fatalf("unexpected directory count: got %d want %d", got, want)
	}
	if got, want := rootPlan.Totals.FileCount, int64(5); got != want {
		t.Fatalf("unexpected file count: got %d want %d", got, want)
	}
	if got, want := rootPlan.Totals.IndexableEntryCount, int64(6); got != want {
		t.Fatalf("unexpected indexable entry count: got %d want %d", got, want)
	}

	directFileJobs := 0
	for _, job := range plan.Jobs[:len(plan.Jobs)-1] {
		if job.Kind != JobKindDirectFiles {
			t.Fatalf("expected wide root to keep only direct-files jobs before finalize, got %s", job.Kind)
		}
		if job.ScopePath != rootPath {
			t.Fatalf("expected direct-files job to stay on the root scope, got %q want %q", job.ScopePath, rootPath)
		}
		if job.PlannedScanUnits != 6 || job.PlannedWriteUnits != 6 {
			t.Fatalf(
				"expected wide direct-files job to own the whole directory scope, got scan=%d write=%d",
				job.PlannedScanUnits,
				job.PlannedWriteUnits,
			)
		}
		directFileJobs++
	}
	if directFileJobs != 1 {
		t.Fatalf("expected one direct-files job for the wide root scope, got %d", directFileJobs)
	}
	if got, want := plan.Jobs[len(plan.Jobs)-1].Kind, JobKindFinalizeRoot; got != want {
		t.Fatalf("unexpected finalize job kind: got %s want %s", got, want)
	}
	if planner.planningRootBuffers != nil {
		t.Fatal("expected planner buffers to be released after sealing")
	}
}

func TestRunPlannerSkipsUnreadableChildDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based unreadable directories are not stable on Windows test hosts")
	}

	rootPath := filepath.Join(t.TempDir(), "root-unreadable-child")
	readableDir := filepath.Join(rootPath, "readable")
	unreadableDir := filepath.Join(rootPath, "unreadable")

	mustWriteTestFile(t, filepath.Join(readableDir, "alpha.txt"), "alpha")
	mustWriteTestFile(t, filepath.Join(unreadableDir, "blocked.txt"), "blocked")

	if err := os.Chmod(unreadableDir, 0o000); err != nil {
		t.Fatalf("chmod unreadable dir: %v", err)
	}
	defer os.Chmod(unreadableDir, 0o755)

	planner := &RunPlanner{
		policy: newPolicyState(Policy{}),
		budget: splitBudget{
			LeafEntryBudget:     3,
			LeafWriteBudget:     3,
			LeafMemoryBudget:    1 << 20,
			DirectFileBatchSize: 2,
		},
	}

	plan, err := planner.PlanFullRun(context.Background(), []RootRecord{testRunPlannerRootRecord("root-unreadable-child", rootPath)})
	if err != nil {
		t.Fatalf("plan full run with unreadable child: %v", err)
	}

	if len(plan.RootPlans) != 1 {
		t.Fatalf("expected one root plan, got %d", len(plan.RootPlans))
	}
	if got := plan.RootPlans[0].Totals.SkippedCount; got <= 0 {
		t.Fatalf("expected unreadable child directory to increase skipped count, got %d", got)
	}
	for _, job := range plan.Jobs {
		if job.Kind == JobKindFinalizeRoot {
			continue
		}
		if filepath.Clean(job.ScopePath) == filepath.Clean(unreadableDir) {
			t.Fatalf("expected unreadable child directory %q to be skipped instead of planned as a job", unreadableDir)
		}
	}
}

func TestRunPlannerIncrementalScopesAreRebuiltFresh(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "incremental-root")
	scopePath := filepath.Join(rootPath, "scope")
	leafAPath := filepath.Join(scopePath, "leaf-a")
	leafBPath := filepath.Join(scopePath, "leaf-b")

	mustWriteTestFile(t, filepath.Join(leafAPath, "a.txt"), "a")

	root := testRunPlannerRootRecord("root-incremental", rootPath)
	planner := &RunPlanner{
		policy: newPolicyState(Policy{}),
		budget: splitBudget{
			LeafEntryBudget:     3,
			LeafWriteBudget:     3,
			LeafMemoryBudget:    1 << 20,
			DirectFileBatchSize: 1,
		},
	}

	firstPlan, err := planner.PlanIncrementalRun(context.Background(), []RootRecord{root}, []ReconcileBatch{{
		RootID: root.ID,
		Mode:   ReconcileModeSubtree,
		Paths:  []string{scopePath},
	}})
	if err != nil {
		t.Fatalf("plan first incremental run: %v", err)
	}

	mustWriteTestFile(t, filepath.Join(leafBPath, "b.txt"), "b")

	secondPlan, err := planner.PlanIncrementalRun(context.Background(), []RootRecord{root}, []ReconcileBatch{{
		RootID: root.ID,
		Mode:   ReconcileModeSubtree,
		Paths:  []string{scopePath},
	}})
	if err != nil {
		t.Fatalf("plan second incremental run: %v", err)
	}

	firstSawLeafB := false
	for _, job := range firstPlan.Jobs {
		if job.ScopePath == leafBPath {
			firstSawLeafB = true
		}
	}
	if firstSawLeafB {
		t.Fatalf("expected first incremental plan to exclude later subtree %q", leafBPath)
	}

	secondSawLeafB := false
	for _, job := range secondPlan.Jobs {
		if job.ScopePath == leafBPath {
			secondSawLeafB = true
		}
	}
	if !secondSawLeafB {
		t.Fatalf("expected second incremental plan to rebuild fresh scopes and include %q", leafBPath)
	}
}

func testRunPlannerRootRecord(id string, path string) RootRecord {
	now := time.Now().UnixMilli()
	return RootRecord{
		ID:        id,
		Path:      path,
		Kind:      RootKindUser,
		Status:    RootStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
