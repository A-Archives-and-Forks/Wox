package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wox/ai"
	"wox/analytics"
	"wox/common"
	"wox/i18n"
	"wox/setting"

	"wox/util"
	"wox/util/notifier"
	"wox/util/selection"
	"wox/util/window"

	"github.com/Masterminds/semver/v3"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/wissance/stringFormatter"
)

var managerInstance *Manager
var managerOnce sync.Once
var logger *util.Log

const (
	// ContextData key/value for favorite tail
	favoriteTailContextDataKey   = "system:favorite"
	favoriteTailContextDataValue = "true"
	scoreTailContextDataKey      = "system:score"
	previewDataMaxSize           = 1024
)

func serializeContextData(contextData map[string]string) string {
	if len(contextData) == 0 {
		return ""
	}
	data, err := json.Marshal(contextData)
	if err != nil {
		return ""
	}
	return string(data)
}

func appendDevScoreTail(ctx context.Context, tails []QueryResultTail, score int64) []QueryResultTail {
	woxSetting := setting.GetSettingManager().GetWoxSetting(ctx)
	if !util.IsDev() || !woxSetting.ShowScoreTail.Get() {
		return tails
	}

	for _, tail := range tails {
		if tail.ContextData[scoreTailContextDataKey] == "true" {
			return tails
		}
	}

	return append(tails, QueryResultTail{
		Type:         QueryResultTailTypeText,
		Text:         fmt.Sprintf("score:%d", score),
		ContextData:  common.ContextData{scoreTailContextDataKey: "true"},
		IsSystemTail: true,
	})
}

type debounceTimer struct {
	timer  *time.Timer
	onStop func()
}

// queryTracker splits query completion into two phases:
// 1. fallbackReady: all plugins that are allowed to block fallback have finished.
// 2. done: every plugin has finished, including debounced plugins that may return later.
//
// Debounced plugins are counted only in remaining. This lets the UI show fallback
// once the immediate plugins are done, while still keeping the query open for late
// debounced results to arrive.
type queryTracker struct {
	remaining         *atomic.Int32
	fallbackRemaining *atomic.Int32
	fallbackReady     chan bool
	done              chan bool
}

type QueryResultSet struct {
	Query   Query
	Results *util.HashMap[string, *QueryResultCache]
}

func newQueryResultSet(query Query) *QueryResultSet {
	set := &QueryResultSet{
		Query:   query,
		Results: util.NewHashMap[string, *QueryResultCache](),
	}
	return set
}

type Manager struct {
	instances       []*Instance
	systemPluginsWg sync.WaitGroup // waits for all system plugins to finish loading
	ui              common.UI

	// ui session based query result cache. [SessionId] => QueryResultSet
	// one ui session can only have one active query at a time
	sessionQueryResultCache *util.HashMap[string, *QueryResultSet]

	debounceQueryTimer *util.HashMap[string, *debounceTimer]
	aiProviders        *util.HashMap[string, ai.Provider]

	activeBrowserUrl string //active browser url before wox is activated

	// Script plugin monitoring
	scriptPluginWatcher *fsnotify.Watcher
	scriptReloadTimers  *util.HashMap[string, *time.Timer]

	// Plugin query latency tracking (EWMA per plugin)
	pluginQueryLatency *util.HashMap[string, *util.EWMA]

	toolbarMsgActions   *util.HashMap[string, *toolbarMsgActionEntry]
	pluginToolbarMsgIds *util.HashMap[string, string]
	glanceActions       *util.HashMap[string, GlanceAction]

	// sessionPluginQueries tracks which plugin query is currently active for each UI session (sessionId -> state)
	sessionPluginQueries *util.HashMap[string, *sessionPluginQueryState]
}

const (
	systemActionPinInQueryID        = "__system_pin_in_query__"
	systemActionUnpinInQueryID      = "__system_unpin_in_query__"
	systemActionOpenPluginSettingID = "__system_open_plugin_setting__"
)

func GetPluginManager() *Manager {
	managerOnce.Do(func() {
		managerInstance = &Manager{
			sessionQueryResultCache: util.NewHashMap[string, *QueryResultSet](),
			debounceQueryTimer:      util.NewHashMap[string, *debounceTimer](),
			aiProviders:             util.NewHashMap[string, ai.Provider](),
			scriptReloadTimers:      util.NewHashMap[string, *time.Timer](),
			pluginQueryLatency:      util.NewHashMap[string, *util.EWMA](),
			toolbarMsgActions:       util.NewHashMap[string, *toolbarMsgActionEntry](),
			pluginToolbarMsgIds:     util.NewHashMap[string, string](),
			glanceActions:           util.NewHashMap[string, GlanceAction](),
			sessionPluginQueries:    util.NewHashMap[string, *sessionPluginQueryState](),
		}
		logger = util.GetLogger()
	})
	return managerInstance
}

func (m *Manager) Start(ctx context.Context, ui common.UI) error {
	m.ui = ui

	loadErr := m.loadPlugins(ctx)
	if loadErr != nil {
		return fmt.Errorf("failed to load plugins: %w", loadErr)
	}

	// Start script plugin monitoring
	util.Go(ctx, "start script plugin monitoring", func() {
		m.startScriptPluginMonitoring(util.NewTraceContext())
	})

	util.Go(ctx, "start store manager", func() {
		GetStoreManager().Start(util.NewTraceContext())
	})

	return nil
}

func (m *Manager) Stop(ctx context.Context) {
	// Stop script plugin monitoring
	if m.scriptPluginWatcher != nil {
		m.scriptPluginWatcher.Close()
	}

	for _, host := range AllHosts {
		host.Stop(ctx)
	}
}

func (m *Manager) SetActiveBrowserUrl(url string) {
	m.activeBrowserUrl = url
}

func (m *Manager) loadPlugins(ctx context.Context) error {
	logger.Info(ctx, "start loading plugins")

	// load system plugin first
	m.loadSystemPlugins(ctx)

	logger.Debug(ctx, "start loading user plugin metadata")
	basePluginDirectory := util.GetLocation().GetPluginDirectory()
	pluginDirectories, readErr := os.ReadDir(basePluginDirectory)
	if readErr != nil {
		logger.Warn(ctx, fmt.Sprintf("failed to read plugin directory (%s), continue without user plugins: %s", basePluginDirectory, readErr.Error()))
		pluginDirectories = []os.DirEntry{}
	}

	var metaDataList []Metadata
	for _, entry := range pluginDirectories {
		if entry.Name() == ".DS_Store" {
			continue
		}
		if !entry.IsDir() {
			continue
		}

		pluginDirectory := path.Join(basePluginDirectory, entry.Name())
		metadata, metadataErr := m.ParseMetadata(ctx, pluginDirectory)
		if metadataErr != nil {
			logger.Error(ctx, metadataErr.Error())
			continue
		}

		//check if metadata already exist, only add newer version
		existMetadata, exist := lo.Find(metaDataList, func(item Metadata) bool {
			return item.Id == metadata.Id
		})
		if exist {
			existVersion, existVersionErr := semver.NewVersion(existMetadata.Version)
			currentVersion, currentVersionErr := semver.NewVersion(metadata.Version)
			if existVersionErr == nil && currentVersionErr == nil {
				if existVersion.GreaterThan(currentVersion) || existVersion.Equal(currentVersion) {
					logger.Info(ctx, fmt.Sprintf("skip parse %s(%s) metadata, because it's already parsed(%s)", metadata.GetName(ctx), metadata.Version, existMetadata.Version))
					continue
				} else {
					// remove older version
					logger.Info(ctx, fmt.Sprintf("remove older metadata version %s(%s)", existMetadata.GetName(ctx), existMetadata.Version))
					var newMetaDataList []Metadata
					for _, item := range metaDataList {
						if item.Id != existMetadata.Id {
							newMetaDataList = append(newMetaDataList, item)
						}
					}
					metaDataList = newMetaDataList
				}
			}
		}
		metaDataList = append(metaDataList, metadata)
	}

	// Load script plugins
	scriptMetaDataList, err := m.loadScriptPlugins(ctx)
	if err != nil {
		logger.Error(ctx, fmt.Sprintf("failed to load script plugins: %s", err.Error()))
	} else {
		metaDataList = append(metaDataList, scriptMetaDataList...)
	}

	logger.Info(ctx, fmt.Sprintf("start loading user plugins, found %d user plugins", len(metaDataList)))

	for _, host := range AllHosts {
		util.Go(ctx, fmt.Sprintf("[%s] start host", host.GetRuntime(ctx)), func() {
			newCtx := util.NewTraceContext()
			hostErr := host.Start(newCtx)
			if hostErr != nil {
				logger.Error(newCtx, fmt.Errorf("[%s HOST] %w", host.GetRuntime(newCtx), hostErr).Error())
				return
			}

			for _, metadata := range metaDataList {
				if !strings.EqualFold(metadata.Runtime, string(host.GetRuntime(newCtx))) {
					continue
				}

				loadErr := m.loadHostPlugin(newCtx, host, metadata)
				if loadErr != nil {
					logger.Error(newCtx, fmt.Errorf("[%s HOST] %w", host.GetRuntime(newCtx), loadErr).Error())
					continue
				}
			}
		})
	}

	return nil
}

// loadScriptPlugins loads script plugins from the user script plugins directory
func (m *Manager) loadScriptPlugins(ctx context.Context) ([]Metadata, error) {
	logger.Debug(ctx, "start loading script plugin metadata")

	userScriptPluginDirectory := util.GetLocation().GetUserScriptPluginsDirectory()
	scriptFiles, readErr := os.ReadDir(userScriptPluginDirectory)
	if readErr != nil {
		return nil, fmt.Errorf("failed to read user script plugin directory: %w", readErr)
	}

	var metaDataList []Metadata
	for _, entry := range scriptFiles {
		if entry.Name() == ".DS_Store" || entry.Name() == "README.md" {
			continue
		}
		if entry.IsDir() {
			continue
		}

		scriptPath := path.Join(userScriptPluginDirectory, entry.Name())
		metadata, metadataErr := m.ParseScriptMetadata(ctx, scriptPath)
		if metadataErr != nil {
			logger.Error(ctx, fmt.Sprintf("failed to parse script plugin metadata for %s: %s", entry.Name(), metadataErr.Error()))
			continue
		}

		metaDataList = append(metaDataList, metadata)
	}

	logger.Debug(ctx, fmt.Sprintf("found %d script plugins", len(metaDataList)))
	return metaDataList, nil
}

func (m *Manager) ReloadPlugin(ctx context.Context, metadata Metadata) error {
	logger.Info(ctx, fmt.Sprintf("start reloading dev plugin: %s", metadata.GetName(ctx)))

	pluginHost, exist := lo.Find(AllHosts, func(item Host) bool {
		return strings.EqualFold(string(item.GetRuntime(ctx)), metadata.Runtime)
	})
	if !exist {
		return fmt.Errorf("unsupported runtime: %s", metadata.Runtime)
	}

	pluginInstance, pluginInstanceExist := lo.Find(m.instances, func(item *Instance) bool {
		return item.Metadata.Id == metadata.Id
	})
	if pluginInstanceExist {
		logger.Info(ctx, fmt.Sprintf("plugin(%s) is loaded, unload first", metadata.GetName(ctx)))
		m.UnloadPlugin(ctx, pluginInstance)
	} else {
		logger.Info(ctx, fmt.Sprintf("plugin(%s) is not loaded, skip unload", metadata.GetName(ctx)))
	}

	loadErr := m.loadHostPlugin(ctx, pluginHost, metadata)
	if loadErr != nil {
		return loadErr
	}

	return nil
}

func (m *Manager) loadHostPlugin(ctx context.Context, host Host, metadata Metadata) error {
	// Plugin loading is the final shared gate for startup, local installs, and dev
	// reloads. Install-time checks can be bypassed by existing files on disk, so
	// keep this guard here to prevent incompatible plugins from entering runtime hosts.
	if err := ensureWoxVersionSupported(metadata.GetName(ctx), metadata.MinWoxVersion); err != nil {
		return err
	}

	loadStartTimestamp := util.GetSystemTimestamp()
	plugin, loadErr := host.LoadPlugin(ctx, metadata, metadata.Directory)
	if loadErr != nil {
		logger.Error(ctx, fmt.Errorf("[%s HOST] failed to load plugin: %w", host.GetRuntime(ctx), loadErr).Error())
		return loadErr
	}
	loadFinishTimestamp := util.GetSystemTimestamp()

	instance := &Instance{
		Metadata:              metadata,
		PluginDirectory:       metadata.Directory,
		Plugin:                plugin,
		Host:                  host,
		LoadStartTimestamp:    loadStartTimestamp,
		LoadFinishedTimestamp: loadFinishTimestamp,
		IsDevPlugin:           metadata.IsDev,
		DevPluginDirectory:    metadata.DevPluginDirectory,
	}
	instance.API = NewAPI(instance)
	pluginSetting, settingErr := setting.GetSettingManager().LoadPluginSetting(ctx, metadata.Id, metadata.SettingDefinitions.ToMap())
	if settingErr != nil {
		instance.API.Log(ctx, LogLevelError, fmt.Errorf("[SYS] failed to load plugin[%s] setting: %w", metadata.GetName(ctx), settingErr).Error())
		return settingErr
	}
	instance.Setting = pluginSetting

	m.instances = append(m.instances, instance)

	if pluginSetting.Disabled.Get() {
		logger.Info(ctx, fmt.Errorf("[%s HOST] plugin is disabled by user, skip init: %s", host.GetRuntime(ctx), metadata.GetName(ctx)).Error())
		instance.API.Log(ctx, LogLevelWarning, fmt.Sprintf("[SYS] plugin is disabled by user, skip init: %s", metadata.GetName(ctx)))
		return nil
	}

	util.Go(ctx, fmt.Sprintf("[%s] init plugin", metadata.GetName(ctx)), func() {
		m.initPlugin(ctx, instance)
	})

	return nil
}

func (m *Manager) LoadPlugin(ctx context.Context, pluginDirectory string) error {
	metadata, parseErr := m.ParseMetadata(ctx, pluginDirectory)
	if parseErr != nil {
		return parseErr
	}

	pluginHost, exist := lo.Find(AllHosts, func(item Host) bool {
		return strings.EqualFold(string(item.GetRuntime(ctx)), metadata.Runtime)
	})
	if !exist {
		return fmt.Errorf("unsupported runtime: %s", metadata.Runtime)
	}

	// Ensure host is started before loading the plugin
	if !pluginHost.IsStarted(ctx) {
		if err := pluginHost.Start(ctx); err != nil {
			return fmt.Errorf("failed to start host for runtime %s: %w", metadata.Runtime, err)
		}
	}

	loadErr := m.loadHostPlugin(ctx, pluginHost, metadata)
	if loadErr != nil {
		return loadErr
	}

	return nil
}

func (m *Manager) UnloadPlugin(ctx context.Context, pluginInstance *Instance) {
	for _, callback := range pluginInstance.UnloadCallbacks {
		callback(ctx)
	}
	pluginInstance.Host.UnloadPlugin(ctx, pluginInstance.Metadata)

	var newInstances []*Instance
	for _, instance := range m.instances {
		if instance.Metadata.Id != pluginInstance.Metadata.Id {
			newInstances = append(newInstances, instance)
		}
	}
	m.instances = newInstances
}

