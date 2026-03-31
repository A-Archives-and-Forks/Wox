package filesearch

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"wox/util"
)

var ErrEverythingNotRunning = errors.New("everything is not running")

type EverythingProvider struct{}

func NewSystemProviders() []SearchProvider {
	dllPath := path.Join(util.GetLocation().GetOthersDirectory(), "Everything3_x64.dll")
	initEverythingDLL(dllPath)

	legacyDLLPath := path.Join(util.GetLocation().GetOthersDirectory(), "Everything64.dll")
	initEverything2DLL(legacyDLLPath)

	return []SearchProvider{&EverythingProvider{}}
}

func (p *EverythingProvider) Name() string {
	return "everything"
}

func (p *EverythingProvider) Search(ctx context.Context, query SearchQuery, limit int) ([]ProviderCandidate, error) {
	var results []SearchResult

	err := WalkEverything(query.Raw, limit*2, func(path string, info FileInfo, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		name := filepath.Base(path)
		pinyinFull, pinyinInitials := buildPinyinFields(name)
		matched, score := scoreSearchTerms(query.Raw, buildSearchTerms(name, path, pinyinFull, pinyinInitials))
		if !matched {
			return nil
		}

		isDir := info.IsDir()
		if statInfo, statErr := os.Stat(path); statErr == nil {
			isDir = statInfo.IsDir()
		}

		results = append(results, SearchResult{
			Path:       path,
			Name:       name,
			ParentPath: filepath.Dir(path),
			IsDir:      isDir,
			Score:      score,
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrEverythingNotRunning) {
			return nil, err
		}
		if errorsIsCanceled(err) {
			return convertResultsToCandidates(sortAndLimitResults(results, limit)), nil
		}
		return nil, err
	}

	return convertResultsToCandidates(sortAndLimitResults(results, limit)), nil
}
