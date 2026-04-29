package fileicon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"wox/util"
)

const fileIconPathCachePrefix = "fileicon_v4_"

// GetFileIconByPath returns the default list-size cached OS icon path for the given path.
// It first tries to resolve the application/file icon, then falls back to the file-type icon.
func GetFileIconByPath(ctx context.Context, filePath string) (string, error) {
	return GetFileIconByPathWithSize(ctx, filePath, util.ResultListIconSize)
}

func GetFileIconByPathWithSize(ctx context.Context, filePath string, size int) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Fileicon caches are shared by list and grid surfaces. Keep the requested
	// size in the cache key so a grid result never reuses the compact list icon.
	if size <= 0 {
		size = util.ResultListIconSize
	}

	iconPath, err := getFileIconImpl(ctx, filePath, size)
	if err == nil && strings.TrimSpace(iconPath) != "" {
		return iconPath, nil
	}

	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(filePath)))
	if ext == "" {
		// Unknown extension – treat as generic
		ext = ".__unknown"
	}
	return GetFileTypeIconWithSize(ctx, ext, size)
}

func CleanFileIconCache(ctx context.Context, filePath string) error {
	// Only remove cache entries produced by the current rendering strategy.
	// When the icon pipeline changes, bump fileIconPathCachePrefix so old
	// cache files naturally stop being referenced instead of carrying legacy
	// cleanup rules for every retired size.
	cacheSizes := []int{util.ResultListIconSize, util.ResultGridIconSize}
	seenSizes := map[int]struct{}{}
	for _, size := range cacheSizes {
		if _, ok := seenSizes[size]; ok {
			continue
		}
		seenSizes[size] = struct{}{}

		cachePath := buildPathCachePath(filePath, size)
		if _, err := os.Stat(cachePath); err == nil {
			if removeErr := os.Remove(cachePath); removeErr != nil {
				return removeErr
			}
		}
	}

	return nil
}

// GetFileTypeIcon returns the default list-size cached OS file-type icon path for the given extension.
// The ext can be with or without leading dot.
func GetFileTypeIcon(ctx context.Context, ext string) (string, error) {
	return GetFileTypeIconWithSize(ctx, ext, util.ResultListIconSize)
}

func GetFileTypeIconWithSize(ctx context.Context, ext string, size int) (string, error) {
	if size <= 0 {
		size = util.ResultListIconSize
	}
	if ext == "" {
		ext = ".__unknown"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return getFileTypeIconImpl(ctx, ext, size)
}

// buildCachePath returns the cache file path for a given extension and size (in px).
func buildCachePath(ext string, size int) string {
	// sanitize ext for filename (remove dot)
	safe := strings.TrimPrefix(ext, ".")
	if safe == "" {
		safe = "__unknown"
	}
	file := "filetype_" + safe + "_" + intToString(size) + ".png"
	return filepath.Join(util.GetLocation().GetImageCacheDirectory(), file)
}

// buildPathCachePath returns the cache file path for a given file path and size (in px).
func buildPathCachePath(filePath string, size int) string {
	hash := util.Md5([]byte(filePath))
	// The prefix is the cache-version boundary for file-icon rendering behavior.
	// Bump it when source extraction or resize semantics change so older blurred
	// caches are ignored without keeping legacy-size cleanup paths.
	file := fileIconPathCachePrefix + hash + "_" + intToString(size) + ".png"
	return filepath.Join(util.GetLocation().GetImageCacheDirectory(), file)
}

// intToString avoids fmt for tiny helper to keep deps minimal here
func intToString(v int) string {
	// very small helper
	if v == 0 {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
