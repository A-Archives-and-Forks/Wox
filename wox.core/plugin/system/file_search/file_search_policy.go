package system

import (
	"os"
	"path/filepath"
	"strings"
	"wox/plugin/system/file_search/indexpolicy"
	"wox/util/filesearch"
)

type fileSearchIndexPolicy struct {
	// Boundary change: the matching implementation lives in a small plugin-owned
	// package so real-index benchmarks can use the same rules without importing
	// the full system plugin and creating a filesearch engine cycle.
	inner *indexpolicy.Policy
}

var defaultFileSearchIgnorePatterns = indexpolicy.DefaultIgnorePatterns()

func newFileSearchIndexPolicy() *fileSearchIndexPolicy {
	return &fileSearchIndexPolicy{inner: indexpolicy.New()}
}

func (p *fileSearchIndexPolicy) toFilesearchPolicy() filesearch.Policy {
	return filesearch.Policy{
		NewTraversalContext: p.newTraversalContext,
		ShouldProcessChange: p.shouldProcessChange,
	}
}

func (p *fileSearchIndexPolicy) newTraversalContext(root filesearch.RootRecord, scopePath string) filesearch.TraversalPolicyContext {
	if p == nil || p.inner == nil {
		return nil
	}
	context := p.inner.NewTraversalContext(root.Path, root.PolicyRootPath, scopePath)
	if context == nil {
		return nil
	}
	return fileSearchTraversalPolicyContext{inner: context}
}

type fileSearchTraversalPolicyContext struct {
	inner *indexpolicy.TraversalContext
}

func (c fileSearchTraversalPolicyContext) ShouldIndexPath(path string, isDir bool) bool {
	if c.inner == nil {
		return true
	}
	return c.inner.ShouldIndexPath(path, isDir)
}

func (c fileSearchTraversalPolicyContext) Descend(directoryPath string) filesearch.TraversalPolicyContext {
	if c.inner == nil {
		return c
	}
	// Optimization boundary: util/filesearch only knows the generic traversal
	// interface, while plugin/system/file_search owns the real ignore matcher.
	// The adapter keeps that dependency direction intact and still lets the core
	// scanner carry incremental .gitignore/configured-rule state.
	return fileSearchTraversalPolicyContext{inner: c.inner.Descend(directoryPath)}
}

func (p *fileSearchIndexPolicy) SetIgnorePatterns(patterns []string) {
	if p == nil || p.inner == nil {
		return
	}
	p.inner.SetIgnorePatterns(patterns)
}

func (p *fileSearchIndexPolicy) shouldProcessChange(root filesearch.RootRecord, change filesearch.ChangeSignal) bool {
	cleanPath := filepath.Clean(strings.TrimSpace(change.Path))
	if cleanPath == "" || cleanPath == "." {
		return true
	}
	if cleanPath == filepath.Clean(root.Path) {
		return true
	}

	isDir := change.PathIsDir
	if !change.PathTypeKnown {
		if info, err := os.Stat(cleanPath); err == nil {
			isDir = info.IsDir()
		}
	}

	context := p.newTraversalContext(root, filepath.Dir(cleanPath))
	if context == nil {
		return true
	}
	// Bug fix: change-signal filtering now uses the same traversal context as
	// full indexing. The previous direct callback kept a second ignore path alive,
	// so future matcher optimizations could make full scans and watcher events
	// disagree.
	return context.ShouldIndexPath(cleanPath, isDir)
}