func (m *Manager) RestartHostForRuntime(ctx context.Context, runtime Runtime, skipPluginIDs []string, progressCallback UninstallProgressCallback) error {
	pluginHost, exist := lo.Find(AllHosts, func(item Host) bool {
		return strings.EqualFold(string(item.GetRuntime(ctx)), string(runtime))
	})
	if !exist {
		return fmt.Errorf("unsupported runtime: %s", runtime)
	}

	skipPluginIDSet := make(map[string]struct{}, len(skipPluginIDs))
	for _, pluginID := range skipPluginIDs {
		skipPluginIDSet[pluginID] = struct{}{}
	}

	var reloadMetadataList []Metadata
	var nextInstances []*Instance
	for _, instance := range m.instances {
		if !strings.EqualFold(instance.Metadata.Runtime, string(runtime)) {
			nextInstances = append(nextInstances, instance)
			continue
		}
		if _, shouldSkip := skipPluginIDSet[instance.Metadata.Id]; shouldSkip {
			continue
		}
		reloadMetadataList = append(reloadMetadataList, instance.Metadata)
	}

	// Bug fix: a shared runtime host can keep process-wide native modules loaded even after one
	// plugin unregisters. Restart the host so uninstall can retry with fresh process state.
	pluginHost.Stop(ctx)

	if progressCallback != nil {
		progressCallback(i18n.GetI18nManager().TranslateWox(ctx, "i18n:plugin_uninstall_progress_starting_host"))
	}
	if err := pluginHost.Start(ctx); err != nil {
		return fmt.Errorf("failed to restart %s host: %w", runtime, err)
	}

	// Replace stale runtime instances only after the new host is available, then rebuild the
	// remaining plugins from metadata so the shared runtime returns to a consistent state.
	m.instances = nextInstances

	if len(reloadMetadataList) == 0 {
		return nil
	}

	var reloadErrors []string
	for _, metadata := range reloadMetadataList {
		if progressCallback != nil {
			progressCallback(fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "i18n:plugin_uninstall_progress_restoring_runtime_plugin"), metadata.GetName(ctx)))
		}
		if err := m.loadHostPlugin(ctx, pluginHost, metadata); err != nil {
			logger.Error(ctx, fmt.Sprintf("failed to reload %s plugin %s(%s) after host restart: %s", runtime, metadata.GetName(ctx), metadata.Version, err.Error()))
			reloadErrors = append(reloadErrors, fmt.Sprintf("%s(%s)", metadata.GetName(ctx), metadata.Version))
		}
	}

	if len(reloadErrors) > 0 {
		return fmt.Errorf("failed to reload %s host plugins after restart: %s", runtime, strings.Join(reloadErrors, ", "))
	}

	return nil
}

func (m *Manager) loadSystemPlugins(ctx context.Context) {
	start := util.GetSystemTimestamp()
	logger.Info(ctx, fmt.Sprintf("start loading system plugins, found %d system plugins", len(AllSystemPlugin)))

	// Add all plugins to wait group before starting goroutines
	m.systemPluginsWg.Add(len(AllSystemPlugin))

	for _, p := range AllSystemPlugin {
		plugin := p
		metadata := plugin.GetMetadata()
		pluginName := metadata.GetName(ctx)

		util.Go(ctx, fmt.Sprintf("load system plugin <%s>", pluginName), func() {
			defer m.systemPluginsWg.Done()

			metadata := plugin.GetMetadata()
			metadata.LoadSystemI18nFromDirectory(ctx)
			instance := &Instance{
				Metadata:              metadata,
				Plugin:                plugin,
				Host:                  nil,
				IsSystemPlugin:        true,
				PluginDirectory:       metadata.Directory,
				LoadStartTimestamp:    util.GetSystemTimestamp(),
				LoadFinishedTimestamp: util.GetSystemTimestamp(),
			}
			instance.API = NewAPI(instance)

			startTimestamp := util.GetSystemTimestamp()
			pluginSetting, settingErr := setting.GetSettingManager().LoadPluginSetting(ctx, metadata.Id, metadata.SettingDefinitions.ToMap())
			if settingErr != nil {
				logger.Error(ctx, fmt.Sprintf("failed to load system plugin[%s] setting, use default plugin setting. err: %s", metadata.GetName(ctx), settingErr.Error()))
				return
			}

			instance.Setting = pluginSetting
			if util.GetSystemTimestamp()-startTimestamp > 100 {
				logger.Warn(ctx, fmt.Sprintf("load system plugin[%s] setting too slow, cost %d ms", metadata.GetName(ctx), util.GetSystemTimestamp()-startTimestamp))
			}

			// Init plugin BEFORE adding to instances list
			// This ensures the plugin is fully initialized before it can be queried
			m.initPlugin(util.NewTraceContext(), instance)

			m.instances = append(m.instances, instance)
		})
	}

	logger.Debug(ctx, fmt.Sprintf("finish loading system plugins, cost %d ms", util.GetSystemTimestamp()-start))
}

func (m *Manager) initPlugin(ctx context.Context, instance *Instance) {
	logger.Info(ctx, fmt.Sprintf("start init plugin: %s", instance.Metadata.GetName(ctx)))
	instance.InitStartTimestamp = util.GetSystemTimestamp()
	instance.Plugin.Init(ctx, InitParams{
		API:             instance.API,
		PluginDirectory: instance.PluginDirectory,
	})
	instance.InitFinishedTimestamp = util.GetSystemTimestamp()
	logger.Info(ctx, fmt.Sprintf("init plugin %s finished, cost %d ms", instance.Metadata.GetName(ctx), instance.InitFinishedTimestamp-instance.InitStartTimestamp))
}

func (m *Manager) ParseMetadata(ctx context.Context, pluginDirectory string) (Metadata, error) {
	configPath := path.Join(pluginDirectory, "plugin.json")
	if _, statErr := os.Stat(configPath); statErr != nil {
		return Metadata{}, fmt.Errorf("missing plugin.json file in %s", configPath)
	}

	metadataJson, err := os.ReadFile(configPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to read plugin.json file: %w", err)
	}

	var metadata Metadata
	unmarshalErr := json.Unmarshal(metadataJson, &metadata)
	if unmarshalErr != nil {
		return Metadata{}, fmt.Errorf("failed to unmarshal plugin.json file (%s): %w", pluginDirectory, unmarshalErr)
	}

	if len(metadata.TriggerKeywords) == 0 {
		return Metadata{}, fmt.Errorf("missing trigger keywords in plugin.json file (%s)", pluginDirectory)
	}
	if !IsSupportedRuntime(metadata.Runtime) {
		return Metadata{}, fmt.Errorf("unsupported runtime in plugin.json file (%s), runtime=%s", pluginDirectory, metadata.Runtime)
	}
	if !IsAllSupportedOS(metadata.SupportedOS) {
		return Metadata{}, fmt.Errorf("unsupported os in plugin.json file (%s), os=%s", pluginDirectory, metadata.SupportedOS)
	}
	if err := metadata.ValidateGlances(); err != nil {
		// Global Glance selections are persisted as PluginId + GlanceId, so the
		// backend must reject ambiguous plugin-local ids before settings can point
		// at a candidate that cannot be resolved deterministically.
		return Metadata{}, fmt.Errorf("invalid glances in plugin.json file (%s): %w", pluginDirectory, err)
	}

	metadata.Directory = pluginDirectory
	metadata.LoadPluginI18nFromDirectory(ctx)

	return metadata, nil
}

// ParseScriptMetadata parses metadata from script plugin file comments
// Supports formats:
// 1. JSON block format (preferred): # { ... } with complete plugin.json structure
func (m *Manager) ParseScriptMetadata(ctx context.Context, scriptPath string) (Metadata, error) {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to read script file: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	// Parse JSON block format
	var jsonBuilder strings.Builder
	inJsonBlock := false
	jsonStartLine := -1
	braceDepth := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Stop parsing when we reach non-comment lines (except shebang)
		if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "#!/") {
			if trimmed != "" {
				break
			}
			continue
		}

		// Remove comment markers
		cleaned := strings.TrimPrefix(trimmed, "#")
		cleaned = strings.TrimPrefix(cleaned, "//")
		cleaned = strings.TrimSpace(cleaned)

		// Check for JSON block start
		if !inJsonBlock && cleaned == "{" {
			inJsonBlock = true
			jsonStartLine = i
			braceDepth = 1
			jsonBuilder.WriteString(cleaned)
			continue
		}

		// Collect JSON content
		if inJsonBlock {
			jsonBuilder.WriteString("\n")
			jsonBuilder.WriteString(cleaned)

			// Track brace depth
			for _, ch := range cleaned {
				if ch == '{' {
					braceDepth++
				} else if ch == '}' {
					braceDepth--
				}
			}

			// Check for JSON block end (when braces are balanced)
			if braceDepth == 0 {
				// Try to parse the collected JSON
				jsonStr := jsonBuilder.String()
				var metadata Metadata
				unmarshalErr := json.Unmarshal([]byte(jsonStr), &metadata)
				if unmarshalErr != nil {
					return Metadata{}, fmt.Errorf("failed to parse JSON metadata block (starting at line %d): %w", jsonStartLine+1, unmarshalErr)
				}

				// Set script-specific fields
				metadata.Runtime = string(PLUGIN_RUNTIME_SCRIPT)
				metadata.Entry = filepath.Base(scriptPath)
				metadata.Directory = filepath.Dir(scriptPath)
				metadata.LoadPluginI18nFromDirectory(ctx)

				// Validate and set defaults
				return m.validateAndSetScriptMetadataDefaults(metadata)
			}
		}
	}

	// No JSON block found
	return Metadata{}, fmt.Errorf("no JSON metadata block found in script file. Script plugins must define metadata as a JSON object in comments")
}

// validateAndSetScriptMetadataDefaults validates required fields and sets default values
func (m *Manager) validateAndSetScriptMetadataDefaults(metadata Metadata) (Metadata, error) {
	// Validate required fields
	if metadata.Id == "" {
		return Metadata{}, fmt.Errorf("missing required field: Id")
	}
	if metadata.GetName(context.Background()) == "" {
		return Metadata{}, fmt.Errorf("missing required field: Name")
	}
	if len(metadata.TriggerKeywords) == 0 {
		return Metadata{}, fmt.Errorf("missing required field: TriggerKeywords")
	}

	// Set default values
	if metadata.Author == "" {
		metadata.Author = "Unknown"
	}
	if metadata.GetDescription(context.Background()) == "" {
		metadata.Description = "A script plugin"
	}
	if metadata.Icon == "" {
		metadata.Icon = "emoji:📝"
	}
	if metadata.MinWoxVersion == "" {
		// Script plugins can omit MinWoxVersion in their inline metadata. Use the
		// same default as packaged plugins so the later compatibility check has a
		// concrete semantic floor instead of treating the field as absent.
		metadata.MinWoxVersion = defaultMinWoxVersion
	}
	if metadata.Version == "" {
		metadata.Version = "1.0.0"
	}

	// Set supported OS to all platforms by default
	if len(metadata.SupportedOS) == 0 {
		metadata.SupportedOS = []string{"Windows", "Linux", "Macos"}
	}

	return metadata, nil
}

// startScriptPluginMonitoring starts monitoring the user script plugins directory for changes
func (m *Manager) startScriptPluginMonitoring(ctx context.Context) {
	userScriptPluginDirectory := util.GetLocation().GetUserScriptPluginsDirectory()

	// Create file system watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error(ctx, fmt.Sprintf("Failed to create script plugin watcher: %s", err.Error()))
		return
	}

	m.scriptPluginWatcher = watcher

	// Add the script plugins directory to the watcher
	err = watcher.Add(userScriptPluginDirectory)
	if err != nil {
		logger.Error(ctx, fmt.Sprintf("Failed to watch script plugin directory: %s", err.Error()))
		watcher.Close()
		return
	}

	logger.Info(ctx, fmt.Sprintf("Started monitoring script plugins directory: %s", userScriptPluginDirectory))

	// Start watching for events
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				logger.Info(ctx, "Script plugin watcher closed")
				return
			}
			m.handleScriptPluginEvent(ctx, event)
		case err, ok := <-watcher.Errors:
			if !ok {
				logger.Info(ctx, "Script plugin watcher error channel closed")
				return
			}
			logger.Error(ctx, fmt.Sprintf("Script plugin watcher error: %s", err.Error()))
		case <-ctx.Done():
			logger.Info(ctx, "Script plugin monitoring stopped due to context cancellation")
			watcher.Close()
			return
		}
	}
}

// handleScriptPluginEvent handles file system events for script plugins
func (m *Manager) handleScriptPluginEvent(ctx context.Context, event fsnotify.Event) {
	// Skip non-script files and temporary files
	fileName := filepath.Base(event.Name)
	if strings.HasPrefix(fileName, ".") || strings.HasSuffix(fileName, "~") || strings.HasSuffix(fileName, ".tmp") {
		return
	}

	// Skip directories
	if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
		return
	}

	logger.Info(ctx, fmt.Sprintf("Script plugin file event: %s (%s)", event.Name, event.Op))

	switch event.Op {
	case fsnotify.Create, fsnotify.Write:
		// File created or modified - reload the plugin
		m.debounceScriptPluginReload(ctx, event.Name, "file changed")
	case fsnotify.Remove, fsnotify.Rename:
		// File deleted or renamed - unload the plugin
		m.unloadScriptPluginByPath(ctx, event.Name)
	case fsnotify.Chmod:
		// File permissions changed - ignore for now
		logger.Debug(ctx, fmt.Sprintf("Script plugin file permissions changed: %s", event.Name))
	}
}

// debounceScriptPluginReload debounces script plugin reload to avoid multiple reloads in a short time
func (m *Manager) debounceScriptPluginReload(ctx context.Context, scriptPath, reason string) {
	fileName := filepath.Base(scriptPath)

	// Cancel existing timer if any
	if timer, exists := m.scriptReloadTimers.Load(fileName); exists {
		timer.Stop()
	}

	// Create new timer
	timer := time.AfterFunc(2*time.Second, func() {
		m.reloadScriptPlugin(util.NewTraceContext(), scriptPath, reason)
		m.scriptReloadTimers.Delete(fileName)
	})

	m.scriptReloadTimers.Store(fileName, timer)
}

// reloadScriptPlugin reloads a script plugin
func (m *Manager) reloadScriptPlugin(ctx context.Context, scriptPath, reason string) {
	logger.Info(ctx, fmt.Sprintf("Reloading script plugin: %s, reason: %s", scriptPath, reason))

	// Check if file still exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		logger.Warn(ctx, fmt.Sprintf("Script plugin file no longer exists: %s", scriptPath))
		return
	}

	// Parse metadata from the script file
	metadata, err := m.ParseScriptMetadata(ctx, scriptPath)
	if err != nil {
		logger.Error(ctx, fmt.Sprintf("Failed to parse script plugin metadata: %s", err.Error()))
		return
	}

	// Find and unload existing plugin instance if any
	existingInstance, exists := lo.Find(m.instances, func(instance *Instance) bool {
		return instance.Metadata.Id == metadata.Id
	})
	if exists {
		logger.Info(ctx, fmt.Sprintf("Unloading existing script plugin instance: %s", metadata.GetName(ctx)))
		m.UnloadPlugin(ctx, existingInstance)
	}

	// Find script plugin host
	scriptHost, hostExists := lo.Find(AllHosts, func(host Host) bool {
		return host.GetRuntime(ctx) == PLUGIN_RUNTIME_SCRIPT
	})
	if !hostExists {
		logger.Error(ctx, "Script plugin host not found")
		return
	}

	// Load the plugin
	err = m.loadHostPlugin(ctx, scriptHost, metadata)
	if err != nil {
		logger.Error(ctx, fmt.Sprintf("Failed to reload script plugin: %s", err.Error()))
		return
	}

	logger.Info(ctx, fmt.Sprintf("Successfully reloaded script plugin: %s", metadata.GetName(ctx)))
}

