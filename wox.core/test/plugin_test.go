package test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wox/common"
	"wox/plugin"
)

func TestUrlPlugin(t *testing.T) {
	suite := NewTestSuite(t)

	tests := []QueryTest{
		{
			Name:           "Domain only",
			Query:          "google.com",
			ExpectedTitle:  "google.com",
			ExpectedAction: "Open",
		},
		{
			Name:           "With https",
			Query:          "https://www.google.com",
			ExpectedTitle:  "https://www.google.com",
			ExpectedAction: "Open",
		},
		{
			Name:           "With path",
			Query:          "github.com/Wox-launcher/Wox",
			ExpectedTitle:  "github.com/Wox-launcher/Wox",
			ExpectedAction: "Open",
		},
		{
			Name:           "With query parameters",
			Query:          "google.com/search?q=wox",
			ExpectedTitle:  "google.com/search?q=wox",
			ExpectedAction: "Open",
		},
	}

	suite.RunQueryTests(tests)
}

func TestSystemPlugin(t *testing.T) {
	suite := NewTestSuite(t)

	tests := []QueryTest{
		{
			Name:           "Lock command",
			Query:          "lock",
			ExpectedTitle:  "Lock PC",
			ExpectedAction: "Execute",
		},
		{
			Name:           "Settings command",
			Query:          "settings",
			ExpectedTitle:  "Open Wox Settings",
			ExpectedAction: "Execute",
		},
		{
			Name:           "Empty trash command",
			Query:          "trash",
			ExpectedTitle:  "Empty Trash",
			ExpectedAction: "Execute",
		},
		{
			Name:           "Exit command",
			Query:          "exit",
			ExpectedTitle:  "Exit",
			ExpectedAction: "Execute",
		},
	}

	suite.RunQueryTests(tests)
}

func TestWebSearchPlugin(t *testing.T) {
	suite := NewTestSuite(t)

	tests := []QueryTest{
		{
			Name:           "Google search",
			Query:          "g wox launcher",
			ExpectedTitle:  "Search Google for wox launcher",
			ExpectedAction: "Search",
		},
	}

	suite.RunQueryTests(tests)
}

func TestFilePlugin_CustomRoots(t *testing.T) {
	suite := NewTestSuite(t)
	ctx := suite.ctx

	rootPath := filepath.Join(t.TempDir(), "filesearch-smoke-root")
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		t.Fatalf("failed to create file search root: %v", err)
	}

	rootSetting, err := json.Marshal([]map[string]string{
		{"Path": rootPath},
	})
	if err != nil {
		t.Fatalf("failed to marshal file search roots setting: %v", err)
	}

	filePlugin := findPluginInstance("979d6363-025a-4f51-88d3-0b04e9dc56bf")
	if filePlugin == nil {
		t.Fatal("file plugin instance not found")
	}

	filePlugin.API.SaveSetting(ctx, "roots", string(rootSetting), false)

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		results, err := runQuery(ctx, "f filesearch-smoke-root")
		if err != nil {
			t.Fatalf("failed to query file plugin: %v", err)
		}

		for _, result := range results {
			if result.Title == "filesearch-smoke-root" && hasAction(result, "Open") {
				return
			}
		}

		time.Sleep(200 * time.Millisecond)
	}

	results, err := runQuery(ctx, "f filesearch-smoke-root")
	if err != nil {
		t.Fatalf("failed to query file plugin: %v", err)
	}

	t.Fatalf("expected custom root to be searchable, got %d result(s)", len(results))
}

func findPluginInstance(pluginID string) *plugin.Instance {
	for _, instance := range plugin.GetPluginManager().GetPluginInstances() {
		if instance.Metadata.Id == pluginID {
			return instance
		}
	}
	return nil
}

func runQuery(ctx context.Context, rawQuery string) ([]plugin.QueryResultUI, error) {
	query, queryPlugin, err := plugin.GetPluginManager().NewQuery(ctx, common.PlainQuery{
		QueryType: plugin.QueryTypeInput,
		QueryText: rawQuery,
	})
	if err != nil {
		return nil, err
	}

	resultChan, doneChan := plugin.GetPluginManager().Query(ctx, query)
	var allResults []plugin.QueryResultUI

collect:
	for {
		select {
		case results := <-resultChan:
			allResults = append(allResults, results...)
		case <-doneChan:
			for {
				select {
				case results := <-resultChan:
					allResults = append(allResults, results...)
				default:
					break collect
				}
			}
		case <-time.After(5 * time.Second):
			return nil, context.DeadlineExceeded
		}
	}

	if len(allResults) == 0 {
		allResults = plugin.GetPluginManager().QueryFallback(ctx, query, queryPlugin)
	}

	return allResults, nil
}

func hasAction(result plugin.QueryResultUI, expectedAction string) bool {
	for _, action := range result.Actions {
		if action.Name == expectedAction {
			return true
		}
	}
	return false
}
