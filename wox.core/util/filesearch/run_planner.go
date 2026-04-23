package filesearch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"wox/util"
)

const estimatedPlannerEntryBytes int64 = 256

// RunPlanner builds one sealed full-run workload in memory before execution.
// The previous root-centric flow discovered and executed work as the same loop,
// which made huge roots hold too much state at once and left progress totals
// unstable. This planner splits the work first, counts it exactly, then seals
// the immutable job list that later execution will consume.
type RunPlanner struct {
	policy     *policyState
	budget     splitBudget
	onProgress func(RunPlannerProgress)

	// planningRootBuffers are intentionally released after sealing so the planner
	// does not retain a second copy of a giant scope tree through execution.
	planningRootBuffers []*runPlannerRootBuffer
}

// RunPlannerProgress reports the planner-owned stages before execution starts.
// Execution progress is reported elsewhere, so this lightweight callback only
// exists to surface planning and pre-scan as first-class, monotonic phases.
type RunPlannerProgress struct {
	Stage     RunStage
	Root      RootRecord
	RootIndex int
	RootTotal int
	ScopePath string
}

type runPlannerRootBuffer struct {
	root      RootRecord
	rootScope *runPlannerScopeBuffer
}

type runPlannerScopeBuffer struct {
	scopePath       string
	scopeKind       ScopeKind
	parentScopePath string
	totals          PlanTotals
	splitRequired   bool
	children        []*runPlannerScopeBuffer
}

func NewRunPlanner(policy *policyState) *RunPlanner {
	if policy == nil {
		policy = newPolicyState(Policy{})
	}
	return &RunPlanner{
		policy: policy,
		budget: defaultSplitBudget(),
	}
}

func (p *RunPlanner) SetProgressCallback(callback func(RunPlannerProgress)) {
	if p == nil {
		return
	}
	p.onProgress = callback
}

func (p *RunPlanner) PlanFullRun(ctx context.Context, roots []RootRecord) (RunPlan, error) {
	if p == nil {
		p = NewRunPlanner(nil)
	}
	if p.policy == nil {
		p.policy = newPolicyState(Policy{})
	}

	budget := normalizeSplitBudget(p.budget)

	// Phase 1: planning. The old root-centric loop only knew about one whole
	// root at execution time. We first build a structural frontier so the later
	// pre-scan can split huge roots without changing persisted root identity.
	rootBuffers, err := p.planRoots(ctx, roots)
	if err != nil {
		return RunPlan{}, err
	}
	p.planningRootBuffers = rootBuffers

	// Phase 2: pre-scan. Version 1 deliberately performs exact metadata reads
	// here without constructing EntryRecord slices. The extra metadata I/O is
	// accepted so progress denominators stay monotonic and truthful once the run
	// starts executing.
	for index, rootBuffer := range p.planningRootBuffers {
		p.emitProgress(RunPlannerProgress{
			Stage:     RunStagePreScan,
			Root:      rootBuffer.root,
			RootIndex: index + 1,
			RootTotal: len(p.planningRootBuffers),
			ScopePath: filepath.Clean(rootBuffer.root.Path),
		})
		if err := p.preScanRoot(ctx, rootBuffer, budget, index+1, len(p.planningRootBuffers)); err != nil {
			p.planningRootBuffers = nil
			return RunPlan{}, wrapRunPlannerRootError(rootBuffer.root.ID, err)
		}
	}

	// Phase 3: seal. We convert the planner-owned buffers into immutable plan
	// structs, deep-copy them with RunPlan.Seal, then drop the planner buffers so
	// execution does not keep the same giant scope tree alive twice.
	draft, err := p.buildDraftPlan(RunKindFull, "full-plan", "full-run")
	if err != nil {
		p.planningRootBuffers = nil
		return RunPlan{}, err
	}
	sealed := draft.Seal()
	p.planningRootBuffers = nil

	return sealed, nil
}