// unloadScriptPluginByPath unloads a script plugin by its file path
func (m *Manager) unloadScriptPluginByPath(ctx context.Context, scriptPath string) {
	fileName := filepath.Base(scriptPath)
	logger.Info(ctx, fmt.Sprintf("Unloading script plugin: %s", fileName))

	// Find plugin instance by script file name
	var pluginToUnload *Instance
	for _, instance := range m.instances {
		if instance.Metadata.Runtime == string(PLUGIN_RUNTIME_SCRIPT) && instance.Metadata.Entry == fileName {
			pluginToUnload = instance
			break
		}
	}

	if pluginToUnload != nil {
		logger.Info(ctx, fmt.Sprintf("Found script plugin to unload: %s", pluginToUnload.Metadata.GetName(ctx)))
		m.UnloadPlugin(ctx, pluginToUnload)
	} else {
		logger.Debug(ctx, fmt.Sprintf("No script plugin found for file: %s", fileName))
	}
}

// WaitForSystemPlugins blocks until all system plugins have finished loading and initializing.
// This is useful for tests or callers that need to ensure all plugins are ready before querying.
func (m *Manager) WaitForSystemPlugins() {
	m.systemPluginsWg.Wait()
}

func (m *Manager) GetPluginInstances() []*Instance {
	return m.instances
}

func (m *Manager) GetPluginInstanceById(pluginId string) *Instance {
	for _, instance := range m.instances {
		if instance.Metadata.Id == pluginId {
			return instance
		}
	}
	return nil
}

func (m *Manager) canOperateQuery(ctx context.Context, pluginInstance *Instance, query Query) bool {
	if pluginInstance.Setting.Disabled.Get() {
		return false
	}

	if query.Type == QueryTypeSelection {
		isPluginSupportSelection := pluginInstance.Metadata.IsSupportFeature(MetadataFeatureQuerySelection)
		return isPluginSupportSelection
	}

	var validGlobalQuery = lo.Contains(pluginInstance.GetTriggerKeywords(), "*") && query.TriggerKeyword == ""
	var validNonGlobalQuery = lo.Contains(pluginInstance.GetTriggerKeywords(), query.TriggerKeyword)
	if !validGlobalQuery && !validNonGlobalQuery {
		return false
	}

	return true
}

func (m *Manager) queryForPlugin(ctx context.Context, pluginInstance *Instance, query Query) (results []QueryResult) {
	defer util.GoRecover(ctx, fmt.Sprintf("<%s> query panic", pluginInstance.GetName(ctx)), func(err error) {
		// if plugin query panic, return error result
		failedResult := m.GetResultForFailedQuery(ctx, pluginInstance.Metadata, query, err)
		results = []QueryResult{
			m.PolishResult(ctx, pluginInstance, query, failedResult),
		}
	})

	// if plugin query requirement not met, return requirement result without calling plugin.Query
	if requirementResult, blocked := m.buildQueryRequirementSettingsResult(ctx, pluginInstance, query); blocked {
		return []QueryResult{
			m.PolishResult(ctx, pluginInstance, query, requirementResult),
		}
	}

	logger.Info(ctx, fmt.Sprintf("<%s> start query: %s", pluginInstance.GetName(ctx), query.RawQuery))
	start := util.GetSystemTimestamp()

	// set query env base on plugin's feature
	currentEnv := query.Env
	newEnv := QueryEnv{}
	if pluginInstance.Metadata.IsSupportFeature(MetadataFeatureQueryEnv) {
		queryEnvParams, err := pluginInstance.Metadata.GetFeatureParamsForQueryEnv()
		if err != nil {
			logger.Error(ctx, fmt.Sprintf("<%s> invalid query env config: %s", pluginInstance.GetName(ctx), err))
		} else {
			if queryEnvParams.RequireActiveWindowName {
				newEnv.ActiveWindowTitle = currentEnv.ActiveWindowTitle
			}
			if queryEnvParams.RequireActiveWindowPid {
				newEnv.ActiveWindowPid = currentEnv.ActiveWindowPid
			}
			if queryEnvParams.RequireActiveWindowIcon {
				newEnv.ActiveWindowIcon = currentEnv.ActiveWindowIcon
			}
			if queryEnvParams.RequireActiveWindowIsOpenSaveDialog {
				newEnv.ActiveWindowIsOpenSaveDialog = currentEnv.ActiveWindowIsOpenSaveDialog
			}
			if queryEnvParams.RequireActiveBrowserUrl {
				newEnv.ActiveBrowserUrl = currentEnv.ActiveBrowserUrl
			}
		}
	}
	query.Env = newEnv

	results = pluginInstance.Plugin.Query(ctx, query)
	pluginQueryCost := util.GetSystemTimestamp() - start

	// Keep the plugin latency EWMA scoped to Plugin.Query itself.
	// Manager-side polishing is shared overhead layered on top of plugin execution.
	m.updatePluginQueryLatency(pluginInstance.Metadata.Id, float64(pluginQueryCost))

	for i := range results {
		defaultActions := m.getDefaultActions(ctx, pluginInstance, query, results[i].Title, results[i].SubTitle)
		results[i].Actions = append(results[i].Actions, defaultActions...)
		results[i] = m.PolishResult(ctx, pluginInstance, query, results[i])
	}

	if query.Type == QueryTypeSelection && query.Search != "" {
		results = lo.Filter(results, func(item QueryResult, _ int) bool {
			return IsStringMatch(ctx, item.Title, query.Search)
		})
	}

	if pluginQueryCost >= 10 {
		logger.Debug(ctx, fmt.Sprintf("<%s> finish query, result count: %d, cost: %dms, query is slow", pluginInstance.GetName(ctx), len(results), pluginQueryCost))
	} else {
		logger.Debug(ctx, fmt.Sprintf("<%s> finish query, result count: %d, cost: %dms", pluginInstance.GetName(ctx), len(results), pluginQueryCost))
	}

	return results
}

func (m *Manager) GetResultForFailedQuery(ctx context.Context, pluginMetadata Metadata, query Query, err error) QueryResult {
	overlayIcon := common.NewWoxImageEmoji("🚫")
	pluginIcon := common.ParseWoxImageOrDefault(pluginMetadata.Icon, overlayIcon)
	icon := pluginIcon.OverlayFullPercentage(overlayIcon, 0.6)
	pluginName := pluginMetadata.GetName(ctx)

	return QueryResult{
		Title:    fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "plugin_manager_query_failed"), pluginName),
		SubTitle: util.EllipsisEnd(err.Error(), 20),
		Icon:     icon,
		Preview: WoxPreview{
			PreviewType: WoxPreviewTypeText,
			PreviewData: err.Error(),
		},
	}
}

func (m *Manager) getDefaultActions(ctx context.Context, pluginInstance *Instance, query Query, title, subTitle string) (defaultActions []QueryResultAction) {
	// Declare both actions first
	var addToFavoriteAction func(context.Context, ActionContext)
	var removeFromFavoriteAction func(context.Context, ActionContext)

	// Define add to favorite action
	addToFavoriteAction = func(ctx context.Context, actionContext ActionContext) {
		setting.GetSettingManager().PinResult(ctx, pluginInstance.Metadata.Id, title, subTitle)

		// Get API instance
		api := NewAPI(pluginInstance)
		api.Notify(ctx, "i18n:plugin_manager_pin_in_query_success")

		// Get current result state
		updatableResult := api.GetUpdatableResult(ctx, actionContext.ResultId)
		if updatableResult == nil {
			return // Result no longer visible
		}

		// Update the result to refresh UI
		// Note: We don't need to manually add favorite tail here because:
		// 1. GetUpdatableResult filters out system tails (including favorite icon)
		// 2. PolishUpdatableResult will automatically add favorite tail back if this is a favorite result
		// 3. This ensures the favorite tail is always managed by the system
		api.UpdateResult(ctx, *updatableResult)
	}

	// Define remove from favorite action
	removeFromFavoriteAction = func(ctx context.Context, actionContext ActionContext) {
		setting.GetSettingManager().UnpinResult(ctx, pluginInstance.Metadata.Id, title, subTitle)

		// Get API instance
		api := NewAPI(pluginInstance)
		api.Notify(ctx, "i18n:plugin_manager_unpin_in_query")

		// Get current result state
		updatableResult := api.GetUpdatableResult(ctx, actionContext.ResultId)
		if updatableResult == nil {
			return // Result no longer visible
		}

		// Update the result to refresh UI
		// Note: We don't need to manually remove favorite tail here because:
		// 1. GetUpdatableResult filters out system tails (including favorite icon)
		// 2. PolishUpdatableResult will NOT add favorite tail back if this is not a favorite result
		// 3. This ensures the favorite tail is always managed by the system
		api.UpdateResult(ctx, *updatableResult)
	}

	if setting.GetSettingManager().IsPinedResult(ctx, pluginInstance.Metadata.Id, title, subTitle) {
		defaultActions = append(defaultActions, QueryResultAction{
			Id:                     systemActionUnpinInQueryID,
			Name:                   "i18n:plugin_manager_unpin_in_query",
			Icon:                   common.UnpinIcon,
			IsSystemAction:         true,
			PreventHideAfterAction: true,
			Action:                 removeFromFavoriteAction,
		})
	} else {
		defaultActions = append(defaultActions, QueryResultAction{
			Id:                     systemActionPinInQueryID,
			Name:                   "i18n:plugin_manager_pin_in_query",
			Icon:                   common.PinIcon,
			IsSystemAction:         true,
			PreventHideAfterAction: true,
			Action:                 addToFavoriteAction,
		})
	}

	defaultActions = append(defaultActions, m.newOpenPluginSettingAction(ctx, pluginInstance))

	return defaultActions
}

func (m *Manager) newOpenPluginSettingAction(ctx context.Context, pluginInstance *Instance) QueryResultAction {
	return QueryResultAction{
		Id:                     systemActionOpenPluginSettingID,
		Name:                   fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "plugin_sys_open_plugin_settings"), pluginInstance.GetName(ctx)),
		Icon:                   common.SettingIcon,
		IsSystemAction:         true,
		PreventHideAfterAction: true,
		Action: func(ctx context.Context, actionContext ActionContext) {
			m.ui.OpenSettingWindow(ctx, common.SettingWindowContext{
				Path:  "/plugin/setting",
				Param: pluginInstance.Metadata.Id,
			})
		},
	}
}

func (m *Manager) formatFileListPreview(ctx context.Context, filePaths []string) string {
	totalFiles := len(filePaths)
	if totalFiles == 0 {
		return i18n.GetI18nManager().TranslateWox(ctx, "selection_no_files_selected")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "selection_selected_files_count"), totalFiles))
	sb.WriteString("\n\n")

	maxDisplayFiles := 10
	for i, filePath := range filePaths {
		if i < maxDisplayFiles {
			sb.WriteString(fmt.Sprintf("- `%s`\n", filePath))
		} else {
			remainingFiles := totalFiles - maxDisplayFiles
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "selection_remaining_files_not_shown"), remainingFiles))
			break
		}
	}

	return sb.String()
}

func (m *Manager) buildSelectionFileListPreviewData(ctx context.Context, filePaths []string) string {
	items := make([]WoxPreviewListItem, 0, len(filePaths))
	for _, filePath := range filePaths {
		icon := common.ConvertIcon(ctx, common.NewWoxImageFileIcon(filePath), "")
		extension := strings.TrimPrefix(filepath.Ext(filePath), ".")
		typeLabel := strings.ToUpper(extension)
		if typeLabel == "" {
			typeLabel = "FILE"
		}

		items = append(items, WoxPreviewListItem{
			Icon:     &icon,
			Title:    filepath.Base(filePath),
			Subtitle: filepath.Dir(filePath),
			Tails:    []QueryResultTail{NewQueryResultTailText(typeLabel)},
		})
	}

	// Selection file previews now use the generic list contract. The previous
	// file-path-only payload could not express progress/status rows, so core maps
	// file selections into normal preview rows instead of keeping a file-specific
	// renderer alive.
	previewData, err := json.Marshal(WoxPreviewListData{Items: items})
	if err != nil {
		// Selection previews used to be markdown strings, which forced the UI to
		// render paths as generic text. Fall back to that stable legacy format if
		// JSON encoding ever fails so selection query preview still works.
		util.GetLogger().Warn(ctx, fmt.Sprintf("failed to marshal selection list preview data: %s", err.Error()))
		return m.formatFileListPreview(ctx, filePaths)
	}

	return string(previewData)
}

func (m *Manager) normalizeListPreviewData(ctx context.Context, pluginInstance *Instance, preview WoxPreview) WoxPreview {
	if preview.PreviewType != WoxPreviewTypeList || preview.PreviewData == "" {
		return preview
	}

	var data WoxPreviewListData
	if err := json.Unmarshal([]byte(preview.PreviewData), &data); err != nil {
		// Leave malformed payloads untouched so the UI can show its existing
		// preview error. Core only normalizes well-formed list rows.
		return preview
	}

	for itemIndex := range data.Items {
		item := &data.Items[itemIndex]
		item.Title = m.translatePlugin(ctx, pluginInstance, item.Title)
		item.Subtitle = m.translatePlugin(ctx, pluginInstance, item.Subtitle)

		if item.Icon != nil && !item.Icon.IsEmpty() {
			convertedIcon := common.ConvertIcon(ctx, *item.Icon, pluginInstance.PluginDirectory)
			item.Icon = &convertedIcon
		}

		for tailIndex := range item.Tails {
			tail := &item.Tails[tailIndex]
			if tail.Type == QueryResultTailTypeText {
				tail.Text = m.translatePlugin(ctx, pluginInstance, tail.Text)
			}
			if tail.Type == QueryResultTailTypeImage {
				tail.Image = common.ConvertIcon(ctx, tail.Image, pluginInstance.PluginDirectory)
			}
		}
	}

	// Re-encode after translation and icon normalization so preview rows render
	// with the same i18n and image behavior as top-level result rows.
	normalizedData, err := json.Marshal(data)
	if err != nil {
		util.GetLogger().Warn(ctx, fmt.Sprintf("failed to marshal normalized list preview data: %s", err.Error()))
		return preview
	}

	preview.PreviewData = string(normalizedData)
	return preview
}

func (m *Manager) calculateResultScore(ctx context.Context, pluginId, title, subTitle string, currentQuery string) int64 {
	var score int64 = 0

	resultHash := setting.NewResultHash(pluginId, title, subTitle)
	woxSetting := setting.GetSettingManager().GetWoxSetting(ctx)
	actionResults, ok := woxSetting.ActionedResults.Get().Load(resultHash)
	if !ok {
		return score
	}

	// actioned score are based on actioned counts, the more actioned, the more score
	// also, action timestamp will be considered, the more recent actioned, the more score weight. If action is in recent 7 days, it will be considered as recent actioned and add score weight
	// we will use fibonacci sequence to calculate score, the more recent actioned, the more score: 5, 8, 13, 21, 34, 55, 89
	// that means, actions in day one, we will add weight 89, day two, we will add weight 55, day three, we will add weight 34, and so on
	// E.g. if actioned 3 times in day one, 2 times in day two, 1 time in day three, the score will be: 89*3 + 55*2 + 34*1 = 450

	for _, actionResult := range actionResults {
		var weight int64 = 2

		hours := (util.GetSystemTimestamp() - actionResult.Timestamp) / 1000 / 60 / 60
		if hours < 24*7 {
			fibonacciIndex := int(math.Ceil(float64(hours) / 24))
			if fibonacciIndex > 7 {
				fibonacciIndex = 7
			}
			if fibonacciIndex < 1 {
				fibonacciIndex = 1
			}
			fibonacci := []int64{5, 8, 13, 21, 34, 55, 89}
			score += fibonacci[7-fibonacciIndex]
		}

		// If the current query is within the historical selected actions, it indicates a stronger connection and increases the score.
		if currentQuery != "" && actionResult.Query == currentQuery {
			score += 20
		}

		score += weight
	}

	return score
}

