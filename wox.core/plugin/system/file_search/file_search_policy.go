package system

import (
	"os"
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
		ShouldIndexPath:     p.shouldIndexPath,
		ShouldProcessChange: p.shouldProcessChange,
	}
}

func (p *fileSearchIndexPolicy) shouldIndexPath(root filesearch.RootRecord, path string, isDir bool) bool {
	if p == nil || p.inner == nil {
		return true
	}
	return p.inner.ShouldIndexPath(root.Path, root.PolicyRootPath, path, isDir)
}

func (p *fileSearchIndexPolicy) SetIgnorePatterns(patterns []string) {
	if p == nil || p.inner == nil {
		return
	}
	p.inner.SetIgnorePatterns(patterns)
}

func (p *fileSearchIndexPolicy) shouldProcessChange(root filesearch.RootRecord, change filesearch.ChangeSignal) bool {
	if strings.TrimSpace(change.Path) == "" {
		return true
	}

	isDir := change.PathIsDir
	if !change.PathTypeKnown {
		if info, err := os.Stat(change.Path); err == nil {
			isDir = info.IsDir()
		}
	}

	return p.shouldIndexPath(root, change.Path, isDir)
}