func (p *RunPlanner) PlanIncrementalRun(ctx context.Context, roots []RootRecord, batches []ReconcileBatch) (RunPlan, error) {
	if p == nil {
		p = NewRunPlanner(nil)
	}
	if p.policy == nil {
		p.policy = newPolicyState(Policy{})
	}

	budget := normalizeSplitBudget(p.budget)
	rootBuffers, err := p.planIncrementalRoots(ctx, roots, batches)
	if err != nil {
		return RunPlan{}, err
	}
	p.planningRootBuffers = rootBuffers

	for index, rootBuffer := range p.planningRootBuffers {
		p.emitProgress(RunPlannerProgress{
			Stage:     RunStagePreScan,
			Root:      rootBuffer.root,
			RootIndex: index + 1,
			RootTotal: len(p.planningRootBuffers),
			ScopePath: filepath.Clean(rootBuffer.root.Path),
		})
		if err := p.preScanRoot(ctx, rootBuffer, budget, index+1, len(p.planningRootBuffers)); err != nil {
			p.planningRootBuffers = nil
			return RunPlan{}, wrapRunPlannerRootError(rootBuffer.root.ID, err)
		}
	}

	draft, err := p.buildDraftPlan(RunKindIncremental, "incremental-plan", "incremental-run")
	if err != nil {
		p.planningRootBuffers = nil
		return RunPlan{}, err
	}
	sealed := draft.Seal()
	p.planningRootBuffers = nil

	return sealed, nil
}

func (p *RunPlanner) planRoots(ctx context.Context, roots []RootRecord) ([]*runPlannerRootBuffer, error) {
	buffers := make([]*runPlannerRootBuffer, 0, len(roots))
	for index, root := range roots {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		p.emitProgress(RunPlannerProgress{
			Stage:     RunStagePlanning,
			Root:      root,
			RootIndex: index + 1,
			RootTotal: len(roots),
			ScopePath: filepath.Clean(root.Path),
		})

		buffer, err := p.planRoot(ctx, root)
		if err != nil {
			return nil, wrapRunPlannerRootError(root.ID, err)
		}
		buffers = append(buffers, buffer)
	}
	return buffers, nil
}

func (p *RunPlanner) planIncrementalRoots(ctx context.Context, roots []RootRecord, batches []ReconcileBatch) ([]*runPlannerRootBuffer, error) {
	rootByID := make(map[string]RootRecord, len(roots))
	for _, root := range roots {
		rootByID[root.ID] = root
	}

	type rootDraft struct {
		root     RootRecord
		children []*runPlannerScopeBuffer
	}

	rootDrafts := make(map[string]*rootDraft, len(batches))
	rootOrder := make([]string, 0, len(batches))
	for _, batch := range batches {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		root, ok := rootByID[batch.RootID]
		if !ok {
			return nil, fmt.Errorf("incremental planner root %q not found", batch.RootID)
		}

		draft, exists := rootDrafts[root.ID]
		if !exists {
			draft = &rootDraft{root: root}
			rootDrafts[root.ID] = draft
			rootOrder = append(rootOrder, root.ID)
		}

		if batch.Mode == ReconcileModeRoot || len(batch.Paths) == 0 {
			draft.children = []*runPlannerScopeBuffer{{
				scopePath: filepath.Clean(root.Path),
				scopeKind: ScopeKindSubtree,
			}}
			continue
		}

		for _, scopePath := range batch.Paths {
			cleanScope := filepath.Clean(scopePath)
			if !pathWithinScope(root.Path, cleanScope) {
				return nil, &runRootError{
					RootID: root.ID,
					Err:    fmt.Errorf("incremental scope path %q is outside root path %q", cleanScope, root.Path),
				}
			}
			draft.children = append(draft.children, &runPlannerScopeBuffer{
				scopePath:       cleanScope,
				scopeKind:       ScopeKindSubtree,
				parentScopePath: filepath.Clean(root.Path),
			})
		}
	}

	buffers := make([]*runPlannerRootBuffer, 0, len(rootOrder))
	for index, rootID := range rootOrder {
		draft := rootDrafts[rootID]
		if draft == nil {
			continue
		}

		p.emitProgress(RunPlannerProgress{
			Stage:     RunStagePlanning,
			Root:      draft.root,
			RootIndex: index + 1,
			RootTotal: len(rootOrder),
			ScopePath: filepath.Clean(draft.root.Path),
		})

		rootScope := &runPlannerScopeBuffer{
			scopePath: filepath.Clean(draft.root.Path),
			scopeKind: ScopeKindSubtree,
		}

		if len(draft.children) == 1 && filepath.Clean(draft.children[0].scopePath) == filepath.Clean(draft.root.Path) {
			rootScope = draft.children[0]
		} else if len(draft.children) > 0 {
			rootScope.splitRequired = true
			rootScope.children = dedupePlannerScopes(draft.children)
		}

		buffers = append(buffers, &runPlannerRootBuffer{
			root:      draft.root,
			rootScope: rootScope,
		})
	}

	return buffers, nil
}