func (m *Manager) startSessionQueryCache(query Query) {
	m.sessionQueryResultCache.Store(query.SessionId, newQueryResultSet(query))
}

func (m *Manager) getQueryResultSet(sessionId string) (*QueryResultSet, bool) {
	return m.sessionQueryResultCache.Load(sessionId)
}

func (m *Manager) getQueryResultSetForQuery(query Query) (*QueryResultSet, bool) {
	if query.Id == "" {
		return nil, false
	}
	set, ok := m.sessionQueryResultCache.Load(query.SessionId)
	if !ok {
		return nil, false
	}
	if set.Query.Id != query.Id {
		return nil, false
	}
	return set, true
}

func (m *Manager) storeQueryResult(ctx context.Context, pluginInstance *Instance, query Query, resultOriginal QueryResult) {
	if query.Id == "" {
		logger.Warn(ctx, "query id is empty, skip result cache")
		return
	}
	if query.SessionId == "" {
		logger.Warn(ctx, "query session id is empty, skip result cache")
		return
	}

	set, ok := m.getQueryResultSetForQuery(query)
	if !ok {
		return
	}

	set.Results.Store(resultOriginal.Id, &QueryResultCache{
		Result:         resultOriginal,
		PluginInstance: pluginInstance,
		Query:          query,
	})

}

func (m *Manager) RecordQueryResultQueryElapsed(sessionId string, queryId string, results []QueryResultUI, elapsedMs int64) {
	if sessionId == "" || queryId == "" || len(results) == 0 {
		return
	}

	set, found := m.sessionQueryResultCache.Load(sessionId)
	if !found || set.Query.Id != queryId {
		return
	}

	for _, result := range results {
		if result.Id == "" {
			continue
		}

		resultCache, ok := set.Results.Load(result.Id)
		if !ok {
			continue
		}
		if resultCache.QueryElapsedSet {
			continue
		}

		resultCache.QueryElapsed = elapsedMs
		resultCache.QueryElapsedSet = true
	}
}

func (m *Manager) RecordQueryResultFlushBatch(sessionId string, queryId string, results []QueryResultUI, batch int) {
	if sessionId == "" || queryId == "" || len(results) == 0 || batch <= 0 {
		return
	}

	set, found := m.sessionQueryResultCache.Load(sessionId)
	if !found || set.Query.Id != queryId {
		return
	}

	for _, result := range results {
		if result.Id == "" {
			continue
		}

		resultCache, ok := set.Results.Load(result.Id)
		if !ok {
			continue
		}
		if resultCache.FlushBatch > 0 {
			continue
		}

		resultCache.FlushBatch = batch
	}
}

func (m *Manager) GetQueryResultDebugInfo(sessionId string, queryId string, resultId string) (batch int, queryElapsed int64, ok bool) {
	resultCache, found := m.findResultCacheInSession(sessionId, queryId, resultId)
	if !found {
		return 0, 0, false
	}
	if resultCache.FlushBatch <= 0 || !resultCache.QueryElapsedSet {
		return 0, 0, false
	}

	return resultCache.FlushBatch, resultCache.QueryElapsed, true
}

func (m *Manager) findResultCacheInSession(sessionId string, queryId string, resultId string) (*QueryResultCache, bool) {
	if resultId == "" {
		return nil, false
	}
	set, found := m.sessionQueryResultCache.Load(sessionId)
	if !found {
		return nil, false
	}
	if queryId != "" && set.Query.Id != queryId {
		return nil, false
	}
	resultCache, found := set.Results.Load(resultId)
	if !found {
		return nil, false
	}
	return resultCache, true
}

func (m *Manager) findResultCacheById(resultId string) (*QueryResultCache, bool) {
	if resultId == "" {
		return nil, false
	}

	var foundCache *QueryResultCache
	m.sessionQueryResultCache.Range(func(_ string, set *QueryResultSet) bool {
		resultCache, found := set.Results.Load(resultId)
		if !found {
			return true
		}
		foundCache = resultCache
		return false
	})

	if foundCache == nil {
		return nil, false
	}

	return foundCache, true
}

func (m *Manager) findResultCacheByIdWithContext(ctx context.Context, resultId string) (*QueryResultCache, bool) {
	sessionId := util.GetContextSessionId(ctx)
	queryId := util.GetContextQueryId(ctx)
	if sessionId != "" {
		if resultCache, ok := m.findResultCacheInSession(sessionId, queryId, resultId); ok {
			return resultCache, true
		}
	}
	return m.findResultCacheById(resultId)
}

func (m *Manager) GetSessionIdByQueryId(queryId string) string {
	if queryId == "" {
		return ""
	}
	var sessionId string
	m.sessionQueryResultCache.Range(func(_ string, set *QueryResultSet) bool {
		if set.Query.Id == queryId {
			sessionId = set.Query.SessionId
			return false
		}
		return true
	})
	return sessionId
}

func (m *Manager) GetResultSessionId(resultId string) string {
	resultCache, found := m.findResultCacheById(resultId)
	if !found {
		return ""
	}
	return resultCache.Query.SessionId
}

func (m *Manager) GetQueryInfoByResultId(resultId string) (string, string) {
	resultCache, found := m.findResultCacheById(resultId)
	if !found {
		return "", ""
	}
	return resultCache.Query.SessionId, resultCache.Query.Id
}

func (m *Manager) IsCurrentQuery(sessionId string, queryId string) bool {
	if queryId == "" {
		return false
	}
	set, found := m.sessionQueryResultCache.Load(sessionId)
	if !found {
		return false
	}
	return set.Query.Id == queryId
}

func (m *Manager) buildResultUI(resultCache *QueryResultCache, queryId string) QueryResultUI {
	uiResult := resultCache.Result
	// Requirement settings must keep their concrete preview type in the result
	// payload. If they were wrapped as remote previews, the Flutter grid path
	// would not know to force the fullscreen settings form.
	if !uiResult.Preview.IsEmpty() &&
		uiResult.Preview.PreviewType != WoxPreviewTypeRemote &&
		uiResult.Preview.PreviewType != WoxPreviewTypeQueryRequirementSettings &&
		len(uiResult.Preview.PreviewData) > previewDataMaxSize {
		uiResult.Preview = WoxPreview{
			PreviewType: WoxPreviewTypeRemote,
			PreviewData: fmt.Sprintf("/preview?sessionId=%s&queryId=%s&id=%s", resultCache.Query.SessionId, queryId, uiResult.Id),
		}
	}
	resultUI := uiResult.ToUI()
	resultUI.QueryId = queryId
	return resultUI
}

// Equal scores must still produce a deterministic order because the result cache is backed by a map.
func compareQueryResultCachesForDisplay(a *QueryResultCache, b *QueryResultCache) int {
	switch {
	case a.Result.Score > b.Result.Score:
		return -1
	case a.Result.Score < b.Result.Score:
		return 1
	}

	if diff := strings.Compare(a.Result.Title, b.Result.Title); diff != 0 {
		return diff
	}
	if diff := strings.Compare(a.Result.SubTitle, b.Result.SubTitle); diff != 0 {
		return diff
	}

	pluginIdA := ""
	if a.PluginInstance != nil {
		pluginIdA = a.PluginInstance.Metadata.Id
	}
	pluginIdB := ""
	if b.PluginInstance != nil {
		pluginIdB = b.PluginInstance.Metadata.Id
	}
	if diff := strings.Compare(pluginIdA, pluginIdB); diff != 0 {
		return diff
	}

	return strings.Compare(a.Result.Id, b.Result.Id)
}

// BuildQueryResultsSnapshot builds a snapshot of query results for the given session and query id.
// Results are grouped by their group name, and both groups and results within groups are sorted by score.
func (m *Manager) BuildQueryResultsSnapshot(sessionId string, queryId string) []QueryResultUI {
	if queryId == "" {
		return []QueryResultUI{}
	}
	set, found := m.sessionQueryResultCache.Load(sessionId)
	if !found || set.Query.Id != queryId {
		return []QueryResultUI{}
	}

	var resultCaches []*QueryResultCache
	set.Results.Range(func(_ string, resultCache *QueryResultCache) bool {
		resultCaches = append(resultCaches, resultCache)
		return true
	})

	if len(resultCaches) == 0 {
		return []QueryResultUI{}
	}

	groupScores := map[string]int64{}
	for _, resultCache := range resultCaches {
		result := resultCache.Result
		if score, ok := groupScores[result.Group]; !ok || result.GroupScore > score {
			groupScores[result.Group] = result.GroupScore
		}
	}

	var groups []string
	for group := range groupScores {
		groups = append(groups, group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		scoreA := groupScores[groups[i]]
		scoreB := groupScores[groups[j]]
		if scoreA == scoreB {
			return groups[i] < groups[j]
		}
		return scoreA > scoreB
	})

	groupedResults := make(map[string][]*QueryResultCache)
	for _, result := range resultCaches {
		group := result.Result.Group
		groupedResults[group] = append(groupedResults[group], result)
	}

	for group := range groupedResults {
		groupResults := groupedResults[group]
		sort.Slice(groupResults, func(i, j int) bool {
			return compareQueryResultCachesForDisplay(groupResults[i], groupResults[j]) < 0
		})
		groupedResults[group] = groupResults
	}

	finalResults := make([]QueryResultUI, 0, len(resultCaches)+len(groups))
	for _, group := range groups {
		groupResults := groupedResults[group]
		if len(groupResults) == 0 {
			continue
		}
		if group != "" {
			finalResults = append(finalResults, QueryResultUI{
				QueryId:    queryId,
				Id:         fmt.Sprintf("group:%s:%s", queryId, group),
				Title:      group,
				SubTitle:   "",
				Icon:       common.WoxImage{},
				Preview:    WoxPreview{},
				Score:      groupScores[group],
				Group:      group,
				GroupScore: groupScores[group],
				Tails:      []QueryResultTail{},
				Actions:    []QueryResultActionUI{},
				IsGroup:    true,
			})
		}
		for _, resultCache := range groupResults {
			finalResults = append(finalResults, m.buildResultUI(resultCache, queryId))
		}
	}

	return finalResults
}

func (m *Manager) PolishResult(ctx context.Context, pluginInstance *Instance, query Query, result QueryResult) QueryResult {
	resultIconSize := m.getResultIconSizeForQuery(pluginInstance, query)

	// set default id
	if result.Id == "" {
		result.Id = uuid.NewString()
	}
	for actionIndex := range result.Actions {
		if result.Actions[actionIndex].Id == "" {
			result.Actions[actionIndex].Id = uuid.NewString()
		}
		if result.Actions[actionIndex].Icon.IsEmpty() {
			// set default action icon if not present
			result.Actions[actionIndex].Icon = common.ExecuteRunIcon
		}
		if result.Actions[actionIndex].Type == "" {
			if len(result.Actions[actionIndex].Form) > 0 || result.Actions[actionIndex].OnSubmit != nil {
				result.Actions[actionIndex].Type = QueryResultActionTypeForm
			} else {
				result.Actions[actionIndex].Type = QueryResultActionTypeExecute
			}
		}
	}
	for actionIndex := range result.Actions {
		if !result.Actions[actionIndex].Icon.IsEmpty() {
			result.Actions[actionIndex].Icon = common.ConvertIcon(ctx, result.Actions[actionIndex].Icon, pluginInstance.PluginDirectory)
		}
	}
	m.attachExternalActionCallbacks(pluginInstance, result.Actions)
	// Result icons are converted for the visual surface selected by metadata. The old
	// hard-coded list size made grid icons blurry because the UI had to scale 40px
	// raster caches into much larger cells.
	result.Icon = common.ConvertIconWithSize(ctx, result.Icon, pluginInstance.PluginDirectory, resultIconSize)
	for i := range result.Tails {
		if result.Tails[i].Type == QueryResultTailTypeImage {
			result.Tails[i].Image = common.ConvertIcon(ctx, result.Tails[i].Image, pluginInstance.PluginDirectory)
		}
	}

	// add default preview for selection query if no preview is set
	if query.Type == QueryTypeSelection && result.Preview.PreviewType == "" {
		if query.Selection.Type == selection.SelectionTypeText {
			result.Preview = WoxPreview{
				PreviewType: WoxPreviewTypeText,
				PreviewData: query.Selection.Text,
			}
		}
		if query.Selection.Type == selection.SelectionTypeFile {
			result.Preview = WoxPreview{
				// Selection-file preview is now expressed as generic list rows so
				// the same preview type can serve progress/status workflows too.
				PreviewType: WoxPreviewTypeList,
				PreviewData: m.buildSelectionFileListPreviewData(ctx, query.Selection.FilePaths),
				PreviewProperties: map[string]string{
					"i18n:selection_files_count": fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "selection_files_count_value"), len(query.Selection.FilePaths)),
				},
			}
		}
	}
	result.Preview = m.normalizeListPreviewData(ctx, pluginInstance, result.Preview)

	// translate title
	result.Title = m.translatePlugin(ctx, pluginInstance, result.Title)
	// translate subtitle
	result.SubTitle = m.translatePlugin(ctx, pluginInstance, result.SubTitle)
	// translate tail text and assign IDs if not present
	for i := range result.Tails {
		if result.Tails[i].Id == "" {
			result.Tails[i].Id = uuid.NewString()
		}
		if result.Tails[i].Type == QueryResultTailTypeText {
			result.Tails[i].Text = m.translatePlugin(ctx, pluginInstance, result.Tails[i].Text)
		}
	}
	// translate preview properties
	var previewProperties = make(map[string]string)
	for key, value := range result.Preview.PreviewProperties {
		translatedKey := m.translatePlugin(ctx, pluginInstance, key)
		previewProperties[translatedKey] = value
	}
	result.Preview.PreviewProperties = previewProperties
	// translate action names
	for actionIndex := range result.Actions {
		result.Actions[actionIndex].Name = m.translatePlugin(ctx, pluginInstance, result.Actions[actionIndex].Name)
		if result.Actions[actionIndex].Type == QueryResultActionTypeForm {
			for definitionIndex := range result.Actions[actionIndex].Form {
				if result.Actions[actionIndex].Form[definitionIndex].Value != nil {
					result.Actions[actionIndex].Form[definitionIndex].Value = result.Actions[actionIndex].Form[definitionIndex].Value.Translate(pluginInstance.API.GetTranslation)
				}
			}
		}
	}
	// translate preview data if preview type is text
	if result.Preview.PreviewType == WoxPreviewTypeText || result.Preview.PreviewType == WoxPreviewTypeMarkdown {
		result.Preview.PreviewData = m.translatePlugin(ctx, pluginInstance, result.Preview.PreviewData)
	}
	// translate group name
	result.Group = m.translatePlugin(ctx, pluginInstance, result.Group)

	// set first action as default if no default action is set
	defaultActionCount := lo.CountBy(result.Actions, func(item QueryResultAction) bool {
		return item.IsDefault
	})
	if defaultActionCount == 0 {
		if len(result.Actions) > 0 {
			result.Actions[0].IsDefault = true
			result.Actions[0].Hotkey = "Enter"
		}
	}

	//move default action to first one of the actions
	sort.Slice(result.Actions, func(i, j int) bool {
		return result.Actions[i].IsDefault
	})

	// normalize hotkeys for all actions
	for actionIndex := range result.Actions {
		var action = result.Actions[actionIndex]

		// if default action's hotkey is empty, set it as Enter
		if action.IsDefault && action.Hotkey == "" {
			result.Actions[actionIndex].Hotkey = "Enter"
		}

		// normalize hotkey for platform specific modifiers
		result.Actions[actionIndex].Hotkey = normalizeHotkeyForPlatform(result.Actions[actionIndex].Hotkey)
	}

	// if query is input and trigger keyword is global, disable preview and group.
	// Query requirement settings keep their preview even in global queries because
	// stripping it would hide the system form that can unblock the query.
	if query.IsGlobalQuery() && result.Preview.PreviewType != WoxPreviewTypeQueryRequirementSettings {
		result.Preview = WoxPreview{}
		result.Group = ""
		result.GroupScore = 0
	}

	// store preview for ui invoke later
	// because preview may contain some heavy data (E.g. image or large text),
	// we will store preview in cache and only send preview to ui when user select the result
	var originalPreview = result.Preview
	// Query requirement settings intentionally bypass remote wrapping so the UI
	// can detect the type before deciding whether grid previews are allowed.
	if !result.Preview.IsEmpty() &&
		result.Preview.PreviewType != WoxPreviewTypeRemote &&
		result.Preview.PreviewType != WoxPreviewTypeQueryRequirementSettings &&
		len(result.Preview.PreviewData) > previewDataMaxSize {
		result.Preview = WoxPreview{
			PreviewType: WoxPreviewTypeRemote,
			PreviewData: fmt.Sprintf("/preview?sessionId=%s&queryId=%s&id=%s", query.SessionId, query.Id, result.Id),
		}
	}

	ignoreAutoScore := pluginInstance.Metadata.IsSupportFeature(MetadataFeatureIgnoreAutoScore)
	if !ignoreAutoScore {
		score := m.calculateResultScore(ctx, pluginInstance.Metadata.Id, result.Title, result.SubTitle, query.RawQuery)
		if score > 0 {
			logger.Debug(ctx, fmt.Sprintf("<%s> result(%s) add score: %d", pluginInstance.GetName(ctx), result.Title, score))
			result.Score += score
		}
	}
	// check if result is favorite result
	// favorite result will not be affected by ignoreAutoScore setting, so we add score here
	isFavorite := setting.GetSettingManager().IsPinedResult(ctx, pluginInstance.Metadata.Id, result.Title, result.SubTitle)
	if isFavorite {
		favScore := int64(100000)
		logger.Debug(ctx, fmt.Sprintf("<%s> result(%s) is favorite result, add score: %d", pluginInstance.GetName(ctx), result.Title, favScore))
		result.Score += favScore

		// Add favorite icon to tails if not already present
		hasFavoriteTail := false
		for _, tail := range result.Tails {
			if tail.ContextData[favoriteTailContextDataKey] == favoriteTailContextDataValue {
				hasFavoriteTail = true
				break
			}
		}
		if !hasFavoriteTail {
			result.Tails = append(result.Tails, QueryResultTail{
				Type:         QueryResultTailTypeImage,
				Image:        common.PinIcon,
				ContextData:  common.ContextData{favoriteTailContextDataKey: favoriteTailContextDataValue}, // Use ContextData to identify favorite tail
				IsSystemTail: true,                                                                         // Mark as system tail so it will be filtered out in GetUpdatableResult
			})
		}
	}

	result.Tails = appendDevScoreTail(ctx, result.Tails, result.Score)

	// Create cache at the end
	resultCopy := result
	// Because we may have replaced preview with remote preview
	// we need to restore the original preview in the cache
	resultCopy.Preview = originalPreview
	m.storeQueryResult(ctx, pluginInstance, query, resultCopy)

	return result
}

