package filesearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"wox/util"

	"github.com/google/uuid"
)

type SearchHandle interface {
	Cancel()
}

type searchHandle struct {
	cancel context.CancelFunc
}

func (h searchHandle) Cancel() {
	h.cancel()
}

type Engine struct {
	db              *FileSearchDB
	localProvider   *LocalIndexProvider
	providers       []SearchProvider
	scanner         *Scanner
	statusListeners *util.HashMap[string, func(StatusSnapshot)]
}

func NewEngine(ctx context.Context) (*Engine, error) {
	db, err := NewFileSearchDB(ctx)
	if err != nil {
		return nil, err
	}

	localProvider := NewLocalIndexProvider()
	engine := &Engine{
		db:              db,
		localProvider:   localProvider,
		statusListeners: util.NewHashMap[string, func(StatusSnapshot)](),
	}

	engine.scanner = NewScanner(db, localProvider)
	engine.scanner.SetStateChangeHandler(engine.notifyStatusChanged)

	if err := engine.reloadLocalEntries(ctx); err != nil {
		db.Close()
		return nil, err
	}

	engine.providers = append([]SearchProvider{localProvider}, NewSystemProviders()...)
	engine.scanner.Start(util.NewTraceContext())
	util.GetLogger().Info(ctx, fmt.Sprintf("filesearch engine initialized: providers=%d", len(engine.providers)))

	return engine, nil
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
		existing.UpdatedAt = now
		existing.Status = RootStatusIdle
		if err := e.db.UpsertRoot(ctx, *existing); err != nil {
			return err
		}
	} else {
		root := RootRecord{
			ID:        uuid.NewString(),
			Path:      cleaned,
			Kind:      RootKindUser,
			Status:    RootStatusIdle,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := e.db.UpsertRoot(ctx, root); err != nil {
			return err
		}
	}

	e.scanner.RequestRescan()
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

	e.scanner.RequestRescan()
	return nil
}

func (e *Engine) ListRoots(ctx context.Context) ([]RootRecord, error) {
	return e.db.ListRoots(ctx)
}

func (e *Engine) GetStatus(ctx context.Context) (StatusSnapshot, error) {
	roots, err := e.db.ListRoots(ctx)
	if err != nil {
		return StatusSnapshot{}, err
	}

	status := StatusSnapshot{
		RootCount: len(roots),
	}
	for _, root := range roots {
		progressCurrent, progressTotal := normalizeRootProgress(root)
		status.ProgressCurrent += progressCurrent
		status.ProgressTotal += progressTotal

		switch root.Status {
		case RootStatusScanning:
			status.ScanningRootCount++
		case RootStatusError:
			status.ErrorRootCount++
			if status.LastError == "" && root.LastError != nil {
				status.LastError = strings.TrimSpace(*root.LastError)
			}
		}
	}

	status.IsInitialIndexing = status.RootCount > 0 && status.ProgressCurrent == 0 && status.ScanningRootCount > 0
	status.IsIndexing = status.ScanningRootCount > 0 || status.IsInitialIndexing
	return status, nil
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
	case RootStatusScanning:
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
			Status:    RootStatusIdle,
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
		e.scanner.RequestRescan()
	}

	return nil
}

func (e *Engine) SearchStream(ctx context.Context, query SearchQuery, limit int, onUpdate func(SearchUpdate)) SearchHandle {
	query.Raw = normalizeQuery(query.Raw)
	streamCtx, cancel := context.WithCancel(ctx)
	queryID := uuid.NewString()

	go e.runSearch(streamCtx, queryID, query, limit, onUpdate)
	return searchHandle{cancel: cancel}
}

func (e *Engine) SearchOnce(ctx context.Context, query SearchQuery, limit int) ([]SearchResult, error) {
	query.Raw = normalizeQuery(query.Raw)
	if query.Raw == "" {
		return []SearchResult{}, nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()

	var (
		lastResults []SearchResult
		lastErr     error
		done        = make(chan struct{})
	)

	e.SearchStream(waitCtx, query, limit, func(update SearchUpdate) {
		lastResults = update.Results
		if update.IsFinal {
			close(done)
		}
	})

	select {
	case <-done:
		return lastResults, lastErr
	case <-waitCtx.Done():
		return lastResults, lastErr
	}
}

func (e *Engine) runSearch(ctx context.Context, queryID string, query SearchQuery, limit int, onUpdate func(SearchUpdate)) {
	if query.Raw == "" {
		onUpdate(SearchUpdate{QueryID: queryID, Stage: SearchStageFinal, Results: []SearchResult{}, IsFinal: true})
		return
	}

	aggregator := newResultAggregator(limit)
	type providerResponse struct {
		name       string
		candidates []ProviderCandidate
		err        error
	}

	responses := make(chan providerResponse, len(e.providers))
	var waitGroup sync.WaitGroup
	for _, provider := range e.providers {
		provider := provider
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()

			providerCtx := ctx
			if provider.Name() != "local-index" {
				var cancel context.CancelFunc
				providerCtx, cancel = context.WithTimeout(ctx, 150*time.Millisecond)
				defer cancel()
			}

			candidates, err := provider.Search(providerCtx, query, limit)
			responses <- providerResponse{name: provider.Name(), candidates: candidates, err: err}
		}()
	}

	go func() {
		waitGroup.Wait()
		close(responses)
	}()

	hasUpdate := false
	for response := range responses {
		if response.err != nil && !errorsIsCanceled(response.err) {
			util.GetLogger().Warn(ctx, "filesearch provider "+response.name+" failed: "+response.err.Error())
		}

		results, changed := aggregator.Add(response.candidates)
		if changed {
			stage := SearchStagePartial
			if hasUpdate {
				stage = SearchStageUpdated
			}
			hasUpdate = true
			onUpdate(SearchUpdate{
				QueryID: queryID,
				Stage:   stage,
				Results: results,
				IsFinal: false,
			})
		}
	}

	onUpdate(SearchUpdate{
		QueryID: queryID,
		Stage:   SearchStageFinal,
		Results: aggregator.snapshot(),
		IsFinal: true,
	})
}

func (e *Engine) reloadLocalEntries(ctx context.Context) error {
	entries, err := e.db.ListEntries(ctx)
	if err != nil {
		return err
	}
	e.localProvider.ReplaceEntries(entries)
	return nil
}

func errorsIsCanceled(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}
