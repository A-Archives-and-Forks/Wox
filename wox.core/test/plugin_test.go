package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
	"wox/common"
	"wox/plugin"
	"wox/ui"
	"wox/util"
	"wox/util/filesearch"

	"github.com/gorilla/websocket"
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

	rootPath := newStableFileSearchRoot(t, "filesearch-smoke-root")

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

	rootName := filepath.Base(rootPath)
	if err := waitForFileSearchResult(ctx, "f "+rootName, rootName, rootPath, 8*time.Second); err == nil {
		return
	}

	results, err := runQuery(ctx, "f "+rootName)
	if err != nil {
		t.Fatalf("failed to query file plugin: %v", err)
	}

	t.Fatalf("expected custom root to be searchable, got %d result(s)", len(results))
}

func TestFilePlugin_CustomRootsIncrementalSync(t *testing.T) {
	suite := NewTestSuite(t)
	ctx := suite.ctx

	rootPath := newStableFileSearchRoot(t, "filesearch-watch-root")

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
	if err := waitForFileSearchRootReady(ctx, rootPath, 30*time.Second); err != nil {
		t.Fatalf("file search root did not become ready: %v", err)
	}

	initialFileName := fmt.Sprintf("initial-%d.txt", time.Now().UnixNano())
	initialFilePath := filepath.Join(rootPath, initialFileName)
	if err := os.WriteFile(initialFilePath, []byte("initial"), 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	if err := waitForFileSearchResult(ctx, "f "+initialFileName, initialFileName, initialFilePath, 8*time.Second); err != nil {
		t.Fatalf("initial file did not become searchable: %v", err)
	}

	observer := newToolbarObserver(t)
	defer observer.Close()

	incrementalFileName := fmt.Sprintf("sync-target-%d.txt", time.Now().UnixNano())
	incrementalFilePath := filepath.Join(rootPath, incrementalFileName)
	sessionID := "file-plugin-incremental-sync"
	results, err := runQueryWithSession(ctx, sessionID, "f "+incrementalFileName)
	if err != nil {
		t.Fatalf("failed to create active file plugin query: %v", err)
	}
	for _, result := range results {
		if result.Title == incrementalFileName && result.SubTitle == incrementalFilePath {
			t.Fatalf("expected incremental file path %q to be absent before creation, got %#v", incrementalFilePath, results)
		}
	}

	if err := os.WriteFile(incrementalFilePath, []byte("incremental"), 0644); err != nil {
		t.Fatalf("failed to create incremental file: %v", err)
	}

	if err := pollUntil(8*time.Second, 100*time.Millisecond, func() (bool, error) {
		return observer.HasToolbarTitlePrefix("Syncing file changes"), nil
	}); err != nil {
		t.Fatalf("expected incremental sync toolbar message, got titles: %v", observer.ToolbarTitles())
	}

	if err := waitForFileSearchResult(ctx, "f "+incrementalFileName, incrementalFileName, incrementalFilePath, 30*time.Second); err != nil {
		t.Fatalf("incremental file did not become searchable: %v", err)
	}
}

func TestFilePlugin_CustomRootsIgnoresDSStore(t *testing.T) {
	suite := NewTestSuite(t)
	ctx := suite.ctx

	rootPath := newStableFileSearchRoot(t, "filesearch-ignore-dsstore-root")

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

	ignoredFilePath := filepath.Join(rootPath, ".DS_Store")
	if err := os.WriteFile(ignoredFilePath, []byte("ignored"), 0644); err != nil {
		t.Fatalf("failed to create ignored file: %v", err)
	}

	visibleFileName := fmt.Sprintf("visible-%d.txt", time.Now().UnixNano())
	visibleFilePath := filepath.Join(rootPath, visibleFileName)
	if err := os.WriteFile(visibleFilePath, []byte("visible"), 0644); err != nil {
		t.Fatalf("failed to create visible file: %v", err)
	}

	filePlugin.API.SaveSetting(ctx, "roots", string(rootSetting), false)

	if err := ensureFileSearchResultAbsent(ctx, "f store", ".DS_Store", ignoredFilePath, 30*time.Second); err != nil {
		t.Fatalf(".DS_Store should remain hidden from file search: %v", err)
	}

	if err := waitForFileSearchResult(ctx, "f "+visibleFileName, visibleFileName, visibleFilePath, 30*time.Second); err != nil {
		t.Fatalf("visible file did not become searchable: %v", err)
	}
}

func TestFilePlugin_WildcardExtensionFilter(t *testing.T) {
	suite := NewTestSuite(t)
	ctx := suite.ctx

	rootPath := newStableFileSearchRoot(t, "filesearch-wildcard-root")

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

	pngFileName := fmt.Sprintf("wildcard-target-%d.png", time.Now().UnixNano())
	pngFilePath := filepath.Join(rootPath, pngFileName)
	if err := os.WriteFile(pngFilePath, []byte("png"), 0644); err != nil {
		t.Fatalf("failed to create png file: %v", err)
	}

	textFileName := fmt.Sprintf("wildcard-target-%d.txt", time.Now().UnixNano())
	textFilePath := filepath.Join(rootPath, textFileName)
	if err := os.WriteFile(textFilePath, []byte("txt"), 0644); err != nil {
		t.Fatalf("failed to create text file: %v", err)
	}

	filePlugin.API.SaveSetting(ctx, "roots", string(rootSetting), false)

	if err := waitForFileSearchResult(ctx, "f *.png", pngFileName, pngFilePath, 30*time.Second); err != nil {
		t.Fatalf("png file did not become searchable with wildcard filter: %v", err)
	}

	if err := ensureFileSearchResultAbsent(ctx, "f *.png", textFileName, textFilePath, 5*time.Second); err != nil {
		t.Fatalf("non-png file should be excluded by wildcard filter: %v", err)
	}
}

func TestFilePlugin_PathFragmentSearch(t *testing.T) {
	suite := NewTestSuite(t)
	ctx := suite.ctx

	rootPath := newStableFileSearchRoot(t, "filesearch-path-fragment-root")

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

	targetDirectory := filepath.Join(rootPath, "alpha", "beta")
	if err := os.MkdirAll(targetDirectory, 0755); err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}

	targetFileName := fmt.Sprintf("path-target-%d.txt", time.Now().UnixNano())
	targetFilePath := filepath.Join(targetDirectory, targetFileName)
	if err := os.WriteFile(targetFilePath, []byte("path"), 0644); err != nil {
		t.Fatalf("failed to create target file: %v", err)
	}

	filePlugin.API.SaveSetting(ctx, "roots", string(rootSetting), false)

	if err := waitForFileSearchResult(ctx, "f alpha/beta", targetFileName, targetFilePath, 30*time.Second); err != nil {
		t.Fatalf("path fragment query did not return target file: %v", err)
	}
}

