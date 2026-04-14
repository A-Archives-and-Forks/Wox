package filesearch

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type DirtyQueueConfig struct {
	DebounceWindow               time.Duration
	SiblingMergeThreshold        int
	RootEscalationPathThreshold  int
	RootEscalationDirectoryRatio float64
}

type DirtyQueue struct {
	config  DirtyQueueConfig
	mu      sync.Mutex
	pending map[string][]DirtySignal
}

func NewDirtyQueue(config DirtyQueueConfig) *DirtyQueue {
	return &DirtyQueue{
		config:  config,
		pending: map[string][]DirtySignal{},
	}
}

func (q *DirtyQueue) Push(signal DirtySignal) {
	normalized, ok := normalizeDirtySignal(signal)
	if !ok {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.pending == nil {
		q.pending = map[string][]DirtySignal{}
	}
	q.pending[normalized.RootID] = append(q.pending[normalized.RootID], normalized)
}

func (q *DirtyQueue) FlushReady(now time.Time, rootDirectoryCounts map[string]int) []ReconcileBatch {
	q.mu.Lock()
	defer q.mu.Unlock()

	rootIDs := make([]string, 0, len(q.pending))
	for rootID, signals := range q.pending {
		if len(signals) == 0 {
			continue
		}
		latest := latestDirtySignalAt(signals)
		if q.debounceWindow() > 0 && now.Sub(latest) < q.debounceWindow() {
			continue
		}
		rootIDs = append(rootIDs, rootID)
	}
	sort.Strings(rootIDs)

	batches := make([]ReconcileBatch, 0, len(rootIDs))
	for _, rootID := range rootIDs {
		signals := q.pending[rootID]
		delete(q.pending, rootID)
		batches = append(batches, buildReconcileBatch(rootID, signals, rootDirectoryCounts[rootID], q.config))
	}

	return batches
}

func (q *DirtyQueue) debounceWindow() time.Duration {
	return q.config.DebounceWindow
}

func buildReconcileBatch(rootID string, signals []DirtySignal, directoryCount int, config DirtyQueueConfig) ReconcileBatch {
	if len(signals) == 0 {
		return ReconcileBatch{RootID: rootID}
	}

	latestSignal := latestDirtySignal(signals)

	for _, signal := range signals {
		if signal.Kind == DirtySignalKindRoot {
			return ReconcileBatch{
				RootID:         rootID,
				TraceID:        latestSignal.TraceID,
				Mode:           ReconcileModeRoot,
				DirtyPathCount: len(signals),
			}
		}
	}

	dirtyPaths := uniqueDirtyPaths(signals)
	paths := coalesceDirtyPaths(dirtyPaths, config.SiblingMergeThreshold)
	if shouldEscalateRoot(len(paths), directoryCount, config) {
		return ReconcileBatch{
			RootID:         rootID,
			TraceID:        latestSignal.TraceID,
			Mode:           ReconcileModeRoot,
			DirtyPathCount: len(signals),
		}
	}

	return ReconcileBatch{
		RootID:         rootID,
		TraceID:        latestSignal.TraceID,
		Mode:           ReconcileModeSubtree,
		Paths:          paths,
		DirtyPathCount: len(signals),
	}
}

func coalesceDirtyPaths(dirtyPaths []string, siblingMergeThreshold int) []string {
	if len(dirtyPaths) == 0 {
		return nil
	}

	if siblingMergeThreshold < 2 {
		siblingMergeThreshold = 2
	}

	tree := newDirtyPathTree()
	for _, dirtyPath := range dirtyPaths {
		tree.add(dirtyPath)
	}

	return tree.reduce(siblingMergeThreshold)
}

func shouldEscalateRoot(pathCount int, directoryCount int, config DirtyQueueConfig) bool {
	if config.RootEscalationPathThreshold > 0 && pathCount >= config.RootEscalationPathThreshold {
		return true
	}
	if config.RootEscalationDirectoryRatio <= 0 || directoryCount <= 0 {
		return false
	}
	return float64(pathCount) > float64(directoryCount)*config.RootEscalationDirectoryRatio
}

func normalizeDirtySignal(signal DirtySignal) (DirtySignal, bool) {
	signal.Path = cleanDirtyQueuePath(signal.Path)
	if signal.RootID == "" {
		return DirtySignal{}, false
	}

	switch signal.Kind {
	case DirtySignalKindRoot:
		if signal.Path == "" {
			// Root dirties are valid without a path.
		}
	case DirtySignalKindPath:
		if signal.Path == "" {
			return DirtySignal{}, false
		}
	default:
		if signal.Path == "" {
			return DirtySignal{}, false
		}
		signal.Kind = DirtySignalKindPath
	}

	if signal.At.IsZero() {
		signal.At = time.Now()
	}

	return signal, true
}

func uniqueDirtyPaths(signals []DirtySignal) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(signals))
	for _, signal := range signals {
		dirtyPath, ok := dirtyPathForSignal(signal)
		if !ok {
			continue
		}
		if _, exists := seen[dirtyPath]; exists {
			continue
		}
		seen[dirtyPath] = struct{}{}
		paths = append(paths, dirtyPath)
	}

	sort.Strings(paths)
	return paths
}

