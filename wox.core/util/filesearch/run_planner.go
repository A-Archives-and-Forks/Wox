package filesearch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"wox/util"
)

const estimatedPlannerEntryBytes int64 = 256

// RunPlanner builds one sealed full-run workload in memory before execution.
// The previous root-centric flow discovered and executed work as the same loop,
// which made huge roots hold too much state at once and left progress totals
// unstable. The current planner seals ownership boundaries first, then lets
// streaming execution do the only recursive walk so large roots avoid a duplicate
// planning traversal.
type RunPlanner struct {
	policy         *policyState
	budget         splitBudget
	onProgress     func(RunPlannerProgress)
	rootExclusions map[string][]string
	// Tests use this hook to assert when subtree scans actually hit the
	// filesystem, so planner optimizations can prove they removed redundant
	// rescans without changing the sealed plan shape.
	onSubtreeScan func(scopePath string)

	// planningRootBuffers are intentionally released after sealing so the planner
	// does not retain a second copy of a giant scope tree through execution.
	planningRootBuffers []*runPlannerRootBuffer
}

// RunPlannerProgress reports the planner-owned stages before execution starts.
// Execution progress is reported elsewhere, so this lightweight callback only
// exists to surface planning as a first-class phase before streaming execution.
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

type subtreeChildSummary struct {
	path   string
	totals PlanTotals
}

func NewRunPlanner(policy *policyState) *RunPlanner {
	if policy == nil {
		policy = newPolicyState(Policy{})
	}
	return &RunPlanner{
		policy:         policy,
		budget:         defaultSplitBudget(),
		rootExclusions: map[string][]string{},
	}
}

func (p *RunPlanner) SetProgressCallback(callback func(RunPlannerProgress)) {
	if p == nil {
		return
	}
	p.onProgress = callback
}

func (p *RunPlanner) SetRootExclusions(exclusions map[string][]string) {
	if p == nil {
		return
	}
	p.rootExclusions = copyRootExclusions(exclusions)
}