func (m *Manager) getResultIconSizeForQuery(pluginInstance *Instance, query Query) int {
	if pluginInstance == nil {
		return common.ResultListIconSize
	}

	// Grid is the only large-icon result surface today. Keep the size pinned in core
	// for now so third-party metadata does not need a public IconSize contract.
	if _, isGridLayout, err := pluginInstance.Metadata.GetFeatureParamsForGridLayoutCommand(query.Command); err == nil && isGridLayout {
		return common.ResultGridIconSize
	}

	return common.ResultListIconSize
}

func (m *Manager) PolishUpdatableResult(ctx context.Context, pluginInstance *Instance, result UpdatableResult) UpdatableResult {
	// Get result cache to update it
	resultCache, found := m.findResultCacheByIdWithContext(ctx, result.Id)
	if !found {
		return result // Result not in cache, just return as-is
	}

	// Polish actions if they are being updated
	if result.Actions != nil {
		actions := *result.Actions

		// Set default action id and icon if not present
		for actionIndex := range actions {
			if actions[actionIndex].Id == "" {
				actions[actionIndex].Id = uuid.NewString()
			}
			if actions[actionIndex].Icon.IsEmpty() {
				actions[actionIndex].Icon = common.ExecuteRunIcon
			} else {
				actions[actionIndex].Icon = common.ConvertIcon(ctx, actions[actionIndex].Icon, pluginInstance.PluginDirectory)
			}
			if actions[actionIndex].Type == "" {
				if len(actions[actionIndex].Form) > 0 || actions[actionIndex].OnSubmit != nil {
					actions[actionIndex].Type = QueryResultActionTypeForm
				} else {
					actions[actionIndex].Type = QueryResultActionTypeExecute
				}
			}
		}
		m.attachExternalActionCallbacks(pluginInstance, actions)

		// Set first action as default if no default action is set
		defaultActionCount := lo.CountBy(actions, func(item QueryResultAction) bool {
			return item.IsDefault
		})
		if defaultActionCount == 0 && len(actions) > 0 {
			actions[0].IsDefault = true
			actions[0].Hotkey = "Enter"
		}

		// If default action's hotkey is empty, set it as Enter
		for actionIndex := range actions {
			if actions[actionIndex].IsDefault && actions[actionIndex].Hotkey == "" {
				actions[actionIndex].Hotkey = "Enter"
			}
			// Normalize hotkey for platform specific modifiers
			actions[actionIndex].Hotkey = normalizeHotkeyForPlatform(actions[actionIndex].Hotkey)
		}

		// Move default action to first position
		sort.Slice(actions, func(i, j int) bool {
			return actions[i].IsDefault
		})

		// Add system actions (like pin/unpin)
		// System actions are added after user actions
		systemActions := m.getDefaultActions(ctx, pluginInstance, resultCache.Query, resultCache.Result.Title, resultCache.Result.SubTitle)
		actions = append(actions, systemActions...)

		// Translate action names
		for actionIndex := range actions {
			actions[actionIndex].Name = m.translatePlugin(ctx, pluginInstance, actions[actionIndex].Name)
			if actions[actionIndex].Type == QueryResultActionTypeForm {
				for definitionIndex := range actions[actionIndex].Form {
					if actions[actionIndex].Form[definitionIndex].Value != nil {
						actions[actionIndex].Form[definitionIndex].Value = actions[actionIndex].Form[definitionIndex].Value.Translate(pluginInstance.API.GetTranslation)
					}
				}
			}
		}

		result.Actions = &actions

		// Update cache: merge new actions with cached actions to preserve callbacks
		// When updating actions, we need to preserve the Action callbacks from cache
		// because callbacks cannot be serialized and may be nil in the updated actions
		for i := range actions {
			// Find matching action in cache by ID
			var cachedAction *QueryResultAction
			for j := range resultCache.Result.Actions {
				if resultCache.Result.Actions[j].Id == actions[i].Id {
					cachedAction = &resultCache.Result.Actions[j]
					break
				}
			}

			// If action callback is nil in the new action but exists in cache, preserve it
			if cachedAction != nil {
				if actions[i].Action == nil && cachedAction.Action != nil {
					actions[i].Action = cachedAction.Action
				}
				if actions[i].Type == QueryResultActionTypeForm && actions[i].OnSubmit == nil && cachedAction.OnSubmit != nil {
					actions[i].OnSubmit = cachedAction.OnSubmit
				}
			}
		}

		// Update cache with merged actions
		resultCache.Result.Actions = actions
	}

	// Translate title if present
	if result.Title != nil {
		translated := m.translatePlugin(ctx, pluginInstance, *result.Title)
		result.Title = &translated
		resultCache.Result.Title = translated
	}

	// Translate subtitle if present
	if result.SubTitle != nil {
		translated := m.translatePlugin(ctx, pluginInstance, *result.SubTitle)
		result.SubTitle = &translated
		resultCache.Result.SubTitle = translated
	}

	// Translate tails if present
	if result.Tails != nil {
		tails := *result.Tails
		for i := range tails {
			// Assign ID if not present
			if tails[i].Id == "" {
				tails[i].Id = uuid.NewString()
			}
			if tails[i].Type == QueryResultTailTypeText {
				tails[i].Text = m.translatePlugin(ctx, pluginInstance, tails[i].Text)
			}
			if tails[i].Type == QueryResultTailTypeImage {
				tails[i].Image = common.ConvertIcon(ctx, tails[i].Image, pluginInstance.PluginDirectory)
			}
		}

		// Add favorite icon to tails if this is a favorite result
		isFavorite := setting.GetSettingManager().IsPinedResult(ctx, pluginInstance.Metadata.Id, resultCache.Result.Title, resultCache.Result.SubTitle)
		if isFavorite {
			// Check if favorite tail already exists
			hasFavoriteTail := false
			for _, tail := range tails {
				if tail.ContextData[favoriteTailContextDataKey] == favoriteTailContextDataValue {
					hasFavoriteTail = true
					break
				}
			}
			if !hasFavoriteTail {
				tails = append(tails, QueryResultTail{
					Type:         QueryResultTailTypeImage,
					Image:        common.PinIcon,
					ContextData:  common.ContextData{favoriteTailContextDataKey: favoriteTailContextDataValue}, // Use ContextData to identify favorite tail
					IsSystemTail: true,                                                                         // Mark as system tail so it will be filtered out in GetUpdatableResult
				})
			}
		}

		tails = appendDevScoreTail(ctx, tails, resultCache.Result.Score)

		result.Tails = &tails
		resultCache.Result.Tails = tails
	}

	// Translate preview properties if present
	if result.Preview != nil {
		// Updated previews must use the same list normalization as initial
		// query results. Long-running actions commonly update list rows in place,
		// so icon conversion and row text translation cannot live only in the
		// first result-processing path.
		preview := m.normalizeListPreviewData(ctx, pluginInstance, *result.Preview)
		var previewProperties = make(map[string]string)
		for key, value := range preview.PreviewProperties {
			translatedKey := m.translatePlugin(ctx, pluginInstance, key)
			previewProperties[translatedKey] = value
		}
		preview.PreviewProperties = previewProperties
		result.Preview = &preview
		resultCache.Result.Preview = preview
	}

	// Update icon in cache if present
	if result.Icon != nil {
		// Updated result icons must keep the same surface-aware size as initial results;
		// otherwise a grid plugin can become blurry after sending an icon update.
		resultIconSize := m.getResultIconSizeForQuery(pluginInstance, resultCache.Query)
		convertedIcon := common.ConvertIconWithSize(ctx, *result.Icon, pluginInstance.PluginDirectory, resultIconSize)
		result.Icon = &convertedIcon
		resultCache.Result.Icon = *result.Icon
	}

	return result
}

// For external plugins (Node.js/Python), create proxy action callbacks
// These callbacks will invoke the host's action method, which will then
// call the actual cached callback in the plugin host
func (m *Manager) attachExternalActionCallbacks(pluginInstance *Instance, actions []QueryResultAction) {
	if proxyCreator, ok := pluginInstance.Plugin.(ActionProxyCreator); ok {
		for actionIndex := range actions {
			if actions[actionIndex].Type == QueryResultActionTypeExecute && actions[actionIndex].Action == nil {
				actions[actionIndex].Action = proxyCreator.CreateActionProxy(actions[actionIndex].Id)
			}
		}
	}

	if proxyCreator, ok := pluginInstance.Plugin.(FormActionProxyCreator); ok {
		for actionIndex := range actions {
			if actions[actionIndex].Type == QueryResultActionTypeForm && actions[actionIndex].OnSubmit == nil {
				actions[actionIndex].OnSubmit = proxyCreator.CreateFormActionProxy(actions[actionIndex].Id)
			}
		}
	}
}

func (m *Manager) GetUpdatableResult(ctx context.Context, resultId string) *UpdatableResult {
	// Try to find the result in the cache
	resultCache, found := m.findResultCacheByIdWithContext(ctx, resultId)
	if !found {
		return nil // Result not found (no longer visible)
	}

	// Construct UpdatableResult from cache
	title := resultCache.Result.Title
	subTitle := resultCache.Result.SubTitle
	icon := resultCache.Result.Icon
	preview := resultCache.Result.Preview

	// Make a copy of tails to avoid modifying cache when developer appends to it
	// Filter out system tails (they will be added back in polish)
	tails := []QueryResultTail{}
	for _, tail := range resultCache.Result.Tails {
		if !tail.IsSystemTail {
			tails = append(tails, tail)
		}
	}

	// Make a copy of actions to avoid modifying cache when developer modifies it
	// Filter out system actions (they will be added back in polish)
	actions := []QueryResultAction{}
	for _, action := range resultCache.Result.Actions {
		if !action.IsSystemAction {
			actions = append(actions, action)
		}
	}

	return &UpdatableResult{
		Id:       resultId,
		Title:    &title,
		SubTitle: &subTitle,
		Icon:     &icon,
		Preview:  &preview,
		Tails:    &tails,
		Actions:  &actions,
	}
}

func (m *Manager) Query(ctx context.Context, query Query) (resultsChan chan []QueryResultUI, fallbackReadyChan chan bool, doneChan chan bool) {
	resultsChan = make(chan []QueryResultUI, 10)
	fallbackReadyChan = make(chan bool, 1)
	doneChan = make(chan bool, 1)

	m.startSessionQueryCache(query)

	tracker := newQueryTracker(fallbackReadyChan, doneChan)

	for _, pluginInstance := range m.instances {
		if !m.canOperateQuery(ctx, pluginInstance, query) {
			continue
		}

		// Debounced plugins are treated as late work: they still participate in the
		// final query completion, but they do not delay fallback.
		blocksFallback := !pluginInstance.Metadata.IsSupportFeature(MetadataFeatureDebounce)
		if pluginInstance.Metadata.IsSupportFeature(MetadataFeatureDebounce) {
			debounceParams, err := pluginInstance.Metadata.GetFeatureParamsForDebounce()
			if err == nil {
				tracker.start(blocksFallback)
				logger.Debug(ctx, fmt.Sprintf("[%s] debounce query, will execute in %d ms", pluginInstance.GetName(ctx), debounceParams.IntervalMs))
				if v, ok := m.debounceQueryTimer.Load(pluginInstance.Metadata.Id); ok {
					if v.timer.Stop() {
						v.onStop()
					}
				}

				timer := time.AfterFunc(time.Duration(debounceParams.IntervalMs)*time.Millisecond, func() {
					m.queryParallel(ctx, pluginInstance, query, resultsChan, tracker, blocksFallback)
				})
				onStop := func() {
					// A newer query replaced this debounced run before it started. Mark this
					// plugin as finished for the current query lifecycle so counters do not hang.
					logger.Debug(ctx, fmt.Sprintf("[%s] previous debounced query cancelled", pluginInstance.GetName(ctx)))
					tracker.finish(blocksFallback)
				}
				m.debounceQueryTimer.Store(pluginInstance.Metadata.Id, &debounceTimer{
					timer:  timer,
					onStop: onStop,
				})
				continue
			} else {
				logger.Error(ctx, fmt.Sprintf("[%s] %s, query directlly", pluginInstance.GetName(ctx), err))
			}
		}

		tracker.start(blocksFallback)
		m.queryParallel(ctx, pluginInstance, query, resultsChan, tracker, blocksFallback)
	}

	// Queries with no runnable plugins should still notify both phases immediately.
	tracker.notifyIfEmpty()

	return
}