func (p *RunPlanner) planRoot(ctx context.Context, root RootRecord) (*runPlannerRootBuffer, error) {
	cleanRootPath := filepath.Clean(root.Path)
	if root.ID == "" {
		return nil, fmt.Errorf("root id is required")
	}
	if root.Path == "" {
		return nil, fmt.Errorf("root path is required")
	}
	info, err := os.Stat(cleanRootPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root path %q is not a directory", cleanRootPath)
	}

	rootScope := &runPlannerScopeBuffer{
		scopePath: cleanRootPath,
		scopeKind: ScopeKindSubtree,
	}

	return &runPlannerRootBuffer{
		root:      root,
		rootScope: rootScope,
	}, nil
}

func (p *RunPlanner) preScanRoot(ctx context.Context, rootBuffer *runPlannerRootBuffer, budget splitBudget, rootIndex int, rootTotal int) error {
	if rootBuffer == nil || rootBuffer.rootScope == nil {
		return nil
	}
	if len(rootBuffer.rootScope.children) > 0 {
		for _, child := range rootBuffer.rootScope.children {
			if err := p.preScanScope(ctx, rootBuffer.root, child, budget, rootIndex, rootTotal); err != nil {
				return err
			}
		}
		rootBuffer.rootScope.totals = aggregateChildTotals(rootBuffer.rootScope.children)
		return nil
	}
	return p.preScanScope(ctx, rootBuffer.root, rootBuffer.rootScope, budget, rootIndex, rootTotal)
}

func (p *RunPlanner) preScanScope(ctx context.Context, root RootRecord, scope *runPlannerScopeBuffer, budget splitBudget, rootIndex int, rootTotal int) error {
	// Pre-scan still reports a root-level stable denominator in version 1
	// because recursive subtree splitting can discover more scopes mid-pass.
	// Emitting the active scope path here still tells the UI exactly which
	// subtree is being measured without letting the percentage move backwards.
	p.emitProgress(RunPlannerProgress{
		Stage:     RunStagePreScan,
		Root:      root,
		RootIndex: rootIndex,
		RootTotal: rootTotal,
		ScopePath: filepath.Clean(scope.scopePath),
	})
	switch scope.scopeKind {
	case ScopeKindDirectFiles:
		return p.preScanDirectFilesScope(ctx, root, scope, budget)
	case ScopeKindSubtree:
		return p.preScanSubtreeScope(ctx, root, scope, budget, rootIndex, rootTotal)
	default:
		return fmt.Errorf("unsupported scope kind %q", scope.scopeKind)
	}
}

func (p *RunPlanner) preScanDirectFilesScope(ctx context.Context, root RootRecord, scope *runPlannerScopeBuffer, _ splitBudget) error {
	totals, _, err := p.scanDirectFilesScope(ctx, root, scope.scopePath)
	if err != nil {
		return err
	}
	scope.totals = totals
	// A single directory now owns all of its direct files in one job. The older
	// chunked plan kept write batches smaller, but it also split delete ownership
	// across sibling jobs so removed direct files could linger in the index.
	// Keeping one job per directory restores a single authoritative prune scope.
	return nil
}