func (p *RunPlanner) PlanFullRun(ctx context.Context, roots []RootRecord) (RunPlan, error) {
	if p == nil {
		p = NewRunPlanner(nil)
	}
	if p.policy == nil {
		p.policy = newPolicyState(Policy{})
	}

	// Phase 1: planning. The old root-centric loop only knew about one whole
	// root at execution time. We still build the structural frontier here, but
	// leave recursive counting to the streaming executor so large roots are not
	// walked twice before search results become available.
	rootBuffers, err := p.planRoots(ctx, roots)
	if err != nil {
		return RunPlan{}, err
	}
	p.planningRootBuffers = rootBuffers

	for _, rootBuffer := range p.planningRootBuffers {
		if rootBuffer == nil || rootBuffer.rootScope == nil {
			continue
		}
		// Optimization: full indexing used to walk every large root once in
		// pre-scan to compute exact totals, then walk it again during execution.
		// Real ~/Projects captures showed that this duplicate traversal alone can
		// dominate the run, so full scans now keep one root-level streaming job and
		// accept approximate progress totals.
		rootBuffer.rootScope.totals = streamingFullRunEstimatedTotals()
	}

	// Phase 2: seal. We convert the planner-owned buffers into immutable plan
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

func streamingFullRunEstimatedTotals() PlanTotals {
	return PlanTotals{
		DirectoryCount:      1,
		IndexableEntryCount: 1,
		PlannedScanUnits:    1,
		PlannedWriteUnits:   1,
	}
}

func applyStreamingEstimatedTotals(scope *runPlannerScopeBuffer) PlanTotals {
	if scope == nil {
		return PlanTotals{}
	}
	if len(scope.children) == 0 {
		scope.totals = streamingFullRunEstimatedTotals()
		return scope.totals
	}

	totals := PlanTotals{}
	for _, child := range scope.children {
		totals = mergePlanTotals(totals, applyStreamingEstimatedTotals(child))
	}
	scope.totals = totals
	return totals
}

func (p *RunPlanner) PlanIncrementalRun(ctx context.Context, roots []RootRecord, batches []ReconcileBatch) (RunPlan, error) {
	if p == nil {
		p = NewRunPlanner(nil)
	}
	if p.policy == nil {
		p.policy = newPolicyState(Policy{})
	}

	rootBuffers, err := p.planIncrementalRoots(ctx, roots, batches)
	if err != nil {
		return RunPlan{}, err
	}
	p.planningRootBuffers = rootBuffers

	for _, rootBuffer := range p.planningRootBuffers {
		if rootBuffer == nil || rootBuffer.rootScope == nil {
			continue
		}
		// Optimization: incremental indexing also skips exact planning. Dirty
		// scopes are already the caller's best available boundary, so walking them
		// once to count work and again to apply work only delays reconciliation.
		applyStreamingEstimatedTotals(rootBuffer.rootScope)
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
			scopeKind := ScopeKindSubtree
			if batch.Mode == ReconcileModeSubtree && cleanScope == filepath.Clean(root.Path) {
				// DirtyQueue collapses file changes to their parent directory. When
				// that parent is the configured root, the smallest correct retry is a
				// direct-files job for the root directory, not a recursive root scan.
				scopeKind = ScopeKindDirectFiles
			}
			draft.children = append(draft.children, &runPlannerScopeBuffer{
				scopePath:       cleanScope,
				scopeKind:       scopeKind,
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
	totals, childSummaries, err := p.scanSubtreeScope(ctx, root, scope.scopePath)
	if err != nil {
		return err
	}
	scope.totals = totals
	if !scopeExceedsBudget(scope.totals, budget) {
		return nil
	}

	scope.splitRequired = true
	scope.children = make([]*runPlannerScopeBuffer, 0, len(childSummaries)+1)

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

	for _, childSummary := range childSummaries {
		childScope := &runPlannerScopeBuffer{
			scopePath:       childSummary.path,
			scopeKind:       ScopeKindSubtree,
			parentScopePath: scope.scopePath,
			totals:          childSummary.totals,
		}
		if childScope.totals.IndexableEntryCount == 0 {
			continue
		}
		// The parent subtree scan already counted each immediate child subtree
		// exactly. Reusing those totals avoids rescanning every child directory
		// when it already fits the leaf budget, while oversized children still
		// recurse and preserve the existing split semantics.
		if scopeExceedsBudget(childScope.totals, budget) {
			if err := p.preScanScope(ctx, root, childScope, budget, rootIndex, rootTotal); err != nil {
				return err
			}
			if childScope.totals.IndexableEntryCount == 0 {
				continue
			}
		}
		scope.children = append(scope.children, childScope)
	}
	return nil
}

func (p *RunPlanner) scanDirectFilesScope(ctx context.Context, root RootRecord, scopePath string) (PlanTotals, []string, error) {
	scopePath = filepath.Clean(scopePath)
	if scopePath != filepath.Clean(root.Path) && p.isExcludedPath(root.ID, scopePath) {
		// A promoted child root owns this scope now. The parent planner returns no
		// work instead of counting it as skipped, because the dynamic root will
		// produce its own progress and write units.
		return PlanTotals{}, nil, nil
	}
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
		if os.IsNotExist(err) {
			// Incremental dirty paths can be short-lived build/temp directories.
			// Treat a scope that disappears after the initial Stat as already
			// reconciled instead of turning one vanished path into a root retry.
			return PlanTotals{}, nil, nil
		}
		return PlanTotals{}, nil, fmt.Errorf("read direct-files scope %q: %w", scopePath, err)
	}

	for _, dirEntry := range dirEntries {
		select {
		case <-ctx.Done():
			return PlanTotals{}, nil, ctx.Err()
		default:
		}

		childPath := filepath.Join(scopePath, dirEntry.Name())
		isDir, _, infoErr := strictDirEntryType(scopePath, dirEntry)
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
		if isDir && p.isExcludedPath(root.ID, childPath) {
			// Dynamic child roots are sealed ownership boundaries. Do not count the
			// child directory in the parent direct-files totals; execution will also
			// skip writing its directory entry.
			continue
		}
		if isDir {
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

		totals.FileCount++
		totals.IndexableEntryCount++
		totals.PlannedScanUnits++
		totals.PlannedWriteUnits++
	}

	// Planner direct-files pre-scan only needs aggregate totals. The previous
	// implementation still accumulated and sorted every child file path even
	// though callers discarded that list, so large flat directories paid extra
	// allocations and comparisons without affecting the sealed plan.
	return totals, nil, nil
}

func (p *RunPlanner) scanSubtreeScope(ctx context.Context, root RootRecord, scopePath string) (PlanTotals, []subtreeChildSummary, error) {
	scopePath = filepath.Clean(scopePath)
	if scopePath != filepath.Clean(root.Path) && p.isExcludedPath(root.ID, scopePath) {
		// Incremental batches can be older than a promotion. If a parent batch now
		// targets a dynamic-owned scope, planning an empty parent job preserves the
		// ownership split and lets the dynamic root handle the real reconcile.
		return PlanTotals{}, nil, nil
	}
	if p != nil && p.onSubtreeScan != nil {
		p.onSubtreeScan(filepath.Clean(scopePath))
	}
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
		path       string
		childScope string
	}

	totals := PlanTotals{}
	rootChildOrder := make([]string, 0)
	rootChildTotals := make(map[string]PlanTotals)
	queue := []queueItem{{path: scopePath}}

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return PlanTotals{}, nil, ctx.Err()
		default:
		}

		current := queue[0]
		queue = queue[1:]
		if current.path != scopePath && p.isExcludedPath(root.ID, current.path) {
			// Guard against stale queued children when exclusions change during a
			// dirty run. The scanner loop is serial, but this keeps the planner's
			// per-root contract explicit and mirrors SnapshotBuilder's ownership skip.
			continue
		}

		dirEntries, readErr := os.ReadDir(current.path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				// A queued directory can vanish between the planner's Stat/ReadDir
				// calls, especially under compiler temp folders. Missing paths mean
				// there is no filesystem work left for this scope; escalating to the
				// root would make one transient delete look like a global index.
				if current.path == scopePath {
					return PlanTotals{}, nil, nil
				}
				totals.SkippedCount++
				if current.childScope != "" {
					childTotals := rootChildTotals[current.childScope]
					childTotals.SkippedCount++
					rootChildTotals[current.childScope] = childTotals
				}
				continue
			}
			// The new run planner originally failed fast on any unreadable child
			// directory so its pre-scan counts stayed exact. That was too strict for
			// real Windows roots such as C:\Windows, where protected children like
			// CSC should be skipped rather than aborting the whole root. We still
			// fail if the scope root itself is unreadable, but unreadable descendants
			// are now treated as skipped work so the rest of the root can index.
			if current.path != scopePath && shouldSkipUnreadableTraversalError(readErr) {
				totals.SkippedCount++
				if current.childScope != "" {
					childTotals := rootChildTotals[current.childScope]
					childTotals.SkippedCount++
					rootChildTotals[current.childScope] = childTotals
				}
				util.GetLogger().Warn(ctx, "filesearch skipped unreadable subtree path "+current.path+": "+readErr.Error())
				continue
			}
			return PlanTotals{}, nil, fmt.Errorf("read subtree scope %q: %w", current.path, readErr)
		}

		totals.DirectoryCount++
		totals.IndexableEntryCount++
		totals.PlannedScanUnits++
		totals.PlannedWriteUnits++
		if current.childScope != "" {
			childTotals := rootChildTotals[current.childScope]
			childTotals.DirectoryCount++
			childTotals.IndexableEntryCount++
			childTotals.PlannedScanUnits++
			childTotals.PlannedWriteUnits++
			rootChildTotals[current.childScope] = childTotals
		}

		for _, dirEntry := range dirEntries {
			childPath := filepath.Join(current.path, dirEntry.Name())
			isDir, _, infoErr := strictDirEntryType(current.path, dirEntry)
			if infoErr != nil {
				if os.IsNotExist(infoErr) {
					// Children can disappear after ReadDir returns their names. That
					// is normal churn in temp/build trees, so skip the stale directory
					// entry instead of failing the whole incremental plan.
					continue
				}
				if shouldSkipUnreadableTraversalError(infoErr) {
					totals.SkippedCount++
					if current.childScope != "" {
						childTotals := rootChildTotals[current.childScope]
						childTotals.SkippedCount++
						rootChildTotals[current.childScope] = childTotals
					}
					util.GetLogger().Warn(ctx, "filesearch skipped unreadable subtree child "+childPath+": "+infoErr.Error())
					continue
				}
				return PlanTotals{}, nil, infoErr
			}
			if isDir && p.isExcludedPath(root.ID, childPath) {
				// Excluded dynamic roots must not appear as child scopes or totals
				// under the parent. Counting them here would make the sealed plan
				// disagree with execution and reopen the parent-ownership bug.
				continue
			}

			if shouldSkipSystemPath(childPath, isDir) {
				totals.SkippedCount++
				if current.childScope != "" {
					childTotals := rootChildTotals[current.childScope]
					childTotals.SkippedCount++
					rootChildTotals[current.childScope] = childTotals
				}
				continue
			}
			if !p.policy.shouldIndexPath(root, childPath, isDir) {
				totals.SkippedCount++
				if current.childScope != "" {
					childTotals := rootChildTotals[current.childScope]
					childTotals.SkippedCount++
					rootChildTotals[current.childScope] = childTotals
				}
				continue
			}

			if isDir {
				childScope := current.childScope
				if current.path == scopePath {
					childScope = childPath
					if _, exists := rootChildTotals[childScope]; !exists {
						rootChildOrder = append(rootChildOrder, childScope)
					}
				}
				queue = append(queue, queueItem{
					path:       childPath,
					childScope: childScope,
				})
				continue
			}

			totals.FileCount++
			totals.IndexableEntryCount++
			totals.PlannedScanUnits++
			totals.PlannedWriteUnits++
			if current.childScope != "" {
				childTotals := rootChildTotals[current.childScope]
				childTotals.FileCount++
				childTotals.IndexableEntryCount++
				childTotals.PlannedScanUnits++
				childTotals.PlannedWriteUnits++
				rootChildTotals[current.childScope] = childTotals
			}
		}
	}

	childSummaries := make([]subtreeChildSummary, 0, len(rootChildOrder))
	for _, childScope := range rootChildOrder {
		childSummaries = append(childSummaries, subtreeChildSummary{
			path:   childScope,
			totals: rootChildTotals[childScope],
		})
	}
	return totals, childSummaries, nil
}

func (p *RunPlanner) isExcludedPath(rootID string, path string) bool {
	if p == nil || len(p.rootExclusions) == 0 {
		return false
	}
	for _, excludedPath := range p.rootExclusions[rootID] {
		if pathWithinScope(excludedPath, path) {
			return true
		}
	}
	return false
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

func strictDirEntryType(parentPath string, dirEntry os.DirEntry) (bool, os.FileInfo, error) {
	// Planner and snapshot traversals used to call Info() for every child even
	// when DirEntry.Type() already proved whether the child was a file or a
	// directory. Reusing the cheap type bits removes a large amount of metadata
	// I/O during pre-scan, while symlinks and unknown entries still fall back to
	// Info() so the previous target-kind behavior stays intact.
	modeType := dirEntry.Type()
	if modeType != 0 && modeType&os.ModeSymlink == 0 {
		return dirEntry.IsDir(), nil, nil
	}

	info, err := strictDirEntryInfo(parentPath, dirEntry)
	if err != nil {
		return false, nil, err
	}
	return info.IsDir(), info, nil
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