func (m *Manager) QuerySilent(ctx context.Context, query Query) bool {
	var startTimestamp = util.GetSystemTimestamp()
	var results []QueryResultUI
	resultChan, _, doneChan := m.Query(ctx, query)
	for {
		select {
		case r := <-resultChan:
			results = append(results, r...)
		case <-doneChan:
			logger.Info(ctx, fmt.Sprintf("silent query done, total results: %d, cost %d ms", len(results), util.GetSystemTimestamp()-startTimestamp))

			// execute default action if only one result
			woxIcon, _ := common.WoxIcon.ToImage()
			if len(results) == 1 {
				result := results[0]
				for _, action := range result.Actions {
					if action.IsDefault {
						actionCtx := util.WithQueryIdContext(util.WithSessionContext(ctx, query.SessionId), query.Id)
						executeErr := m.ExecuteAction(actionCtx, query.SessionId, query.Id, result.Id, action.Id)
						if executeErr != nil {
							logger.Error(ctx, fmt.Sprintf("silent query execute failed: %s", executeErr.Error()))
							notifier.Notify(woxIcon, fmt.Sprintf("Silent query execute failed: %s", executeErr.Error()))
							return false
						}

						return true
					}
				}
			} else {
				notifier.Notify(woxIcon, fmt.Sprintf("Silent query failed, there shouldbe only one result, but got %d", len(results)))
			}

			return false
		case <-time.After(time.Minute):
			logger.Error(ctx, "silent query timeout")
			return false
		}
	}
}

func (m *Manager) QueryFallback(ctx context.Context, query Query, queryPlugin *Instance) (results []QueryResultUI) {
	var queryResults []QueryResult
	if query.IsGlobalQuery() {
		for _, pluginInstance := range m.instances {
			if v, ok := pluginInstance.Plugin.(FallbackSearcher); ok {
				fallbackResults := v.QueryFallback(ctx, query)
				for _, fallbackResult := range fallbackResults {
					polishedFallbackResult := m.PolishResult(ctx, pluginInstance, query, fallbackResult)
					queryResults = append(queryResults, polishedFallbackResult)
				}
				continue
			}
		}
	} else {
		if query.Command != "" {
			return results
		}

		// search query commands
		commands := lo.Filter(queryPlugin.GetQueryCommands(), func(item MetadataCommand, index int) bool {
			return strings.Contains(item.Command, query.Search) || query.Search == ""
		})
		queryResults = lo.Map(commands, func(item MetadataCommand, index int) QueryResult {
			return QueryResult{
				Title:    item.Command,
				SubTitle: string(item.Description),
				Icon:     common.ParseWoxImageOrDefault(queryPlugin.Metadata.Icon, common.NewWoxImageEmoji("🔍")),
				Actions: []QueryResultAction{
					{
						Name:                   "Execute",
						PreventHideAfterAction: true,
						Action: func(ctx context.Context, actionContext ActionContext) {
							m.ui.ChangeQuery(ctx, common.PlainQuery{
								QueryType: QueryTypeInput,
								QueryText: fmt.Sprintf("%s %s ", query.TriggerKeyword, item.Command),
							})
						},
					},
				},
			}
		})
		for i := range queryResults {
			queryResults[i] = m.PolishResult(ctx, queryPlugin, query, queryResults[i])
		}
	}

	queryResultsUI := lo.Map(queryResults, func(item QueryResult, index int) QueryResultUI {
		return item.ToUI()
	})
	results = append(results, queryResultsUI...)
	return results
}

func newQueryTracker(fallbackReady chan bool, done chan bool) *queryTracker {
	return &queryTracker{
		remaining:         &atomic.Int32{},
		fallbackRemaining: &atomic.Int32{},
		fallbackReady:     fallbackReady,
		done:              done,
	}
}

func (t *queryTracker) start(blocksFallback bool) {
	t.remaining.Add(1)
	if blocksFallback {
		t.fallbackRemaining.Add(1)
	}
}

func (t *queryTracker) finish(blocksFallback bool) {
	// fallbackReady fires once the last fallback-blocking plugin completes.
	if blocksFallback && t.fallbackRemaining.Add(-1) == 0 {
		t.fallbackReady <- true
	}
	// done fires only after every plugin completes, including debounced ones.
	if t.remaining.Add(-1) == 0 {
		t.done <- true
	}
}

func (t *queryTracker) notifyIfEmpty() {
	// When nothing was scheduled, both phases are already complete.
	if t.fallbackRemaining.Load() == 0 {
		t.fallbackReady <- true
	}
	if t.remaining.Load() == 0 {
		t.done <- true
	}
}

func (m *Manager) queryParallel(ctx context.Context, pluginInstance *Instance, query Query, results chan []QueryResultUI, tracker *queryTracker, blocksFallback bool) {
	util.Go(ctx, fmt.Sprintf("[%s] parallel query", pluginInstance.GetName(ctx)), func() {
		queryResults := m.queryForPlugin(ctx, pluginInstance, query)
		results <- lo.Map(queryResults, func(item QueryResult, index int) QueryResultUI {
			return item.ToUI()
		})
		tracker.finish(blocksFallback)
	}, func() {
		tracker.finish(blocksFallback)
	})
}

func (m *Manager) translatePlugin(ctx context.Context, pluginInstance *Instance, key string) string {
	if !strings.HasPrefix(key, "i18n:") {
		return key
	}

	return pluginInstance.Metadata.translate(ctx, common.I18nString(key))
}

func (m *Manager) GetUI() common.UI {
	return m.ui
}

func (m *Manager) updatePluginQueryLatency(pluginId string, costMs float64) {
	ewma, ok := m.pluginQueryLatency.Load(pluginId)
	if !ok {
		ewma = util.NewEWMA(0.3)
		m.pluginQueryLatency.Store(pluginId, ewma)
	}
	ewma.Add(costMs)
}

func (m *Manager) GetQueryFirstFlushDelayMs(query Query) int64 {
	const minDelay int64 = 11 // most plugins can return results within 10ms, so we set min delay to 11ms to avoid unnecessary flush
	const maxDelay int64 = 54 //
	const defaultDelay int64 = 32

	var totalAvg float64
	var count int

	for _, pluginInstance := range m.instances {
		if !m.canOperateQuery(util.NewTraceContext(), pluginInstance, query) {
			continue
		}
		if ewma, ok := m.pluginQueryLatency.Load(pluginInstance.Metadata.Id); ok {
			if avg, hasValue := ewma.Value(); hasValue {
				totalAvg += avg
				count++
			}
		}
	}

	if count == 0 {
		return defaultDelay
	}

	avgCost := totalAvg / float64(count)
	firstDelay := int64(0.8 * avgCost)

	if firstDelay < minDelay {
		firstDelay = minDelay
	}
	if firstDelay > maxDelay {
		firstDelay = maxDelay
	}

	return firstDelay
}

func (m *Manager) NewQuery(ctx context.Context, plainQuery common.PlainQuery) (Query, *Instance, error) {
	if plainQuery.QueryType == QueryTypeInput {
		newQuery := plainQuery.QueryText
		woxSetting := setting.GetSettingManager().GetWoxSetting(ctx)
		if len(woxSetting.QueryShortcuts.Get()) > 0 {
			originQuery := plainQuery.QueryText
			expandedQuery := m.expandQueryShortcut(ctx, plainQuery.QueryText, woxSetting.QueryShortcuts.Get())
			if originQuery != expandedQuery {
				logger.Info(ctx, fmt.Sprintf("expand query shortcut: %s -> %s", originQuery, expandedQuery))
				newQuery = expandedQuery
			}
		}
		query, instance := newQueryInputWithPlugins(newQuery, GetPluginManager().GetPluginInstances())
		query.Id = plainQuery.QueryId
		query.SessionId = util.GetContextSessionId(ctx)
		activeWindowSnapshot := m.GetUI().GetActiveWindowSnapshot(ctx)
		query.Env.ActiveWindowTitle = activeWindowSnapshot.Name
		query.Env.ActiveWindowPid = activeWindowSnapshot.Pid
		query.Env.ActiveWindowIcon = activeWindowSnapshot.Icon
		query.Env.ActiveWindowIsOpenSaveDialog = activeWindowSnapshot.IsOpenSaveDialog
		query.Env.ActiveBrowserUrl = m.getActiveBrowserUrl(ctx)
		return query, instance, nil
	}

	if plainQuery.QueryType == QueryTypeSelection {
		query := Query{
			Id:        plainQuery.QueryId,
			Type:      QueryTypeSelection,
			RawQuery:  plainQuery.QueryText,
			Search:    plainQuery.QueryText,
			Selection: plainQuery.QuerySelection,
		}
		query.SessionId = util.GetContextSessionId(ctx)
		activeWindowSnapshot := m.GetUI().GetActiveWindowSnapshot(ctx)
		query.Env.ActiveWindowTitle = activeWindowSnapshot.Name
		query.Env.ActiveWindowPid = activeWindowSnapshot.Pid
		query.Env.ActiveWindowIcon = activeWindowSnapshot.Icon
		query.Env.ActiveWindowIsOpenSaveDialog = activeWindowSnapshot.IsOpenSaveDialog
		query.Env.ActiveBrowserUrl = m.getActiveBrowserUrl(ctx)
		return query, nil, nil
	}

	return Query{}, nil, errors.New("invalid query type")
}

func (m *Manager) getActiveBrowserUrl(ctx context.Context) string {
	activeWindowSnapshot := m.GetUI().GetActiveWindowSnapshot(ctx)
	isGoogleChrome := strings.ToLower(activeWindowSnapshot.Name) == "google chrome"
	if !isGoogleChrome {
		return ""
	}

	return m.activeBrowserUrl
}

func (m *Manager) getActiveFileExplorerPath(ctx context.Context) string {
	// Supported on Windows (File Explorer) and macOS (Finder)
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		return ""
	}

	// Use platform-specific implementation via util/window
	return window.GetActiveFileExplorerPath()
}

func (m *Manager) expandQueryShortcut(ctx context.Context, query string, queryShorts []setting.QueryShortcut) (newQuery string) {
	newQuery = query

	//sort query shorts by shortcut length, we will expand the longest shortcut first
	slices.SortFunc(queryShorts, func(i, j setting.QueryShortcut) int {
		return len(j.Shortcut) - len(i.Shortcut)
	})

	for _, shortcut := range queryShorts {
		if shortcut.Disabled {
			continue
		}

		// Query shortcuts are command-style aliases for the first query token. Plain
		// prefix matching made short aliases such as "th" rewrite normal queries like
		// "theme xx", so the shortcut must end at the query boundary while still
		// supporting "th args".
		if query == shortcut.Shortcut || strings.HasPrefix(query, shortcut.Shortcut+" ") {
			if !shortcut.HasPlaceholder() {
				newQuery = strings.Replace(query, shortcut.Shortcut, shortcut.Query, 1)
				break
			} else {
				queryWithoutShortcut := strings.Replace(query, shortcut.Shortcut, "", 1)
				queryWithoutShortcut = strings.TrimLeft(queryWithoutShortcut, " ")
				parameters := strings.Split(queryWithoutShortcut, " ")
				placeholderCount := shortcut.PlaceholderCount()
				var paramsCount = 0

				var params []any
				var nonPrams string
				for _, param := range parameters {
					if paramsCount < placeholderCount {
						paramsCount++
						params = append(params, param)
					} else {
						nonPrams += " " + param
					}
				}
				newQuery = stringFormatter.Format(shortcut.Query, params...) + nonPrams
				break
			}
		}
	}

	return newQuery
}

func (m *Manager) ExecuteAction(ctx context.Context, sessionId string, queryId string, resultId string, actionId string) error {
	resultCache, found := m.findResultCacheInSession(sessionId, queryId, resultId)
	if !found {
		return fmt.Errorf("result cache not found for result id (execute action): %s", resultId)
	}

	// Find the action in cache
	var actionCache *QueryResultAction
	for i := range resultCache.Result.Actions {
		if resultCache.Result.Actions[i].Id == actionId {
			actionCache = &resultCache.Result.Actions[i]
			break
		}
	}
	if actionCache == nil {
		return fmt.Errorf("action not found for result id: %s, action id: %s", resultId, actionId)
	}

	// Check if action callback is nil
	if actionCache.Action == nil {
		return fmt.Errorf("action callback is nil for result id: %s, action id: %s", resultId, actionId)
	}

	meta := resultCache.PluginInstance.Metadata
	analytics.TrackActionExecuted(ctx, meta.Id, resultCache.PluginInstance.GetName(ctx))

	actionCtx := util.WithQueryIdContext(util.WithSessionContext(ctx, resultCache.Query.SessionId), resultCache.Query.Id)
	actionCache.Action(actionCtx, ActionContext{
		ResultId:       resultId,
		ResultActionId: actionId,
		ContextData:    actionCache.ContextData,
	})

	util.Go(actionCtx, fmt.Sprintf("[%s] post execute action", resultCache.PluginInstance.GetName(actionCtx)), func() {
		m.postExecuteAction(actionCtx, resultCache, actionCache.ContextData)
	})

	return nil
}

func (m *Manager) SubmitFormAction(ctx context.Context, sessionId string, queryId string, resultId string, actionId string, values map[string]string) error {
	resultCache, found := m.findResultCacheInSession(sessionId, queryId, resultId)
	if !found {
		return fmt.Errorf("result cache not found for result id (submit form action): %s", resultId)
	}

	var actionCache *QueryResultAction
	for i := range resultCache.Result.Actions {
		if resultCache.Result.Actions[i].Id == actionId && resultCache.Result.Actions[i].Type == QueryResultActionTypeForm {
			actionCache = &resultCache.Result.Actions[i]
			break
		}
	}
	if actionCache == nil {
		return fmt.Errorf("form action not found for result id: %s, action id: %s", resultId, actionId)
	}

	if actionCache.OnSubmit == nil {
		return fmt.Errorf("form action callback is nil for result id: %s, action id: %s", resultId, actionId)
	}

	meta := resultCache.PluginInstance.Metadata
	analytics.TrackActionExecuted(ctx, meta.Id, resultCache.PluginInstance.GetName(ctx))

	actionCtx := util.WithQueryIdContext(util.WithSessionContext(ctx, resultCache.Query.SessionId), resultCache.Query.Id)
	actionCache.OnSubmit(actionCtx, FormActionContext{
		ActionContext: ActionContext{
			ResultId:       resultId,
			ResultActionId: actionId,
			ContextData:    actionCache.ContextData,
		},
		Values: values,
	})

	util.Go(actionCtx, fmt.Sprintf("[%s] post execute action", resultCache.PluginInstance.GetName(actionCtx)), func() {
		m.postExecuteAction(actionCtx, resultCache, actionCache.ContextData)
	})

	return nil
}

