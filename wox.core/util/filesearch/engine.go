package filesearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"wox/util"

	"github.com/google/uuid"
)

type Engine struct {
	db              *FileSearchDB
	searchProvider  *SQLiteSearchProvider
	scanner         *Scanner
	policy          *policyState
	statusListeners *util.HashMap[string, func(StatusSnapshot)]
}

func NewEngine(ctx context.Context) (*Engine, error) {
	return NewEngineWithOptions(ctx, DefaultEngineOptions())
}

func NewEngineWithOptions(ctx context.Context, options EngineOptions) (*Engine, error) {
	db, err := NewFileSearchDB(ctx)
	if err != nil {
		return nil, err
	}

	engine := &Engine{
		db:              db,
		searchProvider:  NewSQLiteSearchProvider(db),
		statusListeners: util.NewHashMap[string, func(StatusSnapshot)](),
	}

	engine.scanner = NewScanner(db)
	engine.policy = engine.scanner.policy
	if engine.policy != nil {
		engine.policy.Set(options.Policy)
	}
	engine.scanner.SetStateChangeHandler(engine.notifyStatusChanged)

	// Keep the built-in file engine focused on the persisted SQLite search index.
	// The previous runtime mirrored every entry into a second in-memory provider,
	// which doubled storage responsibilities and made memory usage scale with the
	// number of indexed roots.
	engine.scanner.Start(util.NewTraceContext())
	util.GetLogger().Info(ctx, "filesearch engine initialized: indexed_provider=sqlite-search")
	engine.logInitSnapshotAsync(ctx)

	return engine, nil
}

func (e *Engine) logInitSnapshotAsync(ctx context.Context) {
	if e == nil || e.db == nil {
		return
	}

	// Capture the heavy SQLite snapshot after engine init returns because the
	// previous synchronous fts5vocab sampling blocked plugin initialization on
	// large databases. That prevented the file plugin instance from registering,
	// so `f ` stopped entering file-plugin query mode even though the engine was
	// otherwise healthy.
	util.Go(ctx, "filesearch init sqlite snapshot", func() {
		snapshotCtx, cancel := context.WithTimeout(util.NewTraceContext(), 30*time.Second)
		defer cancel()

		snapshot, err := e.db.SearchIndexSnapshot(snapshotCtx)
		if err != nil {
			util.GetLogger().Warn(snapshotCtx, "filesearch failed to capture sqlite snapshot during init: "+err.Error())
			return
		}
		logSQLiteIndexSnapshot(snapshotCtx, "engine_initialized", snapshot, true)
	})
}

func (e *Engine) UpdatePolicy(policy Policy) {
	if e == nil {
		return
	}
	if e.policy != nil {
		e.policy.Set(policy)
	}
	if e.scanner != nil {
		e.scanner.RequestRescan(util.NewTraceContext())
	}
}

