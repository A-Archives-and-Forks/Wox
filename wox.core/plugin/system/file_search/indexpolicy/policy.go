package indexpolicy

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Policy owns the plugin-level filesystem ignore rules that sit above the
// policy-neutral filesearch engine.
type Policy struct {
	mu            sync.RWMutex
	patternsByDir map[string][]gitIgnorePattern
	ignoreRules   fileSearchIgnoreRules
	diagnostics   *Diagnostics
}

// Diagnostics accumulates policy costs for the opt-in real-index benchmark.
// The previous benchmark could report "scan is slow" without showing whether
// the cost came from configured globs, ancestor .gitignore checks, or uncached
// .gitignore reads, so these counters stay attached to the real plugin policy.
type Diagnostics struct {
	policyChecks                  atomic.Int64
	policyNanos                   atomic.Int64
	policyIgnored                 atomic.Int64
	configuredPatternChecks       atomic.Int64
	configuredPatternNanos        atomic.Int64
	configuredPatternIgnored      atomic.Int64
	gitIgnoreChecks               atomic.Int64
	gitIgnoreNanos                atomic.Int64
	gitIgnoreIgnored              atomic.Int64
	gitIgnoreAncestorDirectories  atomic.Int64
	gitIgnoreDirectoriesWithRules atomic.Int64
	gitIgnorePatternComparisons   atomic.Int64
	gitIgnorePatternLoads         atomic.Int64
	gitIgnorePatternsLoaded       atomic.Int64
	gitIgnorePatternLoadNanos     atomic.Int64
}

type DiagnosticsSnapshot struct {
	PolicyChecks                  int64 `json:"policy_checks"`
	PolicyMillis                  int64 `json:"policy_millis"`
	PolicyIgnored                 int64 `json:"policy_ignored"`
	ConfiguredPatternChecks       int64 `json:"configured_pattern_checks"`
	ConfiguredPatternMillis       int64 `json:"configured_pattern_millis"`
	ConfiguredPatternIgnored      int64 `json:"configured_pattern_ignored"`
	GitIgnoreChecks               int64 `json:"gitignore_checks"`
	GitIgnoreMillis               int64 `json:"gitignore_millis"`
	GitIgnoreIgnored              int64 `json:"gitignore_ignored"`
	GitIgnoreAncestorDirectories  int64 `json:"gitignore_ancestor_directories"`
	GitIgnoreDirectoriesWithRules int64 `json:"gitignore_directories_with_rules"`
	GitIgnorePatternComparisons   int64 `json:"gitignore_pattern_comparisons"`
	GitIgnorePatternLoads         int64 `json:"gitignore_pattern_loads"`
	GitIgnorePatternsLoaded       int64 `json:"gitignore_patterns_loaded"`
	GitIgnorePatternLoadMillis    int64 `json:"gitignore_pattern_load_millis"`
}

func NewDiagnostics() *Diagnostics {
	return &Diagnostics{}
}

func (p *Policy) SetDiagnostics(diagnostics *Diagnostics) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.diagnostics = diagnostics
	p.mu.Unlock()
}

func (p *Policy) DiagnosticsSnapshot() DiagnosticsSnapshot {
	diagnostics := p.diagnosticsRef()
	if diagnostics == nil {
		return DiagnosticsSnapshot{}
	}
	return diagnostics.Snapshot()
}

func (p *Policy) diagnosticsRef() *Diagnostics {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	diagnostics := p.diagnostics
	p.mu.RUnlock()
	return diagnostics
}

func (d *Diagnostics) Snapshot() DiagnosticsSnapshot {
	if d == nil {
		return DiagnosticsSnapshot{}
	}
	return DiagnosticsSnapshot{
		PolicyChecks:                  d.policyChecks.Load(),
		PolicyMillis:                  diagnosticMillis(d.policyNanos.Load()),
		PolicyIgnored:                 d.policyIgnored.Load(),
		ConfiguredPatternChecks:       d.configuredPatternChecks.Load(),
		ConfiguredPatternMillis:       diagnosticMillis(d.configuredPatternNanos.Load()),
		ConfiguredPatternIgnored:      d.configuredPatternIgnored.Load(),
		GitIgnoreChecks:               d.gitIgnoreChecks.Load(),
		GitIgnoreMillis:               diagnosticMillis(d.gitIgnoreNanos.Load()),
		GitIgnoreIgnored:              d.gitIgnoreIgnored.Load(),
		GitIgnoreAncestorDirectories:  d.gitIgnoreAncestorDirectories.Load(),
		GitIgnoreDirectoriesWithRules: d.gitIgnoreDirectoriesWithRules.Load(),
		GitIgnorePatternComparisons:   d.gitIgnorePatternComparisons.Load(),
		GitIgnorePatternLoads:         d.gitIgnorePatternLoads.Load(),
		GitIgnorePatternsLoaded:       d.gitIgnorePatternsLoaded.Load(),
		GitIgnorePatternLoadMillis:    diagnosticMillis(d.gitIgnorePatternLoadNanos.Load()),
	}
}

