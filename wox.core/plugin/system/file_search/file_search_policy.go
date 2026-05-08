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

	policyRootPath := strings.TrimSpace(root.PolicyRootPath)
	if policyRootPath == "" {
		policyRootPath = root.Path
	}
	// Dynamic roots keep their own scan scope but must inherit the user's
	// parent .gitignore chain. Using PolicyRootPath for ignore lookup preserves
	// that policy without widening the scanner's ownership boundary.
	return !p.shouldIgnoreByGitIgnore(filepath.Clean(policyRootPath), cleanPath, isDir)
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
	cleanPath := filepath.Clean(strings.TrimSpace(fullPath))
	base := strings.ToLower(filepath.Base(cleanPath))
	if base == ".ds_store" {
		return true
	}

	if hasFileSearchSystemDirectorySegment(cleanPath) {
		return true
	}

	if hasFileSearchGeneratedDirectorySegment(cleanPath) {
		return true
	}

	if util.IsMacOS() && hasMacPackageDirectorySegment(cleanPath) {
		return true
	}

	if !isDir {
		return false
	}

	if !util.IsMacOS() {
		return false
	}

	return strings.HasSuffix(base, ".photoslibrary") ||
		strings.HasSuffix(base, ".lrlibrary") ||
		strings.HasSuffix(base, ".lrdata")
}

func hasFileSearchSystemDirectorySegment(fullPath string) bool {
	for _, segment := range splitFileSearchPathSegments(fullPath) {
		switch strings.ToLower(segment) {
		case ".git", ".hg", ".svn":
			// macOS FSEvents can report descendants such as .git/objects/... without
			// first reporting the .git directory itself. Checking every path segment
			// keeps repository internals out of the dirty queue instead of only
			// ignoring direct .git directory scan entries.
			return true
		}
	}

	return false
}

func hasFileSearchGeneratedDirectorySegment(fullPath string) bool {
	for _, segment := range splitFileSearchPathSegments(fullPath) {
		switch strings.ToLower(segment) {
		case "build", "dist", "node_modules", ".dart_tool", ".gradle", ".swiftpm", ".build", "deriveddata":
			// Optimization: build tools can emit hundreds of FSEvents per second under
			// generated directories. These paths are noisy search results and were the
			// main source of repeated incremental file-search runs on macOS, so ignore
			// them at the segment level before they reach the dirty queue.
			return true
		}
	}

	return false
}

func hasMacPackageDirectorySegment(fullPath string) bool {
	for _, segment := range splitFileSearchPathSegments(fullPath) {
		lowerSegment := strings.ToLower(segment)
		if strings.HasSuffix(lowerSegment, ".photoslibrary") ||
			strings.HasSuffix(lowerSegment, ".lrlibrary") ||
			strings.HasSuffix(lowerSegment, ".lrdata") {
			// macOS package directories can also surface child paths directly through
			// FSEvents. Segment-level matching prevents package internals from
			// re-queueing scans after the top-level package directory was skipped.
			return true
		}
	}

	return false
}

func splitFileSearchPathSegments(fullPath string) []string {
	normalized := filepath.ToSlash(filepath.Clean(strings.TrimSpace(fullPath)))
	if normalized == "." || normalized == "" {
		return nil
	}

	return strings.Split(normalized, "/")
}