func TestFilePlugin_PinyinInitialSearch(t *testing.T) {
	suite := NewTestSuite(t)
	ctx := suite.ctx

	rootPath := newStableFileSearchRoot(t, "filesearch-pinyin-root")

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

	targetFileName := fmt.Sprintf("总结报告-%d.txt", time.Now().UnixNano())
	targetFilePath := filepath.Join(rootPath, targetFileName)
	if err := os.WriteFile(targetFilePath, []byte("pinyin"), 0644); err != nil {
		t.Fatalf("failed to create pinyin target file: %v", err)
	}

	filePlugin.API.SaveSetting(ctx, "roots", string(rootSetting), false)

	if err := waitForFileSearchResult(ctx, "f zjbg", targetFileName, targetFilePath, 30*time.Second); err != nil {
		t.Fatalf("pinyin initials query did not return target file: %v", err)
	}
}

func TestFilePlugin_PolicyUpdateRemovesIndexedPath(t *testing.T) {
	suite := NewTestSuite(t)
	ctx := suite.ctx

	rootPath := newStableFileSearchRoot(t, "filesearch-policy-update-root")

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

	targetFileName := fmt.Sprintf("policy-target-%d.txt", time.Now().UnixNano())
	targetFilePath := filepath.Join(rootPath, targetFileName)
	if err := os.WriteFile(targetFilePath, []byte("indexed"), 0644); err != nil {
		t.Fatalf("failed to create target file: %v", err)
	}

	if err := waitForFileSearchResult(ctx, "f "+targetFileName, targetFileName, targetFilePath, 30*time.Second); err != nil {
		t.Fatalf("target file did not become searchable before policy update: %v", err)
	}

	engine, err := getFileSearchEngine()
	if err != nil {
		t.Fatalf("failed to get file search engine: %v", err)
	}

	engine.UpdatePolicy(filesearch.Policy{
		ShouldIndexPath: func(root filesearch.RootRecord, path string, isDir bool) bool {
			return filepath.Clean(path) != filepath.Clean(targetFilePath)
		},
	})

	if err := ensureFileSearchResultAbsent(ctx, "f "+targetFileName, targetFileName, targetFilePath, 30*time.Second); err != nil {
		t.Fatalf("target file should be evicted after policy update: %v", err)
	}
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
	return runQueryWithSession(ctx, "", rawQuery)
}