func (p *RunPlanner) preScanSubtreeScope(ctx context.Context, root RootRecord, scope *runPlannerScopeBuffer, budget splitBudget, rootIndex int, rootTotal int) error {
	totals, childDirs, err := p.scanSubtreeScope(ctx, root, scope.scopePath)
	if err != nil {
		return err
	}
	scope.totals = totals
	if !scopeExceedsBudget(scope.totals, budget) {
		return nil
	}

	scope.splitRequired = true
	scope.children = make([]*runPlannerScopeBuffer, 0, len(childDirs)+1)

	// The old root-centric path could only keep a huge subtree as one future job.
	// Replacing an oversized subtree with direct files plus child subtrees keeps
	// each leaf bounded while preserving the user's original root definition.
	directFilesChild := &runPlannerScopeBuffer{
		scopePath:       scope.scopePath,
		scopeKind:       ScopeKindDirectFiles,
		parentScopePath: scope.scopePath,
	}
	if err := p.preScanScope(ctx, root, directFilesChild, budget, rootIndex, rootTotal); err != nil {
		return err
	}
	if directFilesChild.totals.IndexableEntryCount > 0 {
		scope.children = append(scope.children, directFilesChild)
	}

	for _, childDir := range childDirs {
		childScope := &runPlannerScopeBuffer{
			scopePath:       childDir,
			scopeKind:       ScopeKindSubtree,
			parentScopePath: scope.scopePath,
		}
		if err := p.preScanScope(ctx, root, childScope, budget, rootIndex, rootTotal); err != nil {
			return err
		}
		if childScope.totals.IndexableEntryCount == 0 {
			continue
		}
		scope.children = append(scope.children, childScope)
	}
	return nil
}

func (p *RunPlanner) scanDirectFilesScope(ctx context.Context, root RootRecord, scopePath string) (PlanTotals, []string, error) {
	info, err := os.Stat(scopePath)
	if err != nil {
		if os.IsNotExist(err) {
			return PlanTotals{}, nil, nil
		}
		return PlanTotals{}, nil, err
	}
	if !info.IsDir() {
		return PlanTotals{}, nil, fmt.Errorf("direct-files scope %q is not a directory", scopePath)
	}

	totals := PlanTotals{
		DirectoryCount:      1,
		IndexableEntryCount: 1,
		PlannedScanUnits:    1,
		PlannedWriteUnits:   1,
	}
	dirEntries, err := os.ReadDir(scopePath)
	if err != nil {
		return PlanTotals{}, nil, fmt.Errorf("read direct-files scope %q: %w", scopePath, err)
	}

	directFiles := make([]string, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		select {
		case <-ctx.Done():
			return PlanTotals{}, nil, ctx.Err()
		default:
		}

		childPath := filepath.Join(scopePath, dirEntry.Name())
		info, infoErr := strictDirEntryInfo(scopePath, dirEntry)
		if infoErr != nil {
			// The run planner used to fail the whole root when one child entry under
			// a readable directory denied metadata access. The older root-centric
			// scanner skipped those unreadable children and kept indexing the rest of
			// the root, so we preserve that behavior here instead of turning one
			// Windows-protected child into a root-wide failure.
			if shouldSkipUnreadableTraversalError(infoErr) {
				totals.SkippedCount++
				util.GetLogger().Warn(ctx, "filesearch skipped unreadable direct-files child "+childPath+": "+infoErr.Error())
				continue
			}
			return PlanTotals{}, nil, infoErr
		}
		if info.IsDir() {
			if shouldSkipSystemPath(childPath, true) || !p.policy.shouldIndexPath(root, childPath, true) {
				totals.SkippedCount++
			}
			continue
		}
		if shouldSkipSystemPath(childPath, false) {
			totals.SkippedCount++
			continue
		}
		if !p.policy.shouldIndexPath(root, childPath, false) {
			totals.SkippedCount++
			continue
		}

		directFiles = append(directFiles, childPath)
		totals.FileCount++
		totals.IndexableEntryCount++
		totals.PlannedScanUnits++
		totals.PlannedWriteUnits++
	}

	sort.Strings(directFiles)
	return totals, directFiles, nil
}

