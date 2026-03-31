package filesearch

/*
#cgo LDFLAGS: -framework CoreServices -framework CoreFoundation
#include <stdbool.h>
#include <stdlib.h>

bool wox_mdquery_search_paths(const char *query, int maxResults, char **outPaths, char **outError);
void wox_mdquery_free(char *ptr);
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

type SpotlightProvider struct{}

func NewSystemProviders() []SearchProvider {
	return []SearchProvider{&SpotlightProvider{}}
}

func (p *SpotlightProvider) Name() string {
	return "spotlight"
}

func (p *SpotlightProvider) Search(ctx context.Context, query SearchQuery, limit int) ([]ProviderCandidate, error) {
	if len(query.Raw) == 0 {
		return nil, nil
	}

	paths, err := searchByMDQueryPaths(query.Raw, limit*2)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(paths))
	for _, path := range paths {
		select {
		case <-ctx.Done():
			return convertResultsToCandidates(sortAndLimitResults(results, limit)), ctx.Err()
		default:
		}

		name := filepath.Base(path)
		pinyinFull, pinyinInitials := buildPinyinFields(name)
		matched, score := scoreSearchTerms(query.Raw, buildSearchTerms(name, path, pinyinFull, pinyinInitials))
		if !matched {
			continue
		}

		isDir := false
		if info, statErr := os.Stat(path); statErr == nil {
			isDir = info.IsDir()
		}

		results = append(results, SearchResult{
			Path:       path,
			Name:       name,
			ParentPath: filepath.Dir(path),
			IsDir:      isDir,
			Score:      score,
		})
	}

	return convertResultsToCandidates(sortAndLimitResults(results, limit)), nil
}

func searchByMDQueryPaths(name string, maxResults int) ([]string, error) {
	if maxResults <= 0 {
		maxResults = 20
	}

	query := buildSpotlightQuery(name)
	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))

	var cPaths *C.char
	var cErr *C.char
	ok := C.wox_mdquery_search_paths(cQuery, C.int(maxResults), &cPaths, &cErr)

	if cErr != nil {
		defer C.wox_mdquery_free(cErr)
	}
	if cPaths != nil {
		defer C.wox_mdquery_free(cPaths)
	}

	if !ok {
		errMsg := "mdquery search failed"
		if cErr != nil {
			errMsg = C.GoString(cErr)
		}
		return nil, errors.New(errMsg)
	}

	if cPaths == nil {
		return []string{}, nil
	}

	lines := strings.Split(C.GoString(cPaths), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, line)
	}

	return paths, nil
}

func buildSpotlightQuery(value string) string {
	escaped := escapeMDQueryLiteral(value)
	return fmt.Sprintf("(kMDItemDisplayName=='*%[1]s*'cd || kMDItemFSName=='*%[1]s*'cd)", escaped)
}

func escapeMDQueryLiteral(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"'", "\\'",
		"*", "\\*",
		"?", "\\?",
	)
	return replacer.Replace(value)
}
