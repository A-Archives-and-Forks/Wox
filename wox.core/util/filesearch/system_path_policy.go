package filesearch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func shouldSkipSystemPathForRoot(root RootRecord, fullPath string, isDir bool) bool {
	if shouldSkipSystemPath(fullPath, isDir) {
		return true
	}
	if !isDir {
		return false
	}
	return shouldSkipDarwinHomeNoisePath(root, fullPath)
}

func shouldSkipDarwinHomeNoisePath(root RootRecord, fullPath string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return false
	}

	cleanHome := filepath.Clean(homeDir)
	cleanRoot := filepath.Clean(strings.TrimSpace(root.Path))
	cleanPath := filepath.Clean(strings.TrimSpace(fullPath))
	if cleanRoot != cleanHome || cleanPath == cleanRoot {
		return false
	}

	relPath, err := filepath.Rel(cleanHome, cleanPath)
	if err != nil || relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
		return false
	}

	segments := strings.Split(relPath, string(filepath.Separator))
	if len(segments) == 0 {
		return false
	}

	// Optimization: a configured home root should behave like launcher file
	// search, not a full `find ~/` crawl. macOS keeps high-churn, protected app
	// state under ~/Library, and traversing it dominated real-index captures while
	// producing noisy launcher results. If the user explicitly adds ~/Library as
	// its own root, cleanRoot no longer equals the home directory and this pruning
	// does not apply.
	if segments[0] == "Library" {
		return true
	}

	if len(segments) == 2 && segments[0] == "Music" && segments[1] == "Music" {
		return true
	}

	if len(segments) >= 2 && segments[0] == "Pictures" && strings.HasSuffix(strings.ToLower(filepath.Base(cleanPath)), ".photoslibrary") {
		return true
	}

	return false
}