func (d *Diagnostics) recordPolicyCheck(elapsed time.Duration, ignored bool) {
	d.policyChecks.Add(1)
	d.policyNanos.Add(elapsed.Nanoseconds())
	if ignored {
		d.policyIgnored.Add(1)
	}
}

func (d *Diagnostics) recordConfiguredPatternCheck(elapsed time.Duration, ignored bool) {
	d.configuredPatternChecks.Add(1)
	d.configuredPatternNanos.Add(elapsed.Nanoseconds())
	if ignored {
		d.configuredPatternIgnored.Add(1)
	}
}

func (d *Diagnostics) recordGitIgnoreCheck(elapsed time.Duration, ignored bool, ancestorDirectories int64, directoriesWithRules int64, patternComparisons int64) {
	d.gitIgnoreChecks.Add(1)
	d.gitIgnoreNanos.Add(elapsed.Nanoseconds())
	d.gitIgnoreAncestorDirectories.Add(ancestorDirectories)
	d.gitIgnoreDirectoriesWithRules.Add(directoriesWithRules)
	d.gitIgnorePatternComparisons.Add(patternComparisons)
	if ignored {
		d.gitIgnoreIgnored.Add(1)
	}
}

func (d *Diagnostics) recordGitIgnorePatternLoad(elapsed time.Duration, patternCount int) {
	d.gitIgnorePatternLoads.Add(1)
	d.gitIgnorePatternsLoaded.Add(int64(patternCount))
	d.gitIgnorePatternLoadNanos.Add(elapsed.Nanoseconds())
}

func diagnosticMillis(nanos int64) int64 {
	if nanos <= 0 {
		return 0
	}
	return (nanos + int64(time.Millisecond) - 1) / int64(time.Millisecond)
}

// Feature addition: seed the user-editable ignore table with the generated and
// hidden folders that are expensive to traverse and noisy as launcher results.
// The list remains plain glob text so settings can expose the same values that
// the scanner uses.
var defaultIgnorePatterns = []string{
	".*",
	"*.tmp",
	"*.temp",
	".DS_Store",
	".git",
	".hg",
	".svn",
	"node_modules",
	"build",
	"dist",
	".dart_tool",
	".gradle",
	".swiftpm",
	".build",
	"DerivedData",
	"__pycache__",
	".pytest_cache",
	".mypy_cache",
	".ruff_cache",
	".venv",
	"venv",
	".cache",
	".umi",
	".umi-production",
	".next",
	".nuxt",
	".vite",
	".turbo",
	".parcel-cache",
	".output",
	"out",
	"output",
	"outputs",
	"coverage",
	"target",
	".idea",
	".vscode",
	".cursor",
	"**/tmp/**",
	"**/temp/**",
	"**/Cache/**",
	"**/Caches/**",
	"**/cache/**",
	"**/caches/**",
	"**/Library/Application Support/**",
	"**/Mobile Documents/**/PreferenceSync/**",
	"**/Mobile Documents/**/Application Support/**",
	"*.photoslibrary",
	"*.lrlibrary",
	"*.lrdata",
	"**/_work/**",
	"**/externals.*/**",
}

// DefaultIgnorePatterns returns a copy so callers can expose or sort the
// defaults without mutating the shared plugin policy baseline.
func DefaultIgnorePatterns() []string {
	return append([]string(nil), defaultIgnorePatterns...)
}

func New() *Policy {
	return &Policy{
		patternsByDir: map[string][]gitIgnorePattern{},
		ignoreRules:   compileFileSearchIgnoreRules(defaultIgnorePatterns),
	}
}

func (p *Policy) ShouldIndexPath(rootPath string, policyRootPath string, path string, isDir bool) bool {
	if p == nil {
		return true
	}
	diagnostics := p.diagnosticsRef()
	startedAt := time.Now()
	ignored := false
	defer func() {
		if diagnostics != nil {
			diagnostics.recordPolicyCheck(time.Since(startedAt), ignored)
		}
	}()

	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" {
		return true
	}

	if p.shouldIgnoreByConfiguredPattern(rootPath, policyRootPath, cleanPath, diagnostics) {
		ignored = true
		return false
	}

	policyRootPath = strings.TrimSpace(policyRootPath)
	if policyRootPath == "" {
		policyRootPath = rootPath
	}
	// Dynamic roots keep their own scan scope but must inherit the user's
	// parent .gitignore chain. Using PolicyRootPath for ignore lookup preserves
	// that policy without widening the scanner's ownership boundary.
	ignored = p.shouldIgnoreByGitIgnore(filepath.Clean(policyRootPath), cleanPath, isDir, diagnostics)
	return !ignored
}