func (m *Manager) postExecuteAction(ctx context.Context, resultCache *QueryResultCache, contextData map[string]string) {
	// Add actioned result for statistics
	meta := resultCache.PluginInstance.Metadata
	setting.GetSettingManager().AddActionedResult(ctx, meta.Id, resultCache.Result.Title, resultCache.Result.SubTitle, resultCache.Query.RawQuery)

	// Add to MRU if plugin supports it
	if meta.IsSupportFeature(MetadataFeatureMRU) {
		mruItem := setting.MRUItem{
			PluginID:    meta.Id,
			Title:       resultCache.Result.Title,
			SubTitle:    resultCache.Result.SubTitle,
			Icon:        resultCache.Result.Icon,
			ContextData: contextData,
		}

		// Decide MRU identity hash based on plugin metadata feature params
		hashTitle := mruItem.Title
		hashSubTitle := mruItem.SubTitle
		if params, err := meta.GetFeatureParamsForMRU(); err == nil {
			switch params.HashBy {
			case "rawquery":
				if resultCache.Query.RawQuery != "" {
					hashTitle = resultCache.Query.RawQuery
					hashSubTitle = ""
				}
			case "search":
				if resultCache.Query.Search != "" {
					hashTitle = resultCache.Query.Search
					hashSubTitle = ""
				}
			default:
				// "title" or unknown: keep default Title/SubTitle based hash
			}
		}
		mruItem.Hash = string(setting.NewResultHash(meta.Id, hashTitle, hashSubTitle))
		if err := setting.GetSettingManager().AddMRUItem(ctx, mruItem); err != nil {
			util.GetLogger().Error(ctx, fmt.Sprintf("failed to add MRU item: %s", err.Error()))
		}
	}

	// Add to query history only if query is not empty (skip empty queries like MRU)
	if resultCache.Query.RawQuery != "" {
		plainQuery := common.PlainQuery{
			QueryType: resultCache.Query.Type,
			QueryText: resultCache.Query.RawQuery,
		}
		setting.GetSettingManager().AddQueryHistory(ctx, plainQuery)
	}
}

func (m *Manager) GetResultPreview(ctx context.Context, sessionId string, queryId string, resultId string) (WoxPreview, error) {
	resultCache, found := m.findResultCacheInSession(sessionId, queryId, resultId)
	if !found {
		return WoxPreview{}, fmt.Errorf("result cache not found for result id (get preview): %s", resultId)
	}

	// if preview text is too long, ellipsis it, otherwise UI maybe freeze when render
	preview := resultCache.Result.Preview
	if preview.PreviewType == WoxPreviewTypeText {
		preview.PreviewData = util.EllipsisMiddle(preview.PreviewData, 2000)
		// translate preview data if preview type is text
		preview.PreviewData = m.translatePlugin(ctx, resultCache.PluginInstance, preview.PreviewData)
	}

	return preview, nil
}

func (m *Manager) ReplaceQueryVariable(ctx context.Context, query string) string {
	if strings.Contains(query, QueryVariableSelectedText) {
		selected, selectedErr := selection.GetSelected(ctx)
		if selectedErr != nil {
			logger.Error(ctx, fmt.Sprintf("failed to get selected text: %s", selectedErr.Error()))
		} else {
			if selected.Type == selection.SelectionTypeText {
				query = strings.ReplaceAll(query, QueryVariableSelectedText, selected.Text)
			} else {
				logger.Error(ctx, fmt.Sprintf("selected data is not text, type: %s", selected.Type))
			}
		}
	}

	if strings.Contains(query, QueryVariableActiveBrowserUrl) {
		activeBrowserUrl := m.activeBrowserUrl
		query = strings.ReplaceAll(query, QueryVariableActiveBrowserUrl, activeBrowserUrl)
	}

	// Replace file explorer path variable if present
	if strings.Contains(query, QueryVariableFileExplorerPath) {
		startTime := time.Now()
		explorerPath := m.getActiveFileExplorerPath(ctx)
		query = strings.ReplaceAll(query, QueryVariableFileExplorerPath, explorerPath)
		logger.Debug(ctx, fmt.Sprintf("replaced file explorer path variable in %d ms", time.Since(startTime).Milliseconds()))
	}

	return query
}

func (m *Manager) IsHostStarted(ctx context.Context, runtime Runtime) bool {
	if runtime == PLUGIN_RUNTIME_GO {
		return true
	}

	for _, host := range AllHosts {
		if host.GetRuntime(ctx) == runtime {
			return host.IsStarted(ctx)
		}
	}

	return false
}

func (m *Manager) RuntimeStatusForRuntime(ctx context.Context, runtime Runtime) (RuntimeHostStatus, bool) {
	pluginHost, exist := lo.Find(AllHosts, func(item Host) bool {
		return strings.EqualFold(string(item.GetRuntime(ctx)), string(runtime))
	})
	if !exist {
		return RuntimeHostStatus{}, false
	}

	// Feature: callers that present install/load failures can reuse the same
	// structured runtime diagnosis as /runtime/status instead of parsing wrapped
	// startup errors that are meant for logs.
	return pluginHost.RuntimeStatus(ctx), true
}

// EnsureHostStarted makes runtime startup an explicit, reusable preflight for
// install and load paths that need a live plugin host before mutating plugin files.
func (m *Manager) EnsureHostStarted(ctx context.Context, runtime Runtime) error {
	if runtime == PLUGIN_RUNTIME_GO || runtime == PLUGIN_RUNTIME_SCRIPT {
		return nil
	}

	pluginHost, exist := lo.Find(AllHosts, func(item Host) bool {
		return strings.EqualFold(string(item.GetRuntime(ctx)), string(runtime))
	})
	if !exist {
		return fmt.Errorf("unsupported runtime: %s", runtime)
	}

	if pluginHost.IsStarted(ctx) {
		return nil
	}

	// Bug fix: install flows previously stopped at a generic "runtime is not started"
	// check, which blocked recovery after users corrected a Node.js/Python path.
	// Starting the host here preserves the explicit preflight while returning the
	// concrete startup or websocket connection error from the host layer.
	if err := pluginHost.Start(ctx); err != nil {
		return fmt.Errorf("failed to start host for runtime %s: %w", runtime, err)
	}

	return nil
}

func (m *Manager) IsTriggerKeywordAIChat(ctx context.Context, triggerKeyword string) bool {
	aiChatPluginInstance := m.GetAIChatPluginInstance(ctx)
	if aiChatPluginInstance == nil {
		return false
	}

	return lo.Contains(aiChatPluginInstance.GetTriggerKeywords(), triggerKeyword)
}

func (m *Manager) GetAIChatPluginInstance(ctx context.Context) *Instance {
	aiChatPlugin := m.GetPluginInstances()
	aiChatPluginInstance, exist := lo.Find(aiChatPlugin, func(item *Instance) bool {
		return item.Metadata.Id == "a9cfd85a-6e53-415c-9d44-68777aa6323d"
	})
	if exist {
		return aiChatPluginInstance
	}

	return nil
}

func (m *Manager) GetAIChatPluginChater(ctx context.Context) common.AIChater {
	aiChatPluginInstance := m.GetAIChatPluginInstance(ctx)
	if aiChatPluginInstance == nil {
		return nil
	}

	chater, ok := aiChatPluginInstance.Plugin.(common.AIChater)
	if ok {
		return chater
	}

	return nil
}

func (m *Manager) GetAIProvider(ctx context.Context, provider common.ProviderName, alias string) (ai.Provider, error) {
	key := string(provider)
	if alias != "" {
		key = fmt.Sprintf("%s_%s", provider, alias)
	}
	if v, exist := m.aiProviders.Load(key); exist {
		return v, nil
	}

	//check if provider has setting
	aiProviderSettings := setting.GetSettingManager().GetWoxSetting(ctx).AIProviders.Get()
	providerSetting, providerSettingExist := lo.Find(aiProviderSettings, func(item setting.AIProvider) bool {
		return item.Name == provider && item.Alias == alias
	})
	if !providerSettingExist {
		return nil, fmt.Errorf("ai provider setting not found: %s (alias=%s)", provider, alias)
	}

	newProvider, newProviderErr := ai.NewProvider(ctx, providerSetting)
	if newProviderErr != nil {
		return nil, newProviderErr
	}
	m.aiProviders.Store(key, newProvider)
	return newProvider, nil
}

func (m *Manager) ExecutePluginDeeplink(ctx context.Context, pluginId string, arguments map[string]string) {
	pluginInstance, exist := lo.Find(m.instances, func(item *Instance) bool {
		return item.Metadata.Id == pluginId
	})
	if !exist {
		logger.Error(ctx, fmt.Sprintf("plugin not found: %s", pluginId))
		return
	}

	logger.Info(ctx, fmt.Sprintf("execute deeplink for plugin: %s, callbacks: %d", pluginInstance.GetName(ctx), len(pluginInstance.DeepLinkCallbacks)))

	for _, callback := range pluginInstance.DeepLinkCallbacks {
		util.Go(ctx, fmt.Sprintf("[%s] execute deeplink callback", pluginInstance.GetName(ctx)), func() {
			callback(ctx, arguments)
		})
	}
}

func (m *Manager) QueryMRU(ctx context.Context, sessionId string, queryId string) []QueryResultUI {
	query := Query{
		Id:             queryId,
		SessionId:      sessionId,
		Type:           QueryTypeInput,
		TriggerKeyword: "mru",
	}
	m.startSessionQueryCache(query)

	mruItems, err := setting.GetSettingManager().GetMRUItems(ctx, 10)
	if err != nil {
		util.GetLogger().Error(ctx, fmt.Sprintf("failed to get MRU items: %s", err.Error()))
		return []QueryResultUI{}
	}

	// Deduplicate restored MRU results by (pluginId, title, subTitle, contextData)
	seen := make(map[string]bool)

	var results []QueryResultUI
	for _, item := range mruItems {
		util.GetLogger().Debug(ctx, fmt.Sprintf("start to restore mru item: %s", item.Title))
		pluginInstance := m.getPluginInstance(item.PluginID)
		if pluginInstance == nil {
			util.GetLogger().Debug(ctx, fmt.Sprintf("plugin not found, skip restore mru item: %s", item.Title))
			continue
		}
		if !pluginInstance.Metadata.IsSupportFeature(MetadataFeatureMRU) {
			util.GetLogger().Debug(ctx, fmt.Sprintf("plugin does not support mru, skip restore mru item: %s", item.Title))
			continue
		}

		if restored := m.restoreFromMRU(ctx, pluginInstance, item); restored != nil {
			util.GetLogger().Debug(ctx, fmt.Sprintf("mru item restored: %s", item.Title))

			// Build a stable dedupe key using restored values, which are language-independent for Go plugins
			key := fmt.Sprintf("%s|%s|%s|%s", item.PluginID, restored.Title, restored.SubTitle, serializeContextData(item.ContextData))
			if seen[key] {
				util.GetLogger().Debug(ctx, fmt.Sprintf("duplicate mru item, skip restore mru item: %s", item.Title))
				continue
			}
			seen[key] = true

			// Add "Remove from MRU" action to each MRU result
			removeMRUAction := QueryResultAction{
				Id:   uuid.NewString(),
				Name: i18n.GetI18nManager().TranslateWox(ctx, "mru_remove_action"),
				Icon: common.TrashIcon,
				Action: func(ctx context.Context, actionContext ActionContext) {
					err := setting.GetSettingManager().RemoveMRUItem(ctx, item.Hash)
					if err != nil {
						util.GetLogger().Error(ctx, fmt.Sprintf("failed to remove MRU item: %s", err.Error()))
					} else {
						util.GetLogger().Info(ctx, fmt.Sprintf("removed MRU item: %s - %s", item.Title, item.SubTitle))
					}
				},
			}

			// Add the remove action to the result
			restored.Actions = append(restored.Actions, removeMRUAction)

			polishedResult := m.PolishResult(ctx, pluginInstance, query, *restored)
			results = append(results, polishedResult.ToUI())
		}
	}

	if len(results) == 0 {
		return results
	}

	return m.BuildQueryResultsSnapshot(query.SessionId, query.Id)
}

// getPluginInstance finds a plugin instance by ID
func (m *Manager) getPluginInstance(pluginID string) *Instance {
	pluginInstance, found := lo.Find(m.instances, func(item *Instance) bool {
		return item.Metadata.Id == pluginID
	})
	if found {
		return pluginInstance
	}
	return nil
}

// restoreFromMRU attempts to restore a QueryResult from MRU data
func (m *Manager) restoreFromMRU(ctx context.Context, pluginInstance *Instance, item setting.MRUItem) *QueryResult {
	// For Go plugins, call MRU restore callbacks directly
	if len(pluginInstance.MRURestoreCallbacks) > 0 {
		mruData := MRUData{
			PluginID:    item.PluginID,
			Title:       item.Title,
			SubTitle:    item.SubTitle,
			Icon:        item.Icon,
			ContextData: item.ContextData,
			LastUsed:    item.LastUsed,
			UseCount:    item.UseCount,
		}

		// Call the first (and typically only) MRU restore callback
		if restored, err := pluginInstance.MRURestoreCallbacks[0](ctx, mruData); err == nil {
			return restored
		} else {
			util.GetLogger().Debug(ctx, fmt.Sprintf("MRU restore failed for plugin %s: %s", pluginInstance.GetName(ctx), err.Error()))
		}
	}

	// For external plugins (Python/Node.js), MRU support will be implemented later
	// Currently only Go plugins support MRU functionality
	if pluginInstance.Host != nil {
		util.GetLogger().Debug(ctx, fmt.Sprintf("External plugin MRU restore not yet implemented for plugin %s", pluginInstance.GetName(ctx)))
	}

	return nil
}

// normalizeHotkeyForPlatform converts hotkey strings to platform-specific format
// This function provides better cross-platform hotkey support by handling various
// modifier key aliases and converting them to the appropriate platform format
func normalizeHotkeyForPlatform(hotkey string) string {
	if hotkey == "" {
		return hotkey
	}

	// Convert to lowercase for case-insensitive matching
	normalized := strings.ToLower(strings.TrimSpace(hotkey))

	// Define platform-specific modifier mappings
	var modifierMappings map[string]string

	if util.IsMacOS() {
		// On macOS: convert Windows/Linux style to macOS style
		modifierMappings = map[string]string{
			"win":     "cmd",
			"windows": "cmd",
			"ctrl":    "control", // Keep ctrl as control on macOS
			"control": "control", // Keep explicit control as-is
			"alt":     "option",
		}
	} else {
		// On Windows/Linux: convert macOS style to Windows/Linux style
		modifierMappings = map[string]string{
			"cmd":     "win", // Map cmd to win key on Windows/Linux
			"command": "win",
			"option":  "alt",
			"control": "ctrl", // Keep control as ctrl
			"ctrl":    "ctrl", // Keep ctrl as-is
		}
	}

	// Split hotkey into parts and process each modifier
	parts := strings.Split(normalized, "+")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if replacement, exists := modifierMappings[part]; exists {
			parts[i] = replacement
		} else {
			parts[i] = part
		}
	}

	return strings.Join(parts, "+")
}

type toolbarMsgActionEntry struct {
	PluginId string
	Actions  map[string]ToolbarMsgAction
}

func glanceActionCacheKey(pluginId, glanceId, actionId string) string {
	return pluginId + "\x00" + glanceId + "\x00" + actionId
}

func (m *Manager) clearGlanceActions(pluginId string, glanceIds []string) {
	// A refreshed glance may return no items, so stale callbacks must be removed
	// before the new response is normalized instead of relying on UI visibility.
	prefixes := make([]string, 0, len(glanceIds))
	for _, glanceId := range glanceIds {
		prefixes = append(prefixes, glanceActionCacheKey(pluginId, glanceId, ""))
	}

	for _, key := range m.glanceActions.Keys() {
		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix) {
				m.glanceActions.Delete(key)
				break
			}
		}
	}
}

