package filesearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const estimatedPlannerEntryBytes int64 = 256

// RunPlanner builds one sealed full-run workload in memory before execution.
// The previous root-centric flow discovered and executed work as the same loop,
// which made huge roots hold too much state at once and left progress totals
// unstable. This planner splits the work first, counts it exactly, then seals
// the immutable job list that later execution will consume.
type RunPlanner struct {
	policy *policyState
	budget splitBudget

	// planningRootBuffers are intentionally released after sealing so the planner
	// does not retain a second copy of a giant scope tree through execution.
	planningRootBuffers []*runPlannerRootBuffer
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
	for _, rootBuffer := range p.planningRootBuffers {
		if err := p.preScanRoot(ctx, rootBuffer, budget); err != nil {
			p.planningRootBuffers = nil
			return RunPlan{}, err
		}
	}

	// Phase 3: seal. We convert the planner-owned buffers into immutable plan
	// structs, deep-copy them with RunPlan.Seal, then drop the planner buffers so
	// execution does not keep the same giant scope tree alive twice.
	draft, err := p.buildDraftPlan()
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
	for _, root := range roots {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		buffer, err := p.planRoot(ctx, root)
		if err != nil {
			return nil, err
		}
		buffers = append(buffers, buffer)
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
		children: []*runPlannerScopeBuffer{{
			scopePath:       cleanRootPath,
			scopeKind:       ScopeKindDirectFiles,
			parentScopePath: cleanRootPath,
		}},
	}

	childDirs, err := p.listImmediateChildDirs(ctx, root, cleanRootPath)
	if err != nil {
		return nil, err
	}
	for _, childDir := range childDirs {
		rootScope.children = append(rootScope.children, &runPlannerScopeBuffer{
			scopePath:       childDir,
			scopeKind:       ScopeKindSubtree,
			parentScopePath: cleanRootPath,
		})
	}

	return &runPlannerRootBuffer{
		root:      root,
		rootScope: rootScope,
	}, nil
}

func (p *RunPlanner) listImmediateChildDirs(ctx context.Context, root RootRecord, dirPath string) ([]string, error) {
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("read root planning frontier %q: %w", dirPath, err)
	}

	childDirs := make([]string, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		childPath := filepath.Join(dirPath, dirEntry.Name())
		info, infoErr := strictDirEntryInfo(dirPath, dirEntry)
		if infoErr != nil {
			return nil, infoErr
		}
		if !info.IsDir() {
			continue
		}
		if shouldSkipSystemPath(childPath, true) {
			continue
		}
		if !p.policy.shouldIndexPath(root, childPath, true) {
			continue
		}
		childDirs = append(childDirs, childPath)
	}

	sort.Strings(childDirs)
	return childDirs, nil
}

func (p *RunPlanner) preScanRoot(ctx context.Context, rootBuffer *runPlannerRootBuffer, budget splitBudget) error {
	for _, child := range rootBuffer.rootScope.children {
		if err := p.preScanScope(ctx, rootBuffer.root, child, budget); err != nil {
			return err
		}
	}
	rootBuffer.rootScope.totals = aggregateChildTotals(rootBuffer.rootScope.children)
	return nil
}

func (p *RunPlanner) preScanScope(ctx context.Context, root RootRecord, scope *runPlannerScopeBuffer, budget splitBudget) error {
	switch scope.scopeKind {
	case ScopeKindDirectFiles:
		return p.preScanDirectFilesScope(ctx, root, scope, budget)
	case ScopeKindSubtree:
		return p.preScanSubtreeScope(ctx, root, scope, budget)
	default:
		return fmt.Errorf("unsupported scope kind %q", scope.scopeKind)
	}
}

func (p *RunPlanner) preScanDirectFilesScope(ctx context.Context, root RootRecord, scope *runPlannerScopeBuffer, budget splitBudget) error {
	totals, directFiles, err := p.scanDirectFilesScope(ctx, root, scope.scopePath)
	if err != nil {
		return err
	}
	scope.totals = totals
	if !scopeExceedsBudget(scope.totals, budget) || len(directFiles) == 0 {
		return nil
	}

	chunks := chunkDirectFiles(directFiles, budget)
	if len(chunks) <= 1 {
		return nil
	}

	scope.splitRequired = true
	scope.children = make([]*runPlannerScopeBuffer, 0, len(chunks))
	for index, chunk := range chunks {
		chunkTotals := PlanTotals{
			FileCount:           int64(len(chunk)),
			IndexableEntryCount: int64(len(chunk)),
			PlannedScanUnits:    int64(len(chunk)),
			PlannedWriteUnits:   int64(len(chunk)),
		}
		if index == 0 {
			chunkTotals.DirectoryCount = 1
			chunkTotals.IndexableEntryCount++
			chunkTotals.PlannedScanUnits++
			chunkTotals.PlannedWriteUnits++
		}
		scope.children = append(scope.children, &runPlannerScopeBuffer{
			scopePath:       scope.scopePath,
			scopeKind:       ScopeKindDirectFiles,
			parentScopePath: scope.parentScopePath,
			totals:          chunkTotals,
		})
	}
	return nil
}

func (p *RunPlanner) preScanSubtreeScope(ctx context.Context, root RootRecord, scope *runPlannerScopeBuffer, budget splitBudget) error {
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
	if err := p.preScanScope(ctx, root, directFilesChild, budget); err != nil {
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
		if err := p.preScanScope(ctx, root, childScope, budget); err != nil {
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

		totals.DirectoryCount++
		totals.IndexableEntryCount++
		totals.PlannedScanUnits++
		totals.PlannedWriteUnits++

		dirEntries, readErr := os.ReadDir(current.path)
		if readErr != nil {
			return PlanTotals{}, nil, fmt.Errorf("read subtree scope %q: %w", current.path, readErr)
		}

		for _, dirEntry := range dirEntries {
			childPath := filepath.Join(current.path, dirEntry.Name())
			info, infoErr := strictDirEntryInfo(current.path, dirEntry)
			if infoErr != nil {
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
				if current.path == scopePath {
					rootChildDirs = append(rootChildDirs, childPath)
				}
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

func (p *RunPlanner) buildDraftPlan() (RunPlan, error) {
	plan := RunPlan{
		PlanID:    "full-plan",
		RunID:     "full-run",
		Kind:      RunKindFull,
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

		for _, leaf := range leafScopes {
			if leaf.totals.IndexableEntryCount == 0 {
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
	if budget.DirectFileChunkSize <= 0 {
		budget.DirectFileChunkSize = defaults.DirectFileChunkSize
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

func chunkDirectFiles(directFiles []string, budget splitBudget) [][]string {
	if len(directFiles) == 0 {
		return nil
	}

	maxFilesPerChunk := budget.DirectFileChunkSize
	if budget.LeafEntryBudget > 1 && int(budget.LeafEntryBudget-1) < maxFilesPerChunk {
		maxFilesPerChunk = int(budget.LeafEntryBudget - 1)
	}
	if budget.LeafWriteBudget > 1 && int(budget.LeafWriteBudget-1) < maxFilesPerChunk {
		maxFilesPerChunk = int(budget.LeafWriteBudget - 1)
	}
	if maxFilesPerChunk <= 0 {
		maxFilesPerChunk = 1
	}

	chunks := make([][]string, 0, (len(directFiles)+maxFilesPerChunk-1)/maxFilesPerChunk)
	for start := 0; start < len(directFiles); start += maxFilesPerChunk {
		end := start + maxFilesPerChunk
		if end > len(directFiles) {
			end = len(directFiles)
		}
		chunks = append(chunks, directFiles[start:end])
	}
	return chunks
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
