package system

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"wox/util"
	"wox/util/filesearch"
)

type fileSearchIndexPolicy struct {
	mu            sync.RWMutex
	patternsByDir map[string][]gitIgnorePattern
}

func newFileSearchIndexPolicy() *fileSearchIndexPolicy {
	return &fileSearchIndexPolicy{
		patternsByDir: map[string][]gitIgnorePattern{},
	}
}

func (p *fileSearchIndexPolicy) toFilesearchPolicy() filesearch.Policy {
	return filesearch.Policy{
		ShouldIndexPath:     p.shouldIndexPath,
		ShouldProcessChange: p.shouldProcessChange,
	}
}

func (p *fileSearchIndexPolicy) shouldIndexPath(root filesearch.RootRecord, path string, isDir bool) bool {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" {
		return true
	}

	if shouldIgnoreFileSearchSystemPath(cleanPath, isDir) {
		return false
	}

	return !p.shouldIgnoreByGitIgnore(filepath.Clean(root.Path), cleanPath, isDir)
}

func (p *fileSearchIndexPolicy) shouldProcessChange(root filesearch.RootRecord, change filesearch.ChangeSignal) bool {
	if change.Path == "" {
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

func shouldIgnoreFileSearchSystemPath(fullPath string, isDir bool) bool {
	base := strings.ToLower(filepath.Base(fullPath))
	if base == ".ds_store" {
		return true
	}

	if !isDir {
		return false
	}

	if base == ".git" || base == ".hg" || base == ".svn" {
		return true
	}

	if !util.IsMacOS() {
		return false
	}

	return strings.HasSuffix(base, ".photoslibrary") ||
		strings.HasSuffix(base, ".lrlibrary") ||
		strings.HasSuffix(base, ".lrdata")
}

func (p *fileSearchIndexPolicy) shouldIgnoreByGitIgnore(rootPath string, fullPath string, isDir bool) bool {
	rootPath = filepath.Clean(strings.TrimSpace(rootPath))
	fullPath = filepath.Clean(strings.TrimSpace(fullPath))
	if rootPath == "" || fullPath == "" || !pathWithinRoot(rootPath, fullPath) || fullPath == rootPath {
		return false
	}

	ignored := false
	for _, directory := range patternDirectoriesForPath(rootPath, fullPath) {
		for _, pattern := range p.patternsForDirectory(directory) {
			if pattern.matches(fullPath, isDir) {
				ignored = !pattern.negate
			}
		}
	}

	return ignored
}

func (p *fileSearchIndexPolicy) patternsForDirectory(directory string) []gitIgnorePattern {
	directory = filepath.Clean(strings.TrimSpace(directory))
	if directory == "" {
		return nil
	}

	p.mu.RLock()
	patterns, ok := p.patternsByDir[directory]
	p.mu.RUnlock()
	if ok {
		return patterns
	}

	loaded := loadGitIgnorePatterns(directory)

	p.mu.Lock()
	if existing, ok := p.patternsByDir[directory]; ok {
		p.mu.Unlock()
		return existing
	}
	p.patternsByDir[directory] = loaded
	p.mu.Unlock()

	return loaded
}

func patternDirectoriesForPath(rootPath string, fullPath string) []string {
	parent := filepath.Dir(fullPath)
	if !pathWithinRoot(rootPath, parent) {
		return nil
	}

	reversed := make([]string, 0, 8)
	for current := filepath.Clean(parent); ; current = filepath.Dir(current) {
		reversed = append(reversed, current)
		if current == rootPath {
			break
		}
		next := filepath.Dir(current)
		if next == current || !pathWithinRoot(rootPath, next) {
			break
		}
	}

	directories := make([]string, 0, len(reversed))
	for index := len(reversed) - 1; index >= 0; index-- {
		directories = append(directories, reversed[index])
	}

	return directories
}

func pathWithinRoot(rootPath string, candidatePath string) bool {
	rel, err := filepath.Rel(filepath.Clean(rootPath), filepath.Clean(candidatePath))
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}

	parentPrefix := ".." + string(filepath.Separator)
	return rel != ".." && !strings.HasPrefix(rel, parentPrefix)
}

type gitIgnorePattern struct {
	baseDir  string
	pattern  string
	negate   bool
	dirOnly  bool
	rooted   bool
	hasSlash bool
}

func loadGitIgnorePatterns(directory string) []gitIgnorePattern {
	gitIgnorePath := filepath.Join(directory, ".gitignore")
	data, err := os.ReadFile(gitIgnorePath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	patterns := make([]gitIgnorePattern, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		pattern := gitIgnorePattern{baseDir: directory}
		if strings.HasPrefix(line, "!") {
			pattern.negate = true
			line = strings.TrimPrefix(line, "!")
		}
		if strings.HasPrefix(line, "/") {
			pattern.rooted = true
			line = strings.TrimPrefix(line, "/")
		}
		if strings.HasSuffix(line, "/") {
			pattern.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		pattern.pattern = line
		pattern.hasSlash = strings.Contains(line, "/")
		if pattern.pattern != "" {
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}

func (p gitIgnorePattern) matches(fullPath string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}

	relPath, err := filepath.Rel(p.baseDir, fullPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	pattern := filepath.ToSlash(p.pattern)

	if p.rooted || p.hasSlash {
		if ok, _ := filepath.Match(pattern, relPath); ok {
			return true
		}
		return strings.HasPrefix(relPath, pattern+"/")
	}

	for _, segment := range strings.Split(relPath, "/") {
		if ok, _ := filepath.Match(pattern, segment); ok {
			return true
		}
	}

	return false
}