func (p *RunPlanner) scanSubtreeScope(ctx context.Context, root RootRecord, scopePath string) (PlanTotals, []string, error) {
	info, err := os.Stat(scopePath)
	if err != nil {
		if os.IsNotExist(err) {
			return PlanTotals{}, nil, nil
		}
		return PlanTotals{}, nil, err
	}
	if !info.IsDir() {
		return PlanTotals{}, nil, fmt.Errorf("subtree scope %q is not a directory", scopePath)
	}

	type queueItem struct {
		path string
	}

	totals := PlanTotals{}
	rootChildDirs := make([]string, 0)
	queue := []queueItem{{path: scopePath}}

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return PlanTotals{}, nil, ctx.Err()
		default:
		}

		current := queue[0]
		queue = queue[1:]

		dirEntries, readErr := os.ReadDir(current.path)
		if readErr != nil {
			// The new run planner originally failed fast on any unreadable child
			// directory so its pre-scan counts stayed exact. That was too strict for
			// real Windows roots such as C:\Windows, where protected children like
			// CSC should be skipped rather than aborting the whole root. We still
			// fail if the scope root itself is unreadable, but unreadable descendants
			// are now treated as skipped work so the rest of the root can index.
			if current.path != scopePath && shouldSkipUnreadableTraversalError(readErr) {
				totals.SkippedCount++
				util.GetLogger().Warn(ctx, "filesearch skipped unreadable subtree path "+current.path+": "+readErr.Error())
				continue
			}
			return PlanTotals{}, nil, fmt.Errorf("read subtree scope %q: %w", current.path, readErr)
		}

		totals.DirectoryCount++
		totals.IndexableEntryCount++
		totals.PlannedScanUnits++
		totals.PlannedWriteUnits++
		if current.path != scopePath && filepath.Dir(current.path) == scopePath {
			rootChildDirs = append(rootChildDirs, current.path)
		}

		for _, dirEntry := range dirEntries {
			childPath := filepath.Join(current.path, dirEntry.Name())
			info, infoErr := strictDirEntryInfo(current.path, dirEntry)
			if infoErr != nil {
				if shouldSkipUnreadableTraversalError(infoErr) {
					totals.SkippedCount++
					util.GetLogger().Warn(ctx, "filesearch skipped unreadable subtree child "+childPath+": "+infoErr.Error())
					continue
				}
				return PlanTotals{}, nil, infoErr
			}

			isDir := info.IsDir()
			if shouldSkipSystemPath(childPath, isDir) {
				totals.SkippedCount++
				continue
			}
			if !p.policy.shouldIndexPath(root, childPath, isDir) {
				totals.SkippedCount++
				continue
			}

			if isDir {
				queue = append(queue, queueItem{path: childPath})
				continue
			}

			totals.FileCount++
			totals.IndexableEntryCount++
			totals.PlannedScanUnits++
			totals.PlannedWriteUnits++
		}
	}

	sort.Strings(rootChildDirs)
	return totals, rootChildDirs, nil
}