func dirtyPathForSignal(signal DirtySignal) (string, bool) {
	switch signal.Kind {
	case DirtySignalKindRoot:
		return cleanDirtyQueuePath(signal.Path), true
	case DirtySignalKindPath:
		return scopePathForDirtySignal(signal.Path, signal.PathIsDir, signal.PathTypeKnown)
	default:
		return scopePathForDirtySignal(signal.Path, signal.PathIsDir, signal.PathTypeKnown)
	}
}

func scopePathForDirtySignal(path string, pathIsDir bool, pathTypeKnown bool) (string, bool) {
	path = cleanDirtyQueuePath(path)
	if path == "" {
		return "", false
	}

	if pathTypeKnown && pathIsDir {
		return path, true
	}

	parent := filepath.Dir(path)
	if parent == "." || parent == path || parent == string(filepath.Separator) && path == parent {
		return path, true
	}

	return parent, true
}

type dirtyPathTree struct {
	root *dirtyPathNode
}

type dirtyPathNode struct {
	path     string
	dirty    bool
	children map[string]*dirtyPathNode
}

func newDirtyPathTree() *dirtyPathTree {
	return &dirtyPathTree{
		root: &dirtyPathNode{
			path:     "",
			children: map[string]*dirtyPathNode{},
		},
	}
}

func (t *dirtyPathTree) add(path string) {
	if path == "" {
		return
	}

	node := t.root
	rootPath, segments := splitDirtyPath(path)
	if rootPath != "" {
		if node.children == nil {
			node.children = map[string]*dirtyPathNode{}
		}
		child := node.children[rootPath]
		if child == nil {
			child = &dirtyPathNode{
				path:     rootPath,
				children: map[string]*dirtyPathNode{},
			}
			node.children[rootPath] = child
		}
		node = child
	}

	if len(segments) == 0 {
		node.dirty = true
		return
	}

	for _, segment := range segments {
		if node.children == nil {
			node.children = map[string]*dirtyPathNode{}
		}
		child := node.children[segment]
		if child == nil {
			child = &dirtyPathNode{
				path:     joinDirtyPath(node.path, segment),
				children: map[string]*dirtyPathNode{},
			}
			node.children[segment] = child
		}
		node = child
	}
	node.dirty = true
}

func (t *dirtyPathTree) reduce(threshold int) []string {
	if threshold < 2 {
		threshold = 2
	}
	return reduceDirtyPathNode(t.root, threshold)
}

func reduceDirtyPathNode(node *dirtyPathNode, threshold int) []string {
	if node == nil {
		return nil
	}
	if node.dirty {
		if node.path == "" {
			return nil
		}
		return []string{node.path}
	}

	childKeys := make([]string, 0, len(node.children))
	for key := range node.children {
		childKeys = append(childKeys, key)
	}
	sort.Strings(childKeys)

	paths := make([]string, 0, len(node.children))
	for _, key := range childKeys {
		paths = append(paths, reduceDirtyPathNode(node.children[key], threshold)...)
	}

	if len(paths) >= threshold {
		return []string{collapseDirtyPath(node)}
	}

	return paths
}

func latestDirtySignalAt(signals []DirtySignal) time.Time {
	latest := signals[0].At
	for i := 1; i < len(signals); i++ {
		if signals[i].At.After(latest) {
			latest = signals[i].At
		}
	}
	return latest
}

func latestDirtySignal(signals []DirtySignal) DirtySignal {
	latest := signals[0]
	for i := 1; i < len(signals); i++ {
		if signals[i].At.After(latest.At) {
			latest = signals[i]
		}
	}
	return latest
}

func cleanDirtyQueuePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(value)
}

func splitDirtyPath(path string) (string, []string) {
	root := filepath.VolumeName(path)
	trimmed := path
	if root != "" {
		trimmed = strings.TrimPrefix(trimmed, root)
	}
	if strings.HasPrefix(trimmed, string(filepath.Separator)) {
		root += string(filepath.Separator)
		trimmed = strings.TrimPrefix(trimmed, string(filepath.Separator))
	}

	parts := strings.Split(trimmed, string(filepath.Separator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return root, out
}

func joinDirtyPath(parent, segment string) string {
	if parent == "" {
		return segment
	}
	return filepath.Join(parent, segment)
}

func collapseDirtyPath(node *dirtyPathNode) string {
	if node == nil {
		return ""
	}
	if node.path != "" {
		return node.path
	}
	if len(node.children) == 1 {
		for _, child := range node.children {
			return child.path
		}
	}
	return string(filepath.Separator)
}
