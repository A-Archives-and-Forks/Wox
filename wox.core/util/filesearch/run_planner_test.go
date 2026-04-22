package filesearch

import "testing"

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