func (p *RunPlanner) buildDraftPlan(kind RunKind, planID string, runID string) (RunPlan, error) {
	plan := RunPlan{
		PlanID:    planID,
		RunID:     runID,
		Kind:      kind,
		RootPlans: make([]RootPlan, 0, len(p.planningRootBuffers)),
		Jobs:      make([]Job, 0),
	}
	plan.PlanningTotals.PlannedScanUnits = int64(len(p.planningRootBuffers))

	orderIndex := 0
	for _, rootBuffer := range p.planningRootBuffers {
		if rootBuffer == nil || rootBuffer.rootScope == nil {
			continue
		}

		leafScopes := collectLeafScopes(rootBuffer.rootScope)
		rootPlan := RootPlan{
			RootID:             rootBuffer.root.ID,
			RootPath:           filepath.Clean(rootBuffer.root.Path),
			ScopeTree:          rootBuffer.rootScope.toScopeNode(),
			Totals:             rootBuffer.rootScope.totals,
			Jobs:               make([]JobRef, 0, len(leafScopes)+1),
			SplitPolicyVersion: runPlannerSplitPolicyVersionV1,
		}
		if len(leafScopes) <= 1 {
			rootPlan.Strategy = RootPlanStrategySingle
		} else {
			rootPlan.Strategy = RootPlanStrategySegmented
		}

		plan.PreScanTotals = mergePlanTotals(plan.PreScanTotals, rootPlan.Totals)

		// The grouped full-run leaf experiment did not reduce the dominant SQLite
		// cost enough to justify a wider multi-scope job contract. Returning to
		// one sealed job per planned leaf keeps the planner/executor/data path
		// straightforward while preserving the SQLite path index improvement that
		// did measurably reduce collect_diff cost.
		for _, leaf := range leafScopes {
			if leaf == nil || leaf.totals.IndexableEntryCount == 0 {
				continue
			}

			job := Job{
				JobID:             fmt.Sprintf("%s-job-%03d", rootBuffer.root.ID, orderIndex),
				RootID:            rootBuffer.root.ID,
				RootPath:          rootPlan.RootPath,
				ScopePath:         leaf.scopePath,
				Kind:              jobKindForScope(leaf.scopeKind),
				PlannedScanUnits:  leaf.totals.PlannedScanUnits,
				PlannedWriteUnits: leaf.totals.PlannedWriteUnits,
				Status:            JobStatusPending,
				OrderIndex:        orderIndex,
			}
			job.PlannedTotalUnits = job.PlannedScanUnits + job.PlannedWriteUnits
			plan.TotalWorkUnits += job.PlannedTotalUnits
			plan.Jobs = append(plan.Jobs, job)
			rootPlan.Jobs = append(rootPlan.Jobs, JobRef{
				JobID:      job.JobID,
				OrderIndex: job.OrderIndex,
			})
			orderIndex++
		}

		finalizeJob := Job{
			JobID:             fmt.Sprintf("%s-job-%03d", rootBuffer.root.ID, orderIndex),
			RootID:            rootBuffer.root.ID,
			RootPath:          rootPlan.RootPath,
			ScopePath:         rootPlan.RootPath,
			Kind:              JobKindFinalizeRoot,
			PlannedWriteUnits: 1,
			PlannedTotalUnits: 1,
			Status:            JobStatusPending,
			OrderIndex:        orderIndex,
		}
		plan.TotalWorkUnits += finalizeJob.PlannedTotalUnits
		plan.Jobs = append(plan.Jobs, finalizeJob)
		rootPlan.Jobs = append(rootPlan.Jobs, JobRef{
			JobID:      finalizeJob.JobID,
			OrderIndex: finalizeJob.OrderIndex,
		})
		orderIndex++

		plan.RootPlans = append(plan.RootPlans, rootPlan)
	}

	return plan, nil
}

func (p *RunPlanner) emitProgress(progress RunPlannerProgress) {
	if p == nil || p.onProgress == nil {
		return
	}
	p.onProgress(progress)
}

func wrapRunPlannerRootError(rootID string, err error) error {
	if err == nil || rootID == "" {
		return err
	}
	var rootErr *runRootError
	if errors.As(err, &rootErr) && rootErr != nil {
		return err
	}
	return &runRootError{
		RootID: rootID,
		Err:    err,
	}
}

func (s *runPlannerScopeBuffer) toScopeNode() *ScopeNode {
	if s == nil {
		return nil
	}

	node := &ScopeNode{
		ScopePath:           s.scopePath,
		ScopeKind:           s.scopeKind,
		ParentScopePath:     s.parentScopePath,
		DirectoryCount:      s.totals.DirectoryCount,
		FileCount:           s.totals.FileCount,
		IndexableEntryCount: s.totals.IndexableEntryCount,
		SkippedCount:        s.totals.SkippedCount,
		PlannedScanUnits:    s.totals.PlannedScanUnits,
		PlannedWriteUnits:   s.totals.PlannedWriteUnits,
		SplitRequired:       s.splitRequired,
	}
	if len(s.children) == 0 {
		return node
	}

	node.Children = make([]ScopeNode, 0, len(s.children))
	for _, child := range s.children {
		if child == nil {
			continue
		}
		sealedChild := child.toScopeNode()
		if sealedChild == nil {
			continue
		}
		node.Children = append(node.Children, *sealedChild)
	}
	return node
}