func (e *Engine) Close() error {
	if e.scanner != nil {
		e.scanner.Stop()
	}
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

func (e *Engine) AddRoot(ctx context.Context, rootPath string) error {
	cleaned := filepath.Clean(rootPath)
	info, err := os.Stat(cleaned)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("filesearch root is not a directory: %s", cleaned)
	}

	existing, err := e.db.FindRootByPath(ctx, cleaned)
	if err != nil {
		return err
	}
	now := util.GetSystemTimestamp()
	if existing != nil {
		existing.Kind = RootKindUser
		existing.DynamicParentRootID = ""
		existing.PolicyRootPath = ""
		existing.PromotedAt = 0
		existing.LastHotAt = 0
		existing.UpdatedAt = now
		existing.Status = RootStatusPreparing
		// A user-added path can collide with a hidden dynamic root. Clearing the
		// lifecycle fields here makes that path a real user root instead of
		// leaving stale parent-policy metadata attached to the reused row.
		if err := e.db.UpsertRoot(ctx, *existing); err != nil {
			return err
		}
	} else {
		root := RootRecord{
			ID:        uuid.NewString(),
			Path:      cleaned,
			Kind:      RootKindUser,
			Status:    RootStatusPreparing,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := e.db.UpsertRoot(ctx, root); err != nil {
			return err
		}
	}

	e.scanner.RequestRescan(ctx)
	return nil
}

func (e *Engine) RemoveRoot(ctx context.Context, rootPath string) error {
	cleaned := filepath.Clean(rootPath)
	root, err := e.db.FindRootByPath(ctx, cleaned)
	if err != nil {
		return err
	}
	if root == nil {
		return nil
	}

	if err := e.db.DeleteRoot(ctx, root.ID); err != nil {
		return err
	}

	e.scanner.RequestRescan(ctx)
	return nil
}

func (e *Engine) ListRoots(ctx context.Context) ([]RootRecord, error) {
	roots, err := e.db.ListRoots(ctx)
	if err != nil {
		return nil, err
	}
	return userVisibleRoots(roots), nil
}

func (e *Engine) GetStatus(ctx context.Context) (StatusSnapshot, error) {
	allRoots, err := e.db.ListRoots(ctx)
	if err != nil {
		return StatusSnapshot{}, err
	}
	// Dynamic roots are internal scan ownership boundaries. Status counters keep
	// reporting the user-configured root set while the scanner still uses all
	// roots for execution and diagnostics.
	roots := userVisibleRoots(allRoots)

	var transientScanState TransientRootState
	hasTransientScanState := false
	var transientSyncState TransientSyncState
	hasTransientSyncState := false

	if e.scanner != nil {
		if activeState, ok := e.scanner.GetTransientRootState(); ok {
			transientScanState = activeState
			hasTransientScanState = true
			mergeTransientRootState(roots, activeState.Root)
		}
		if activeState, ok := e.scanner.GetTransientSyncState(); ok {
			transientSyncState = activeState
			hasTransientSyncState = true
			if activeState.Root.ID != "" {
				mergeTransientRootState(roots, activeState.Root)
			}
		}
	}

	status := StatusSnapshot{
		RootCount: len(roots),
	}
	if hasTransientSyncState {
		status.PendingDirtyRootCount = transientSyncState.PendingRootCount
		status.PendingDirtyPathCount = transientSyncState.PendingPathCount
	}

	for _, root := range roots {
		progressCurrent, progressTotal := normalizeRootProgress(root)
		status.ProgressCurrent += progressCurrent
		status.ProgressTotal += progressTotal

		switch root.Status {
		case RootStatusPreparing:
			status.PreparingRootCount++
		case RootStatusScanning:
			status.ScanningRootCount++
		case RootStatusSyncing:
			status.SyncingRootCount++
		case RootStatusWriting:
			status.WritingRootCount++
		case RootStatusFinalizing:
			status.FinalizingRootCount++
		case RootStatusError:
			status.ErrorRootCount++
			if status.LastError == "" && root.LastError != nil {
				status.LastError = strings.TrimSpace(*root.LastError)
			}
			if status.ErrorRootPath == "" {
				status.ErrorRootPath = root.Path
			}
		}

		if isActiveRootStatus(root.Status) && activeRootStatusPriority(root.Status) >= activeRootStatusPriority(status.ActiveRootStatus) {
			status.ActiveRootStatus = root.Status
			status.ActiveProgressCurrent = root.ProgressCurrent
			status.ActiveProgressTotal = root.ProgressTotal
			switch {
			case hasTransientSyncState && transientSyncState.Root.ID == root.ID:
				status.ActiveRootIndex = transientSyncState.RootIndex
				status.ActiveRootTotal = transientSyncState.RootTotal
				status.ActiveDiscoveredCount = 0
				status.ActiveDirectoryIndex = transientSyncState.ScopeCount
				status.ActiveDirectoryTotal = transientSyncState.ScopeCount
				status.ActiveItemCurrent = 0
				status.ActiveItemTotal = int64(transientSyncState.DirtyPathCount)
			case hasTransientScanState && transientScanState.Root.ID == root.ID:
				status.ActiveRootIndex = transientScanState.RootIndex
				status.ActiveRootTotal = transientScanState.RootTotal
				status.ActiveDiscoveredCount = transientScanState.DiscoveredCount
				status.ActiveDirectoryIndex = transientScanState.DirectoryIndex
				status.ActiveDirectoryTotal = transientScanState.DirectoryTotal
				status.ActiveItemCurrent = transientScanState.ItemCurrent
				status.ActiveItemTotal = transientScanState.ItemTotal
			}
		}
	}

	var activeRun StatusSnapshot
	hasActiveRun := false
	if e.scanner != nil {
		if currentRun, ok := e.scanner.GetTransientRunState(); ok {
			activeRun = currentRun
			hasActiveRun = true
			mergeTransientRunStatus(&status, activeRun)
		}
	}

	// Planner/executor runs own the live indexing state. The previous code
	// merged the active run and then immediately overwrote IsIndexing from the
	// persisted root counters, which made the toolbar treat a live pre-scan as
	// "not indexing" whenever another root was already in error.
	if hasActiveRun {
		status.IsInitialIndexing = activeRun.IsIndexing &&
			(activeRun.ActiveStage == RunStagePlanning || activeRun.ActiveStage == RunStagePreScan) &&
			activeRun.ActiveProgressCurrent == 0
		status.IsIndexing = activeRun.IsIndexing
		return status, nil
	}

	status.IsInitialIndexing = status.RootCount > 0 && (status.ActiveRootStatus == RootStatusPreparing || status.ActiveRootStatus == RootStatusScanning) && status.ActiveProgressCurrent == 0 && (status.PreparingRootCount > 0 || status.ScanningRootCount > 0)
	status.IsIndexing = status.PreparingRootCount > 0 || status.ScanningRootCount > 0 || status.SyncingRootCount > 0 || status.WritingRootCount > 0 || status.FinalizingRootCount > 0 || status.IsInitialIndexing
	return status, nil
}

func mergeTransientRunStatus(status *StatusSnapshot, activeRun StatusSnapshot) {
	if status == nil {
		return
	}

	// Run-scoped progress now owns the user-facing denominator because one
	// logical root can expand into many jobs. The legacy root counters remain in
	// the snapshot as diagnostics, but active status/progress should prefer the
	// sealed run state whenever a planner/executor run is in flight.
	status.ProgressCurrent = activeRun.ProgressCurrent
	status.ProgressTotal = activeRun.ProgressTotal
	status.ActiveRootStatus = activeRun.ActiveRootStatus
	status.ActiveProgressCurrent = activeRun.ActiveProgressCurrent
	status.ActiveProgressTotal = activeRun.ActiveProgressTotal
	status.ActiveRootIndex = activeRun.ActiveRootIndex
	status.ActiveRootTotal = activeRun.ActiveRootTotal
	status.ActiveDiscoveredCount = activeRun.ActiveDiscoveredCount
	status.ActiveDirectoryIndex = activeRun.ActiveDirectoryIndex
	status.ActiveDirectoryTotal = activeRun.ActiveDirectoryTotal
	status.ActiveItemCurrent = activeRun.ActiveItemCurrent
	status.ActiveItemTotal = activeRun.ActiveItemTotal
	status.ActiveRootPath = activeRun.ActiveRootPath
	status.ActiveRunStatus = activeRun.ActiveRunStatus
	status.ActiveJobKind = activeRun.ActiveJobKind
	status.ActiveScopePath = activeRun.ActiveScopePath
	status.ActiveStage = activeRun.ActiveStage
	status.RunProgressCurrent = activeRun.RunProgressCurrent
	status.RunProgressTotal = activeRun.RunProgressTotal
	status.IsIndexing = activeRun.IsIndexing
	if strings.TrimSpace(activeRun.LastError) != "" {
		status.LastError = activeRun.LastError
	}
}

func mergeTransientRootState(roots []RootRecord, transientRoot RootRecord) {
	for index := range roots {
		if roots[index].ID == transientRoot.ID {
			roots[index] = transientRoot
			return
		}
	}
}

func userVisibleRoots(roots []RootRecord) []RootRecord {
	visible := make([]RootRecord, 0, len(roots))
	for _, root := range roots {
		if root.Kind == RootKindDynamic {
			continue
		}
		visible = append(visible, root)
	}
	return visible
}

func (e *Engine) OnStatusChanged(callback func(StatusSnapshot)) func() {
	if callback == nil {
		return func() {}
	}

	listenerId := uuid.NewString()
	e.statusListeners.Store(listenerId, callback)
	return func() {
		e.statusListeners.Delete(listenerId)
	}
}

func (e *Engine) notifyStatusChanged(ctx context.Context) {
	status, err := e.GetStatus(ctx)
	if err != nil {
		util.GetLogger().Warn(ctx, "filesearch failed to emit status changed event: "+err.Error())
		return
	}

	e.statusListeners.Range(func(_ string, callback func(StatusSnapshot)) bool {
		callback(status)
		return true
	})
}

func normalizeRootProgress(root RootRecord) (int64, int64) {
	switch root.Status {
	case RootStatusPreparing:
		return 0, RootProgressScale
	case RootStatusScanning, RootStatusSyncing, RootStatusWriting:
		total := root.ProgressTotal
		if total <= 0 || total > RootProgressScale {
			total = RootProgressScale
		}

		current := root.ProgressCurrent
		if current < 0 {
			current = 0
		}
		if current > total {
			current = total
		}

		return current, total
	case RootStatusFinalizing:
		if root.ProgressTotal > 0 {
			total := root.ProgressTotal
			if total > RootProgressScale {
				total = RootProgressScale
			}
			current := root.ProgressCurrent
			if current < 0 {
				current = 0
			}
			if current > total {
				current = total
			}
			return current, total
		}
		return RootProgressScale, RootProgressScale
	case RootStatusIdle:
		if root.ProgressTotal > 0 {
			return RootProgressScale, RootProgressScale
		}
		return 0, RootProgressScale
	case RootStatusError:
		return 0, 0
	default:
		return 0, RootProgressScale
	}
}

func isActiveRootStatus(status RootStatus) bool {
	switch status {
	case RootStatusPreparing, RootStatusScanning, RootStatusSyncing, RootStatusWriting, RootStatusFinalizing:
		return true
	default:
		return false
	}
}

func activeRootStatusPriority(status RootStatus) int {
	switch status {
	case RootStatusFinalizing:
		return 5
	case RootStatusWriting:
		return 4
	case RootStatusSyncing:
		return 3
	case RootStatusScanning:
		return 2
	case RootStatusPreparing:
		return 1
	default:
		return 0
	}
}

func (e *Engine) SyncUserRoots(ctx context.Context, rootPaths []string) error {
	desiredRoots := map[string]struct{}{}
	for _, rootPath := range rootPaths {
		cleaned := strings.TrimSpace(rootPath)
		if cleaned == "" {
			continue
		}

		cleaned = filepath.Clean(cleaned)
		info, err := os.Stat(cleaned)
		if err != nil {
			util.GetLogger().Warn(ctx, "filesearch skipped missing root "+cleaned+": "+err.Error())
			continue
		}
		if !info.IsDir() {
			util.GetLogger().Warn(ctx, "filesearch skipped non-directory root "+cleaned)
			continue
		}

		desiredRoots[cleaned] = struct{}{}
	}

	roots, err := e.db.ListRoots(ctx)
	if err != nil {
		return err
	}

	existingUserRoots := map[string]RootRecord{}
	for _, root := range roots {
		if root.Kind != RootKindUser {
			continue
		}
		existingUserRoots[filepath.Clean(root.Path)] = root
	}

	changed := false
	addedCount := 0
	removedCount := 0
	for existingPath, root := range existingUserRoots {
		if _, ok := desiredRoots[existingPath]; ok {
			continue
		}
		if err := e.db.DeleteRoot(ctx, root.ID); err != nil {
			return err
		}
		changed = true
		removedCount++
	}

	now := util.GetSystemTimestamp()
	for desiredPath := range desiredRoots {
		if _, ok := existingUserRoots[desiredPath]; ok {
			continue
		}

		root := RootRecord{
			ID:        uuid.NewString(),
			Path:      desiredPath,
			Kind:      RootKindUser,
			Status:    RootStatusPreparing,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := e.db.UpsertRoot(ctx, root); err != nil {
			return err
		}
		changed = true
		addedCount++
	}

	util.GetLogger().Info(ctx, fmt.Sprintf(
		"filesearch sync user roots: desired=%d existing=%d added=%d removed=%d changed=%v",
		len(desiredRoots),
		len(existingUserRoots),
		addedCount,
		removedCount,
		changed,
	))
	if changed && e.scanner != nil {
		e.scanner.RequestRescan(ctx)
	}

	return nil
}

func (e *Engine) Search(ctx context.Context, query SearchQuery, limit int) ([]SearchResult, error) {
	// Filesearch now has one SQLite-backed provider, so the engine stays as a
	// thin owner of lifecycle/policy state and returns the provider result
	// directly instead of preserving the old stream/aggregation wrapper.
	return e.searchProvider.Search(ctx, query, limit)
}

func (e *Engine) IndexSnapshotSummary() string {
	if e == nil || e.db == nil {
		return formatSQLiteIndexSnapshotSummary("manual", sqliteIndexSnapshot{})
	}
	snapshot, err := e.db.SearchIndexSnapshot(context.Background())
	if err != nil {
		return fmt.Sprintf("filesearch sqlite snapshot: stage=manual error=%s", err.Error())
	}
	return formatSQLiteIndexSnapshotSummary("manual", snapshot)
}

func (e *Engine) IndexTopRootsSummary() string {
	if e == nil || e.db == nil {
		return ""
	}
	snapshot, err := e.db.SearchIndexSnapshot(context.Background())
	if err != nil {
		return ""
	}
	return formatSQLiteIndexTopRoots("manual", snapshot)
}
