package filesearch

import (
	"context"
	"fmt"
)

type JobExecutor struct {
	snapshot *SnapshotBuilder
}

func NewJobExecutor(snapshot *SnapshotBuilder) *JobExecutor {
	if snapshot == nil {
		snapshot = NewSnapshotBuilder(nil)
	}
	return &JobExecutor{snapshot: snapshot}
}

func (e *JobExecutor) ExecuteRun(ctx context.Context, plan RunPlan, roots []RootRecord, onSnapshot func(StatusSnapshot, Job)) (Run, []Job, error) {
	if e == nil {
		e = NewJobExecutor(nil)
	}
	if e.snapshot == nil {
		e.snapshot = NewSnapshotBuilder(nil)
	}

	rootByID := make(map[string]RootRecord, len(roots))
	for _, root := range roots {
		rootByID[root.ID] = root
	}

	rootOrder := make(map[string]int, len(plan.RootPlans))
	for index, rootPlan := range plan.RootPlans {
		rootOrder[rootPlan.RootID] = index + 1
	}

	run := Run{
		RunID:          plan.RunID,
		PlanID:         plan.PlanID,
		Status:         RunStatusExecuting,
		Stage:          RunStageExecuting,
		TotalWorkUnits: plan.TotalWorkUnits,
	}

	jobs := make([]Job, len(plan.Jobs))
	copy(jobs, plan.Jobs)
	var lastJob Job

	for index := range jobs {
		select {
		case <-ctx.Done():
			run.Status = RunStatusCanceled
			run.LastError = ctx.Err().Error()
			return run, jobs, ctx.Err()
		default:
		}

		job := &jobs[index]
		lastJob = *job
		job.Status = JobStatusRunning
		run.ActiveJobID = job.JobID
		run.Status = RunStatusExecuting
		run.Stage = RunStageExecuting
		if job.Kind == JobKindFinalizeRoot {
			// Consumers need to see finalizing while the finalize job is active,
			// not only after it finishes, otherwise the last root still looks like
			// generic execution right before the run completes.
			run.Status = RunStatusFinalizing
			run.Stage = RunStageFinalizing
		}
		emitJobExecutorSnapshot(run, plan, rootOrder, *job, onSnapshot)

		root, ok := rootByID[job.RootID]
		if !ok {
			err := fmt.Errorf("root %q not found for job %q", job.RootID, job.JobID)
			job.Status = JobStatusFailed
			run.Status = RunStatusFailed
			run.LastError = err.Error()
			emitJobExecutorSnapshot(run, plan, rootOrder, *job, onSnapshot)
			return run, jobs, err
		}

		if err := e.executeJobSnapshot(ctx, root, *job); err != nil {
			job.Status = JobStatusFailed
			run.Status = RunStatusFailed
			run.LastError = err.Error()
			emitJobExecutorSnapshot(run, plan, rootOrder, *job, onSnapshot)
			return run, jobs, err
		}

		// Run-scoped progress must advance from sealed work totals instead of
		// resetting per root. Using the plan's fixed unit budget keeps progress
		// monotonic even when execution crosses from one root's last job into the
		// next root's first job.
		job.Status = JobStatusCompleted
		run.CompletedWorkUnits += job.PlannedTotalUnits
		lastJob = *job
		emitJobExecutorSnapshot(run, plan, rootOrder, *job, onSnapshot)
	}

	run.Status = RunStatusCompleted
	run.ActiveJobID = ""
	lastJob.Status = ""
	emitJobExecutorSnapshot(run, plan, rootOrder, lastJob, onSnapshot)
	return run, jobs, nil
}

func (e *JobExecutor) executeJobSnapshot(ctx context.Context, root RootRecord, job Job) error {
	switch job.Kind {
	case JobKindDirectFiles:
		_, err := e.snapshot.BuildDirectFilesJobSnapshot(ctx, root, job)
		return err
	case JobKindSubtree:
		_, err := e.snapshot.BuildSubtreeJobSnapshot(ctx, root, job)
		return err
	case JobKindFinalizeRoot:
		return nil
	default:
		return fmt.Errorf("unsupported job kind %q", job.Kind)
	}
}

func emitJobExecutorSnapshot(run Run, plan RunPlan, rootOrder map[string]int, job Job, onSnapshot func(StatusSnapshot, Job)) {
	if onSnapshot == nil {
		return
	}
	onSnapshot(buildJobExecutorStatusSnapshot(run, plan, rootOrder, job), job)
}

func buildJobExecutorStatusSnapshot(run Run, plan RunPlan, rootOrder map[string]int, job Job) StatusSnapshot {
	rootStatus := RootStatusScanning
	if job.Kind == JobKindFinalizeRoot {
		rootStatus = RootStatusFinalizing
	}
	activeProgressCurrent, activeProgressTotal := activeJobProgress(job)

	// The previous root-local progress view could show regressions when the next
	// root started at zero. Mirroring the run's sealed unit counters into the
	// exported run-progress fields makes the global progress bar monotonic across
	// job and root boundaries while preserving the legacy active-progress fields
	// as the scoped progress of the current job/root.
	return StatusSnapshot{
		RootCount:             len(plan.RootPlans),
		ProgressCurrent:       run.CompletedWorkUnits,
		ProgressTotal:         run.TotalWorkUnits,
		ActiveRootStatus:      rootStatus,
		ActiveProgressCurrent: activeProgressCurrent,
		ActiveProgressTotal:   activeProgressTotal,
		ActiveRootIndex:       rootOrder[job.RootID],
		ActiveRootTotal:       len(plan.RootPlans),
		ActiveRunStatus:       run.Status,
		ActiveJobKind:         job.Kind,
		ActiveScopePath:       job.ScopePath,
		ActiveStage:           run.Stage,
		RunProgressCurrent:    run.CompletedWorkUnits,
		RunProgressTotal:      run.TotalWorkUnits,
		IsIndexing:            run.Status == RunStatusExecuting || run.Status == RunStatusFinalizing,
		LastError:             run.LastError,
	}
}

func activeJobProgress(job Job) (int64, int64) {
	total := job.PlannedTotalUnits
	if total < 0 {
		total = 0
	}
	current := int64(0)
	if job.Status == JobStatusCompleted {
		current = total
	}
	return current, total
}