func collectLeafScopes(scope *runPlannerScopeBuffer) []*runPlannerScopeBuffer {
	if scope == nil {
		return nil
	}
	if len(scope.children) == 0 {
		return []*runPlannerScopeBuffer{scope}
	}

	leaves := make([]*runPlannerScopeBuffer, 0)
	for _, child := range scope.children {
		leaves = append(leaves, collectLeafScopes(child)...)
	}
	return leaves
}

func dedupePlannerScopes(scopes []*runPlannerScopeBuffer) []*runPlannerScopeBuffer {
	if len(scopes) == 0 {
		return nil
	}

	seen := make(map[string]*runPlannerScopeBuffer, len(scopes))
	order := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope == nil {
			continue
		}
		key := filepath.Clean(scope.scopePath) + "|" + string(scope.scopeKind)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = scope
		order = append(order, key)
	}

	result := make([]*runPlannerScopeBuffer, 0, len(order))
	for _, key := range order {
		result = append(result, seen[key])
	}
	return result
}

func aggregateChildTotals(children []*runPlannerScopeBuffer) PlanTotals {
	totals := PlanTotals{}
	for _, child := range children {
		if child == nil {
			continue
		}
		totals = mergePlanTotals(totals, child.totals)
	}
	return totals
}

func mergePlanTotals(left PlanTotals, right PlanTotals) PlanTotals {
	left.DirectoryCount += right.DirectoryCount
	left.FileCount += right.FileCount
	left.IndexableEntryCount += right.IndexableEntryCount
	left.SkippedCount += right.SkippedCount
	left.PlannedScanUnits += right.PlannedScanUnits
	left.PlannedWriteUnits += right.PlannedWriteUnits
	return left
}

func normalizeSplitBudget(budget splitBudget) splitBudget {
	defaults := defaultSplitBudget()
	if budget.LeafEntryBudget <= 0 {
		budget.LeafEntryBudget = defaults.LeafEntryBudget
	}
	if budget.LeafWriteBudget <= 0 {
		budget.LeafWriteBudget = defaults.LeafWriteBudget
	}
	if budget.LeafMemoryBudget <= 0 {
		budget.LeafMemoryBudget = defaults.LeafMemoryBudget
	}
	if budget.DirectFileBatchSize <= 0 {
		budget.DirectFileBatchSize = defaults.DirectFileBatchSize
	}
	return budget
}

func scopeExceedsBudget(totals PlanTotals, budget splitBudget) bool {
	if totals.IndexableEntryCount > budget.LeafEntryBudget {
		return true
	}
	if totals.PlannedWriteUnits > budget.LeafWriteBudget {
		return true
	}
	return totals.IndexableEntryCount*estimatedPlannerEntryBytes > budget.LeafMemoryBudget
}

func jobKindForScope(scopeKind ScopeKind) JobKind {
	switch scopeKind {
	case ScopeKindDirectFiles:
		return JobKindDirectFiles
	default:
		return JobKindSubtree
	}
}

func strictDirEntryInfo(parentPath string, dirEntry os.DirEntry) (os.FileInfo, error) {
	// Planning and pre-scan must keep exact counts. The previous silent continue
	// undercounted files or directories when metadata lookup failed, which could
	// change split decisions and make progress totals dishonest. Failing fast here
	// keeps the sealed workload truthful instead of pretending the missing entry
	// never existed.
	info, err := dirEntry.Info()
	if err != nil {
		return nil, fmt.Errorf("read metadata for %q: %w", filepath.Join(parentPath, dirEntry.Name()), err)
	}
	return info, nil
}

func shouldSkipUnreadableTraversalError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "access is denied") ||
		strings.Contains(message, "permission denied") ||
		strings.Contains(message, "operation not permitted")
}