func (p *Policy) SetIgnorePatterns(patterns []string) {
	// Feature addition: ignore rules moved from a fixed code list into user
	// settings. Compile them once on setting changes so every visited path pays
	// only cheap matcher checks during large file-index runs.
	compiled := compileFileSearchIgnoreRules(patterns)

	p.mu.Lock()
	p.ignoreRules = compiled
	p.mu.Unlock()
}

func splitFileSearchPathSegments(fullPath string) []string {
	normalized := filepath.ToSlash(filepath.Clean(strings.TrimSpace(fullPath)))
	if normalized == "." || normalized == "" {
		return nil
	}

	// Bug fix: absolute paths start with "/", so strings.Split would emit an
	// empty first segment. Segment ignore rules should only see real path
	// components, otherwise generated empty segments add work to the hot path and
	// make user-visible glob behavior harder to reason about.
	rawSegments := strings.Split(normalized, "/")
	segments := make([]string, 0, len(rawSegments))
	for _, segment := range rawSegments {
		if segment == "" {
			continue
		}
		segments = append(segments, segment)
	}
	return segments
}

type fileSearchIgnoreRule struct {
	hasSlash       bool
	segmentLiteral string
	pathRegex      *regexp.Regexp
	segmentRe      *regexp.Regexp
}

type fileSearchIgnoreRules struct {
	pathRules       []fileSearchIgnoreRule
	segmentLiterals map[string]struct{}
	segmentRules    []fileSearchIgnoreRule
}

func compileFileSearchIgnoreRules(patterns []string) fileSearchIgnoreRules {
	compiled := fileSearchIgnoreRules{segmentLiterals: map[string]struct{}{}}
	seen := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		raw := strings.TrimSpace(pattern)
		if raw == "" {
			continue
		}

		normalized := filepath.ToSlash(raw)
		key := strings.ToLower(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		rule, ok := compileFileSearchIgnoreRule(normalized)
		if ok {
			if rule.hasSlash {
				compiled.pathRules = append(compiled.pathRules, rule)
				continue
			}
			if rule.segmentLiteral != "" {
				compiled.segmentLiterals[rule.segmentLiteral] = struct{}{}
				continue
			}
			compiled.segmentRules = append(compiled.segmentRules, rule)
		}
	}

	return compiled
}

func compileFileSearchIgnoreRule(pattern string) (fileSearchIgnoreRule, bool) {
	pattern = strings.TrimSpace(filepath.ToSlash(pattern))
	if pattern == "" {
		return fileSearchIgnoreRule{}, false
	}

	// Ignore patterns are user-facing, so they intentionally use Raycast-style
	// path globs instead of Go regexes. Segment-only patterns such as
	// "node_modules" match any path segment, while path patterns such as
	// "**/cache/**" can prune a whole subtree before the scanner descends into it.
	hasSlash := strings.Contains(pattern, "/")
	if !hasSlash && !strings.ContainsAny(pattern, "*?[") {
		return fileSearchIgnoreRule{
			segmentLiteral: strings.ToLower(pattern),
		}, true
	}
	expr := globPatternToRegex(pattern, !hasSlash)
	if expr == "" {
		return fileSearchIgnoreRule{}, false
	}

	compiled, err := regexp.Compile("(?i)^" + expr + "$")
	if err != nil {
		return fileSearchIgnoreRule{}, false
	}

	rule := fileSearchIgnoreRule{
		hasSlash: hasSlash,
	}
	if hasSlash {
		rule.pathRegex = compiled
	} else {
		rule.segmentRe = compiled
	}
	return rule, true
}