func runQueryWithSession(ctx context.Context, sessionID string, rawQuery string) ([]plugin.QueryResultUI, error) {
	if sessionID != "" {
		ctx = util.WithSessionContext(ctx, sessionID)
	}

	query, queryPlugin, err := plugin.GetPluginManager().NewQuery(ctx, common.PlainQuery{
		QueryType: plugin.QueryTypeInput,
		QueryText: rawQuery,
	})
	if err != nil {
		return nil, err
	}

	plugin.GetPluginManager().HandleQueryLifecycle(ctx, query, queryPlugin)
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

func pollUntil(timeout time.Duration, interval time.Duration, check func() (bool, error)) error {
	if timeout <= 0 {
		ok, err := check()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		return fmt.Errorf("condition not met")
	}

	deadline := time.Now().Add(timeout)
	for {
		ok, err := check()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("condition not met within %s", timeout)
		}
		time.Sleep(interval)
	}
}

func waitForFileSearchResult(ctx context.Context, rawQuery string, expectedTitle string, expectedPath string, timeout time.Duration) error {
	var lastResults []plugin.QueryResultUI
	err := pollUntil(timeout, 200*time.Millisecond, func() (bool, error) {
		results, err := runQuery(ctx, rawQuery)
		if err != nil {
			return false, err
		}
		lastResults = results

		for _, result := range results {
			if result.Title == expectedTitle && result.SubTitle == expectedPath {
				return true, nil
			}
		}

		return false, nil
	})
	if err == nil {
		return nil
	}

	summaries := make([]string, 0, len(lastResults))
	for _, result := range lastResults {
		summaries = append(summaries, fmt.Sprintf("%q (%q)", result.Title, result.SubTitle))
	}
	if len(summaries) == 0 {
		return err
	}

	return fmt.Errorf("%w; last results: %s", err, strings.Join(summaries, ", "))
}

func ensureFileSearchResultAbsent(ctx context.Context, rawQuery string, unexpectedTitle string, unexpectedPath string, timeout time.Duration) error {
	var lastResults []plugin.QueryResultUI
	err := pollUntil(timeout, 200*time.Millisecond, func() (bool, error) {
		results, err := runQuery(ctx, rawQuery)
		if err != nil {
			return false, err
		}
		lastResults = results

		for _, result := range results {
			if result.Title == unexpectedTitle && result.SubTitle == unexpectedPath {
				return false, nil
			}
		}

		return true, nil
	})
	if err == nil {
		return nil
	}

	summaries := make([]string, 0, len(lastResults))
	for _, result := range lastResults {
		summaries = append(summaries, fmt.Sprintf("%q (%q)", result.Title, result.SubTitle))
	}
	if len(summaries) == 0 {
		return err
	}

	return fmt.Errorf("%w; last results: %s", err, strings.Join(summaries, ", "))
}

func waitForFileSearchRootReady(ctx context.Context, rootPath string, timeout time.Duration) error {
	engine, err := getFileSearchEngine()
	if err != nil {
		return err
	}

	cleanRootPath := filepath.Clean(rootPath)
	return pollUntil(timeout, 100*time.Millisecond, func() (bool, error) {
		roots, err := engine.ListRoots(ctx)
		if err != nil {
			return false, err
		}

		for _, root := range roots {
			if filepath.Clean(root.Path) != cleanRootPath {
				continue
			}

			return root.Status == filesearch.RootStatusIdle, nil
		}

		return false, nil
	})
}