func (m *Manager) GetGlanceItems(ctx context.Context, keys []GlanceKey, reason GlanceRefreshReason) []GlanceItemUI {
	requestIdsByPlugin := map[string][]string{}
	requested := map[string]bool{}
	for _, key := range keys {
		if key.PluginId == "" || key.GlanceId == "" {
			continue
		}
		cacheKey := key.PluginId + "\x00" + key.GlanceId
		if requested[cacheKey] {
			continue
		}
		requested[cacheKey] = true
		requestIdsByPlugin[key.PluginId] = append(requestIdsByPlugin[key.PluginId], key.GlanceId)
	}

	var uiItems []GlanceItemUI
	for pluginId, ids := range requestIdsByPlugin {
		pluginInstance := m.GetPluginInstanceById(pluginId)
		if pluginInstance == nil || pluginInstance.Setting.Disabled.Get() {
			continue
		}
		provider, ok := pluginInstance.Plugin.(GlanceProvider)
		if !ok {
			continue
		}

		declared := map[string]bool{}
		var declaredIds []string
		for _, id := range ids {
			if pluginInstance.Metadata.HasGlance(id) {
				declared[id] = true
				declaredIds = append(declaredIds, id)
			}
		}
		if len(declaredIds) == 0 {
			continue
		}

		// Global Glance is a pull-based API. Calling the plugin only for user-selected
		// ids keeps third-party providers from occupying the query-box accessory area
		// unless the setting explicitly points at them.
		m.clearGlanceActions(pluginId, declaredIds)
		response := provider.Glance(ctx, GlanceRequest{Ids: declaredIds, Reason: reason})
		for _, item := range response.Items {
			if !declared[item.Id] {
				continue
			}
			uiItems = append(uiItems, m.normalizeGlanceItem(ctx, pluginInstance, item))
		}
	}
	return uiItems
}

func (m *Manager) normalizeGlanceItem(ctx context.Context, pluginInstance *Instance, item GlanceItem) GlanceItemUI {
	uiItem := GlanceItemUI{
		PluginId: pluginInstance.Metadata.Id,
		Id:       item.Id,
		Text:     pluginInstance.translateMetadataText(ctx, common.I18nString(item.Text)),
		Icon:     common.ConvertIcon(ctx, item.Icon, pluginInstance.PluginDirectory),
		Tooltip:  pluginInstance.translateMetadataText(ctx, common.I18nString(item.Tooltip)),
	}

	if item.Action != nil {
		// Glance intentionally exposes only one action in v1. A single optional
		// callback keeps the query-box accessory glanceable instead of turning it
		// into a secondary action menu.
		action := *item.Action
		if action.Id == "" {
			action.Id = uuid.NewString()
		}
		action.Name = pluginInstance.translateMetadataText(ctx, common.I18nString(action.Name))
		action.ContextData = common.ContextData(lo.Assign(map[string]string{}, action.ContextData))
		if !action.Icon.IsEmpty() {
			action.Icon = common.ConvertIcon(ctx, action.Icon, pluginInstance.PluginDirectory)
		}
		m.glanceActions.Store(glanceActionCacheKey(pluginInstance.Metadata.Id, item.Id, action.Id), action)
		uiItem.Action = &GlanceActionUI{
			Id:                     action.Id,
			Name:                   action.Name,
			Icon:                   action.Icon,
			PreventHideAfterAction: action.PreventHideAfterAction,
			ContextData:            common.ContextData(lo.Assign(map[string]string{}, action.ContextData)),
		}
	}

	return uiItem
}

func (m *Manager) ExecuteGlanceAction(ctx context.Context, pluginId string, glanceId string, actionId string) error {
	action, found := m.glanceActions.Load(glanceActionCacheKey(pluginId, glanceId, actionId))
	if !found {
		return fmt.Errorf("glance action not found: %s/%s/%s", pluginId, glanceId, actionId)
	}
	if action.Action == nil {
		return fmt.Errorf("glance action callback missing: %s", actionId)
	}
	action.Action(ctx, GlanceActionContext{
		PluginId:    pluginId,
		GlanceId:    glanceId,
		ActionId:    actionId,
		ContextData: common.ContextData(lo.Assign(map[string]string{}, action.ContextData)),
	})
	return nil
}

type sessionPluginQueryState struct {
	PluginId string
	QueryId  string
}

// ShowToolbarMsg normalizes action callbacks, replaces the current toolbar msg owned by this plugin,
// and forwards the latest snapshot to UI.
func (m *Manager) ShowToolbarMsg(ctx context.Context, pluginInstance *Instance, msg ToolbarMsg) {
	if pluginInstance == nil {
		return
	}
	resolvedCtx, ok := m.resolveActiveToolbarMsgContext(ctx, pluginInstance.Metadata.Id)
	if !ok {
		return
	}
	if msg.Id == "" {
		msg.Id = uuid.NewString()
	}

	normalized := m.normalizeToolbarMsg(resolvedCtx, pluginInstance, msg)
	m.clearCurrentPluginToolbarMsgAction(pluginInstance.Metadata.Id)

	actionEntry := &toolbarMsgActionEntry{
		PluginId: pluginInstance.Metadata.Id,
		Actions:  make(map[string]ToolbarMsgAction, len(normalized.Actions)),
	}
	for _, action := range normalized.Actions {
		actionEntry.Actions[action.Id] = action
	}

	m.toolbarMsgActions.Store(normalized.Id, actionEntry)
	m.pluginToolbarMsgIds.Store(pluginInstance.Metadata.Id, normalized.Id)
	m.GetUI().ShowToolbarMsg(resolvedCtx, normalized.toToolbarMsgUI())
}

// ClearToolbarMsg removes the toolbar msg action callbacks owned by the current plugin and asks UI
// to clear the matching toolbar msg if it is still visible.
func (m *Manager) ClearToolbarMsg(ctx context.Context, pluginInstance *Instance, toolbarMsgId string) {
	if pluginInstance == nil || toolbarMsgId == "" {
		return
	}
	resolvedCtx, ok := m.resolveActiveToolbarMsgContext(ctx, pluginInstance.Metadata.Id)
	if !ok {
		resolvedCtx = ctx
	}

	if currentToolbarMsgId, found := m.pluginToolbarMsgIds.Load(pluginInstance.Metadata.Id); found && currentToolbarMsgId == toolbarMsgId {
		m.pluginToolbarMsgIds.Delete(pluginInstance.Metadata.Id)
	}
	m.toolbarMsgActions.Delete(toolbarMsgId)
	m.GetUI().ClearToolbarMsg(resolvedCtx, toolbarMsgId)
}

// ExecuteToolbarMsgAction resolves the current toolbar msg action callback and invokes it.
func (m *Manager) ExecuteToolbarMsgAction(ctx context.Context, sessionId string, toolbarMsgId string, actionId string) error {
	entry, found := m.toolbarMsgActions.Load(toolbarMsgId)
	if !found {
		return fmt.Errorf("toolbar msg not found: %s", toolbarMsgId)
	}

	action, found := entry.Actions[actionId]
	if !found {
		return fmt.Errorf("toolbar msg action not found: %s", actionId)
	}
	if action.Action == nil {
		return fmt.Errorf("toolbar msg action callback missing: %s", actionId)
	}

	actionCtx := ToolbarMsgActionContext{
		ToolbarMsgId:       toolbarMsgId,
		ToolbarMsgActionId: actionId,
		ContextData:        common.ContextData(lo.Assign(map[string]string{}, action.ContextData)),
	}

	callbackCtx := ctx
	if sessionId != "" {
		callbackCtx = util.WithSessionContext(callbackCtx, sessionId)
	}
	action.Action(callbackCtx, actionCtx)
	return nil
}

// HasVisibleToolbarMsg reports whether backend currently tracks any persistent toolbar msg. This is
// an approximation used for routing Notify() calls without maintaining toolbar visibility policy.
func (m *Manager) HasVisibleToolbarMsg(ctx context.Context) bool {
	return m.pluginToolbarMsgIds.Len() > 0
}

func (m *Manager) clearCurrentPluginToolbarMsgAction(pluginId string) {
	if pluginId == "" {
		return
	}

	currentToolbarMsgId, found := m.pluginToolbarMsgIds.Load(pluginId)
	if !found || currentToolbarMsgId == "" {
		return
	}

	m.pluginToolbarMsgIds.Delete(pluginId)
	m.toolbarMsgActions.Delete(currentToolbarMsgId)
}

func (m *Manager) clearCurrentPluginToolbarMsg(ctx context.Context, pluginId string) {
	if pluginId == "" {
		return
	}

	currentToolbarMsgId, found := m.pluginToolbarMsgIds.Load(pluginId)
	if !found || currentToolbarMsgId == "" {
		return
	}

	m.clearCurrentPluginToolbarMsgAction(pluginId)
	m.GetUI().ClearToolbarMsg(ctx, currentToolbarMsgId)
}

func (m *Manager) resolveActiveToolbarMsgContext(ctx context.Context, pluginId string) (context.Context, bool) {
	sessionId := util.GetContextSessionId(ctx)
	if sessionId != "" {
		return ctx, m.isPluginActiveInSession(sessionId, pluginId)
	}

	activeSessionId, activeQueryId, found := m.findActivePluginQuery(pluginId)
	if !found {
		return nil, false
	}

	resolvedCtx := util.WithSessionContext(ctx, activeSessionId)
	if activeQueryId != "" {
		resolvedCtx = util.WithQueryIdContext(resolvedCtx, activeQueryId)
	}
	return resolvedCtx, true
}

// normalizeToolbarMsg translates user-facing text, normalizes UI-facing icons, clones context data,
// and backfills host proxies for external plugin action callbacks.
func (m *Manager) normalizeToolbarMsg(ctx context.Context, pluginInstance *Instance, msg ToolbarMsg) ToolbarMsg {
	normalizedIcon := common.ConvertIcon(ctx, msg.Icon, pluginInstance.PluginDirectory)

	normalized := ToolbarMsg{
		Id:            msg.Id,
		Title:         pluginInstance.translateMetadataText(ctx, common.I18nString(msg.Title)),
		Icon:          normalizedIcon,
		Progress:      msg.Progress,
		Indeterminate: msg.Indeterminate,
		Actions:       make([]ToolbarMsgAction, 0, len(msg.Actions)),
	}

	for _, action := range msg.Actions {
		if action.Id == "" {
			action.Id = uuid.NewString()
		}
		action.Name = pluginInstance.translateMetadataText(ctx, common.I18nString(action.Name))
		action.ContextData = common.ContextData(lo.Assign(map[string]string{}, action.ContextData))
		if !action.Icon.IsEmpty() {
			// Action icons share the same toolbar payload path as the message icon. Normalize them
			// before storing callbacks so plugin-relative assets stay usable when actions are shown.
			action.Icon = common.ConvertIcon(ctx, action.Icon, pluginInstance.PluginDirectory)
		}

		if action.Action == nil {
			if proxyCreator, ok := pluginInstance.Plugin.(ToolbarMsgActionProxyCreator); ok {
				action.Action = proxyCreator.CreateToolbarMsgActionProxy(action.Id)
			}
		}

		normalized.Actions = append(normalized.Actions, action)
	}

	return normalized
}

// toToolbarMsgUI strips callbacks and returns a UI-safe snapshot.
func (m ToolbarMsg) toToolbarMsgUI() ToolbarMsgUI {
	uiMsg := ToolbarMsgUI{
		Id:            m.Id,
		Title:         m.Title,
		Icon:          m.Icon,
		Progress:      m.Progress,
		Indeterminate: m.Indeterminate,
		Actions:       make([]ToolbarMsgActionUI, 0, len(m.Actions)),
	}
	for _, action := range m.Actions {
		uiMsg.Actions = append(uiMsg.Actions, ToolbarMsgActionUI{
			Id:                     action.Id,
			Name:                   action.Name,
			Icon:                   action.Icon,
			Hotkey:                 action.Hotkey,
			IsDefault:              action.IsDefault,
			PreventHideAfterAction: action.PreventHideAfterAction,
			ContextData:            common.ContextData(lo.Assign(map[string]string{}, action.ContextData)),
		})
	}
	return uiMsg
}

// HandleQueryLifecycle updates the active plugin query for the session and fires enter/leave callbacks
// when the owning plugin changes. Leaving a plugin query also clears its toolbar msg snapshot.
func (m *Manager) HandleQueryLifecycle(ctx context.Context, query Query, pluginInstance *Instance) {
	sessionId := query.SessionId
	if sessionId == "" {
		return
	}

	nextPluginId := ""
	if pluginInstance != nil && query.Type == QueryTypeInput && query.TriggerKeyword != "" {
		nextPluginId = pluginInstance.Metadata.Id
	}

	prevState, hasPrev := m.sessionPluginQueries.Load(sessionId)
	prevPluginId := ""
	prevQueryId := ""
	if hasPrev {
		prevPluginId = prevState.PluginId
		prevQueryId = prevState.QueryId
	}

	if prevPluginId == nextPluginId {
		if nextPluginId == "" {
			m.sessionPluginQueries.Delete(sessionId)
		} else {
			m.sessionPluginQueries.Store(sessionId, &sessionPluginQueryState{PluginId: nextPluginId, QueryId: query.Id})
		}
		return
	}

	// Switch the active owner first so background plugin callbacks cannot re-resolve the
	// previous plugin after we leave its query context.
	if nextPluginId == "" {
		m.sessionPluginQueries.Delete(sessionId)
	} else {
		m.sessionPluginQueries.Store(sessionId, &sessionPluginQueryState{PluginId: nextPluginId, QueryId: query.Id})
	}

	if prevPluginId != "" {
		leaveCtx := util.WithQueryIdContext(util.WithSessionContext(ctx, sessionId), prevQueryId)
		m.clearCurrentPluginToolbarMsg(leaveCtx, prevPluginId)

		if prevInstance := m.getPluginInstance(prevPluginId); prevInstance != nil {
			for _, callback := range prevInstance.LeavePluginQueryCallbacks {
				util.Go(leaveCtx, fmt.Sprintf("[%s] leave plugin query callback", prevInstance.GetName(leaveCtx)), func() {
					callback(leaveCtx)
				})
			}
		}
	}

	if nextPluginId == "" {
		return
	}

	if nextInstance := m.getPluginInstance(nextPluginId); nextInstance != nil {
		enterCtx := util.WithQueryIdContext(util.WithSessionContext(ctx, sessionId), query.Id)
		for _, callback := range nextInstance.EnterPluginQueryCallbacks {
			util.Go(enterCtx, fmt.Sprintf("[%s] enter plugin query callback", nextInstance.GetName(enterCtx)), func() {
				callback(enterCtx)
			})
		}
	}
}

// isPluginActiveInSession reports whether the given plugin currently owns the session query context.
func (m *Manager) isPluginActiveInSession(sessionId string, pluginId string) bool {
	if sessionId == "" || pluginId == "" {
		return false
	}

	state, found := m.sessionPluginQueries.Load(sessionId)
	return found && state.PluginId == pluginId
}

// findActivePluginQuery returns one active session/query pair for the plugin.
// Toolbar msg routing uses this to resolve the UI session when plugins fire status updates
// without carrying a session-bound context.
func (m *Manager) findActivePluginQuery(pluginId string) (string, string, bool) {
	if pluginId == "" {
		return "", "", false
	}

	var sessionId string
	var queryId string
	m.sessionPluginQueries.Range(func(currentSessionId string, state *sessionPluginQueryState) bool {
		if state != nil && state.PluginId == pluginId {
			sessionId = currentSessionId
			queryId = state.QueryId
			return false
		}
		return true
	})

	return sessionId, queryId, sessionId != ""
}