func globPatternToRegex(pattern string, segmentOnly bool) string {
	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		if base == "" {
			return ".*"
		}
		return globPatternToRegex(base, segmentOnly) + "(?:/.*)?"
	}

	var builder strings.Builder
	for index := 0; index < len(pattern); {
		if strings.HasPrefix(pattern[index:], "**/") {
			builder.WriteString("(?:.*/)?")
			index += 3
			continue
		}
		if strings.HasPrefix(pattern[index:], "**") {
			builder.WriteString(".*")
			index += 2
			continue
		}

		character := pattern[index]
		switch character {
		case '*':
			if segmentOnly {
				builder.WriteString(".*")
			} else {
				builder.WriteString("[^/]*")
			}
		case '?':
			if segmentOnly {
				builder.WriteByte('.')
			} else {
				builder.WriteString("[^/]")
			}
		case '[':
			end := strings.IndexByte(pattern[index+1:], ']')
			if end < 0 {
				builder.WriteString(regexp.QuoteMeta(string(character)))
			} else {
				class := pattern[index : index+end+2]
				builder.WriteString(class)
				index += end + 2
				continue
			}
		default:
			builder.WriteString(regexp.QuoteMeta(string(character)))
		}
		index++
	}

	return builder.String()
}

func (p *Policy) shouldIgnoreByConfiguredPattern(rootPath string, policyRootPath string, fullPath string, diagnostics *Diagnostics) bool {
	startedAt := time.Now()
	ignored := false
	defer func() {
		if diagnostics != nil {
			diagnostics.recordConfiguredPatternCheck(time.Since(startedAt), ignored)
		}
	}()

	matchRootPath := strings.TrimSpace(policyRootPath)
	if matchRootPath == "" {
		matchRootPath = strings.TrimSpace(rootPath)
	}

	relPath, hasRelPath := relativePathForGitIgnoreMatch(filepath.Clean(matchRootPath), fullPath)
	fullSlash := filepath.ToSlash(filepath.Clean(fullPath))
	candidates := []string{fullSlash}
	if hasRelPath {
		candidates = append(candidates, relPath)
	}
	segments := splitFileSearchPathSegments(fullSlash)

	p.mu.RLock()
	defer p.mu.RUnlock()
	// Optimization: ignore rules are evaluated for every filesystem entry during
	// full indexing. The first configurable implementation split the same path
	// once per segment rule and ran regexes for literal names like "node_modules",
	// which made the new ignore feature itself part of the slow index path.
	if p.ignoreRules.matches(candidates, segments) {
		ignored = true
		return true
	}
	return false
}

func (rules fileSearchIgnoreRules) matches(pathCandidates []string, segments []string) bool {
	for _, rule := range rules.pathRules {
		if rule.matchesPath(pathCandidates) {
			return true
		}
	}

	for _, segment := range segments {
		normalizedSegment := strings.ToLower(segment)
		if _, ok := rules.segmentLiterals[normalizedSegment]; ok {
			return true
		}
		for _, rule := range rules.segmentRules {
			if rule.matchesSegment(segment) {
				return true
			}
		}
	}

	return false
}

func (r fileSearchIgnoreRule) matchesPath(pathCandidates []string) bool {
	if r.pathRegex == nil {
		return false
	}
	for _, candidate := range pathCandidates {
		if candidate == "." || candidate == "" {
			continue
		}
		if r.pathRegex.MatchString(strings.TrimPrefix(candidate, "/")) || r.pathRegex.MatchString(candidate) {
			return true
		}
	}
	return false
}

func (r fileSearchIgnoreRule) matchesSegment(segment string) bool {
	return r.segmentRe != nil && r.segmentRe.MatchString(segment)
}

func (p *Policy) shouldIgnoreByGitIgnore(rootPath string, fullPath string, isDir bool, diagnostics *Diagnostics) bool {
	startedAt := time.Now()
	directoriesVisited := int64(0)
	directoriesWithPatterns := int64(0)
	patternComparisons := int64(0)
	ignored := false
	defer func() {
		if diagnostics != nil {
			diagnostics.recordGitIgnoreCheck(time.Since(startedAt), ignored, directoriesVisited, directoriesWithPatterns, patternComparisons)
		}
	}()

	rootPath = filepath.Clean(strings.TrimSpace(rootPath))
	fullPath = filepath.Clean(strings.TrimSpace(fullPath))
	if rootPath == "" || fullPath == "" || !pathWithinRoot(rootPath, fullPath) || fullPath == rootPath {
		return false
	}

	for _, directory := range patternDirectoriesForPath(rootPath, fullPath) {
		directoriesVisited++
		patterns := p.patternsForDirectory(directory, diagnostics)
		if len(patterns) == 0 {
			continue
		}
		directoriesWithPatterns++
		patternComparisons += int64(len(patterns))
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

func (p *Policy) patternsForDirectory(directory string, diagnostics *Diagnostics) []gitIgnorePattern {
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

	startedAt := time.Now()
	loaded := loadGitIgnorePatterns(directory)
	if diagnostics != nil {
		diagnostics.recordGitIgnorePatternLoad(time.Since(startedAt), len(loaded))
	}

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