func (p *fileSearchIndexPolicy) shouldIgnoreByGitIgnore(rootPath string, fullPath string, isDir bool) bool {
	rootPath = filepath.Clean(strings.TrimSpace(rootPath))
	fullPath = filepath.Clean(strings.TrimSpace(fullPath))
	if rootPath == "" || fullPath == "" || !pathWithinRoot(rootPath, fullPath) || fullPath == rootPath {
		return false
	}

	ignored := false
	for _, directory := range patternDirectoriesForPath(rootPath, fullPath) {
		patterns := p.patternsForDirectory(directory)
		if len(patterns) == 0 {
			continue
		}
		// CPU profiles showed incremental pre-scan spending nearly all time
		// recalculating the same relative path once per .gitignore pattern. Compute
		// it once for the directory's pattern set, then reuse it so ignore matching
		// stays semantically identical while avoiding repeated filepath.Rel work.
		relPath, ok := relativePathForGitIgnoreMatch(directory, fullPath)
		if !ok {
			continue
		}
		for _, pattern := range patterns {
			if pattern.matchesRelPath(relPath, isDir) {
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
	patternSlash string
	negate       bool
	dirOnly      bool
	rooted       bool
	hasSlash     bool
	hasMeta      bool
	simpleGlob   bool
	simpleParts  []string
	leadingStar  bool
	trailingStar bool
	hasQuestion  bool
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

		pattern := gitIgnorePattern{}
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
		pattern.patternSlash = filepath.ToSlash(line)
		pattern.hasSlash = strings.Contains(pattern.patternSlash, "/")
		pattern.hasMeta = hasGitIgnoreGlobMeta(pattern.patternSlash)
		pattern.simpleGlob = isSimpleGitIgnoreGlob(pattern.patternSlash)
		if pattern.simpleGlob {
			pattern.simpleParts = strings.Split(pattern.patternSlash, "*")
			pattern.leadingStar = strings.HasPrefix(pattern.patternSlash, "*")
			pattern.trailingStar = strings.HasSuffix(pattern.patternSlash, "*")
			pattern.hasQuestion = strings.Contains(pattern.patternSlash, "?")
		}
		if pattern.patternSlash != "" {
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}

func relativePathForGitIgnoreMatch(baseDir string, fullPath string) (string, bool) {
	relPath, err := filepath.Rel(baseDir, fullPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", false
	}
	return filepath.ToSlash(relPath), true
}

func (p gitIgnorePattern) matchesRelPath(relPath string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}

	pattern := p.patternSlash

	if p.rooted || p.hasSlash {
		// Most ignore patterns are literals such as "target" or "coverage/". The
		// old matcher sent those through filepath.Match for every visited file,
		// which dominated filesearch CPU during startup restore. Literal patterns
		// can be compared directly; glob patterns keep the previous matcher path.
		if !p.hasMeta && relPath == pattern {
			return true
		}
		if p.hasMeta {
			if p.matchesCandidate(pattern, relPath) {
				return true
			}
		}
		return strings.HasPrefix(relPath, pattern+"/")
	}

	if !p.hasMeta {
		// For unrooted literal patterns, scan segments without strings.Split so
		// large trees do not allocate a segment slice for every path/pattern pair.
		return containsGitIgnorePathSegment(relPath, pattern)
	}

	return containsGitIgnoreMatchingSegment(relPath, p)
}

func (p gitIgnorePattern) matchesCandidate(pattern string, candidate string) bool {
	if p.simpleGlob {
		// CPU profiles showed simple patterns such as "*.ext" still dominating
		// startup restore because filepath.Match uses a general parser for every
		// path segment. Patterns without '?' are pre-split once at .gitignore load
		// time so common suffix/prefix globs use string searches instead of
		// per-candidate backtracking; anything more complex keeps the safe matcher.
		if !p.hasQuestion {
			return matchSimpleGitIgnoreLiteralGlob(p.simpleParts, p.leadingStar, p.trailingStar, candidate)
		}
		return matchSimpleGitIgnoreGlob(pattern, candidate)
	}

	ok, _ := filepath.Match(pattern, candidate)
	return ok
}

func containsGitIgnoreMatchingSegment(relPath string, pattern gitIgnorePattern) bool {
	for start := 0; start <= len(relPath); {
		end := strings.IndexByte(relPath[start:], '/')
		if end < 0 {
			return pattern.matchesCandidate(pattern.patternSlash, relPath[start:])
		}
		if pattern.matchesCandidate(pattern.patternSlash, relPath[start:start+end]) {
			return true
		}
		start += end + 1
	}

	return false
}

func hasGitIgnoreGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func isSimpleGitIgnoreGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?") && !strings.ContainsAny(pattern, "[\\")
}

func matchSimpleGitIgnoreLiteralGlob(parts []string, leadingStar bool, trailingStar bool, candidate string) bool {
	if len(parts) == 0 {
		return candidate == ""
	}

	position := 0
	firstPart := 0
	if !leadingStar {
		prefix := parts[0]
		if !strings.HasPrefix(candidate, prefix) {
			return false
		}
		position = len(prefix)
		firstPart = 1
	}

	lastPart := len(parts) - 1
	searchLimit := len(candidate)
	if !trailingStar {
		suffix := parts[lastPart]
		if !strings.HasSuffix(candidate, suffix) {
			return false
		}
		searchLimit = len(candidate) - len(suffix)
		lastPart--
	}

	for index := firstPart; index <= lastPart; index++ {
		part := parts[index]
		if part == "" {
			continue
		}
		if position > searchLimit {
			return false
		}
		offset := strings.Index(candidate[position:searchLimit], part)
		if offset < 0 {
			return false
		}
		position += offset + len(part)
	}

	return position <= searchLimit
}

func matchSimpleGitIgnoreGlob(pattern string, candidate string) bool {
	patternIndex := 0
	candidateIndex := 0
	starIndex := -1
	starCandidateIndex := 0

	for candidateIndex < len(candidate) {
		if patternIndex < len(pattern) && (pattern[patternIndex] == '?' || pattern[patternIndex] == candidate[candidateIndex]) {
			patternIndex++
			candidateIndex++
			continue
		}
		if patternIndex < len(pattern) && pattern[patternIndex] == '*' {
			starIndex = patternIndex
			starCandidateIndex = candidateIndex
			patternIndex++
			continue
		}
		if starIndex >= 0 {
			patternIndex = starIndex + 1
			starCandidateIndex++
			candidateIndex = starCandidateIndex
			continue
		}
		return false
	}

	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}

	return patternIndex == len(pattern)
}

func containsGitIgnorePathSegment(relPath string, pattern string) bool {
	for start := 0; start <= len(relPath); {
		end := strings.IndexByte(relPath[start:], '/')
		if end < 0 {
			return relPath[start:] == pattern
		}
		if relPath[start:start+end] == pattern {
			return true
		}
		start += end + 1
	}

	return false
}
