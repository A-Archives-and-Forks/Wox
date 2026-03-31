package filesearch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type LocateProvider struct{}

func NewSystemProviders() []SearchProvider {
	return []SearchProvider{&LocateProvider{}}
}

func (p *LocateProvider) Name() string {
	return "locate"
}

func (p *LocateProvider) Search(ctx context.Context, query SearchQuery, limit int) ([]ProviderCandidate, error) {
	paths, err := locateWithOptions(ctx, query.Raw, limit*2)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(paths))
	for _, item := range paths {
		name := filepath.Base(item)
		pinyinFull, pinyinInitials := buildPinyinFields(name)
		matched, score := scoreSearchTerms(query.Raw, buildSearchTerms(name, item, pinyinFull, pinyinInitials))
		if !matched {
			continue
		}

		isDir := false
		if info, statErr := os.Stat(item); statErr == nil {
			isDir = info.IsDir()
		}

		results = append(results, SearchResult{
			Path:       item,
			Name:       name,
			ParentPath: filepath.Dir(item),
			IsDir:      isDir,
			Score:      score,
		})
	}

	return convertResultsToCandidates(sortAndLimitResults(results, limit)), nil
}

func locateWithOptions(parentCtx context.Context, query string, maxResults int) ([]string, error) {
	ctx, cancel := context.WithTimeout(parentCtx, 200*time.Millisecond)
	defer cancel()

	args := []string{"-0", "-b", "-l", fmt.Sprintf("%d", maxResults), query}
	cmd := exec.CommandContext(ctx, "locate", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to run locate (%v): %s", err, out.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\x00")
	var results []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		results = append(results, line)
	}

	return results, nil
}