func getFileSearchEngine() (*filesearch.Engine, error) {
	filePlugin := findPluginInstance("979d6363-025a-4f51-88d3-0b04e9dc56bf")
	if filePlugin == nil {
		return nil, fmt.Errorf("file plugin instance not found")
	}

	value := reflect.ValueOf(filePlugin.Plugin)
	if !value.IsValid() || value.Kind() != reflect.Ptr || value.IsNil() {
		return nil, fmt.Errorf("file plugin implementation is unavailable")
	}

	engineField := value.Elem().FieldByName("engine")
	if !engineField.IsValid() || engineField.IsNil() || !engineField.CanAddr() {
		return nil, fmt.Errorf("file plugin engine is unavailable")
	}

	return reflect.NewAt(engineField.Type(), unsafe.Pointer(engineField.UnsafeAddr())).Elem().Interface().(*filesearch.Engine), nil
}

func newStableFileSearchRoot(t *testing.T, prefix string) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory for stable file search root: %v", err)
	}

	basePath := filepath.Join(cwd, ".tmp-filesearch-roots")
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		t.Fatalf("create stable file search root base: %v", err)
	}

	rootPath, err := os.MkdirTemp(basePath, prefix+"-")
	if err != nil {
		t.Fatalf("create stable file search root: %v", err)
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(rootPath)
	})

	return rootPath
}

func hasAction(result plugin.QueryResultUI, expectedAction string) bool {
	for _, action := range result.Actions {
		if action.Name == expectedAction {
			return true
		}
	}
	return false
}

var (
	testUIWebsocketOnce sync.Once
	testUIWebsocketPort int
	testUIWebsocketErr  error
)

type toolbarObserver struct {
	t      *testing.T
	conn   *websocket.Conn
	mu     sync.Mutex
	titles []string
}

func newToolbarObserver(t *testing.T) *toolbarObserver {
	t.Helper()

	wsURL := ensureTestUIWebsocket(t)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to test UI websocket: %v", err)
	}

	observer := &toolbarObserver{
		t:    t,
		conn: conn,
	}
	go observer.readLoop()
	return observer
}

func (o *toolbarObserver) readLoop() {
	for {
		_, payload, err := o.conn.ReadMessage()
		if err != nil {
			return
		}

		var message map[string]any
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}

		if message["Type"] != "WebsocketMsgTypeRequest" {
			continue
		}

		if method, _ := message["Method"].(string); method == "ShowToolbarMsg" {
			if data, ok := message["Data"].(map[string]any); ok {
				if title, ok := data["Title"].(string); ok && title != "" {
					o.mu.Lock()
					o.titles = append(o.titles, title)
					o.mu.Unlock()
				}
			}
		}

		response := map[string]any{
			"RequestId": message["RequestId"],
			"TraceId":   message["TraceId"],
			"SessionId": message["SessionId"],
			"Type":      "WebsocketMsgTypeResponse",
			"Method":    message["Method"],
			"Success":   true,
			"Data":      nil,
		}
		if err := o.conn.WriteJSON(response); err != nil {
			return
		}
	}
}

func (o *toolbarObserver) HasToolbarTitle(expected string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, title := range o.titles {
		if title == expected {
			return true
		}
	}
	return false
}

func (o *toolbarObserver) HasToolbarTitlePrefix(expected string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, title := range o.titles {
		if strings.HasPrefix(title, expected) {
			return true
		}
	}
	return false
}

func (o *toolbarObserver) ToolbarTitles() []string {
	o.mu.Lock()
	defer o.mu.Unlock()

	return append([]string(nil), o.titles...)
}

func (o *toolbarObserver) Close() {
	_ = o.conn.Close()
}

func ensureTestUIWebsocket(t *testing.T) string {
	t.Helper()

	testUIWebsocketOnce.Do(func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			testUIWebsocketErr = err
			return
		}
		testUIWebsocketPort = listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()

		ui.GetUIManager().UpdateServerPort(testUIWebsocketPort)
		go ui.GetUIManager().StartWebsocketAndWait(context.Background())

		wsURL := testUIWebsocketURL(testUIWebsocketPort)
		testUIWebsocketErr = pollUntil(5*time.Second, 100*time.Millisecond, func() (bool, error) {
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				return false, nil
			}
			_ = conn.Close()
			return true, nil
		})
	})

	if testUIWebsocketErr != nil {
		t.Fatalf("failed to start test UI websocket: %v", testUIWebsocketErr)
	}

	return testUIWebsocketURL(testUIWebsocketPort)
}

func testUIWebsocketURL(port int) string {
	return (&url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("127.0.0.1:%d", port),
		Path:   "/ws",
	}).String()
}
