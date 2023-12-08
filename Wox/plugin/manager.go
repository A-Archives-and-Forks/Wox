package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/wissance/stringFormatter"
	"math"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"wox/i18n"
	"wox/setting"
	"wox/share"
	"wox/util"
)

var managerInstance *Manager
var managerOnce sync.Once
var logger *util.Log

type debounceTimer struct {
	timer  *time.Timer
	onStop func()
}

type Manager struct {
	instances          []*Instance
	ui                 share.UI
	resultCache        *util.HashMap[string, *QueryResultCache]
	debounceQueryTimer *util.HashMap[string, *debounceTimer]
}

func GetPluginManager() *Manager {
	managerOnce.Do(func() {
		managerInstance = &Manager{
			resultCache:        util.NewHashMap[string, *QueryResultCache](),
			debounceQueryTimer: util.NewHashMap[string, *debounceTimer](),
		}
		logger = util.GetLogger()
	})
	return managerInstance
}

func (m *Manager) Start(ctx context.Context, ui share.UI) error {
	m.ui = ui

	loadErr := m.loadPlugins(ctx)
	if loadErr != nil {
		return fmt.Errorf("failed to load plugins: %w", loadErr)
	}

	util.Go(ctx, "start store manager", func() {
		GetStoreManager().Start(util.NewTraceContext())
	})

	return nil
}

func (m *Manager) Stop(ctx context.Context) {
	for _, host := range AllHosts {
		host.Stop(ctx)
	}
}

func (m *Manager) loadPlugins(ctx context.Context) error {
	logger.Info(ctx, "start loading plugins")

	// load system plugin first
	m.loadSystemPlugins(ctx)

	basePluginDirectory := util.GetLocation().GetPluginDirectory()
	pluginDirectories, readErr := os.ReadDir(basePluginDirectory)
	if readErr != nil {
		return fmt.Errorf("failed to read plugin directory: %w", readErr)
	}

	var metaDataList []MetadataWithDirectory
	for _, entry := range pluginDirectories {
		pluginDirectory := path.Join(basePluginDirectory, entry.Name())
		metadata, metadataErr := m.parseMetadata(ctx, pluginDirectory)
		if metadataErr != nil {
			logger.Error(ctx, metadataErr.Error())
			continue
		}

		//check if metadata already exist, only add newer version
		existMetadata, exist := lo.Find(metaDataList, func(item MetadataWithDirectory) bool {
			return item.Metadata.Id == metadata.Id
		})
		if exist {
			existVersion, existVersionErr := semver.NewVersion(existMetadata.Metadata.Version)
			currentVersion, currentVersionErr := semver.NewVersion(metadata.Version)
			if existVersionErr == nil && currentVersionErr == nil {
				if existVersion.GreaterThan(currentVersion) || existVersion.Equal(currentVersion) {
					logger.Info(ctx, fmt.Sprintf("skip parse %s(%s) metadata, because it's already parsed(%s)", metadata.Name, metadata.Version, existMetadata.Metadata.Version))
					continue
				} else {
					// remove older version
					logger.Info(ctx, fmt.Sprintf("remove older metadata version %s(%s)", existMetadata.Metadata.Name, existMetadata.Metadata.Version))
					var newMetaDataList []MetadataWithDirectory
					for _, item := range metaDataList {
						if item.Metadata.Id != existMetadata.Metadata.Id {
							newMetaDataList = append(newMetaDataList, item)
						}
					}
					metaDataList = newMetaDataList
				}
			}
		}
		metaDataList = append(metaDataList, MetadataWithDirectory{metadata, pluginDirectory})
	}
	logger.Info(ctx, fmt.Sprintf("start loading user plugins, found %d user plugins", len(metaDataList)))

	for _, h := range AllHosts {
		host := h
		util.Go(ctx, fmt.Sprintf("[%s] start host", host.GetRuntime(ctx)), func() {
			newCtx := util.NewTraceContext()
			hostErr := host.Start(newCtx)
			if hostErr != nil {
				logger.Error(newCtx, fmt.Errorf("[%s HOST] %w", host.GetRuntime(newCtx), hostErr).Error())
				return
			}

			for _, metadata := range metaDataList {
				if strings.ToUpper(metadata.Metadata.Runtime) != strings.ToUpper(string(host.GetRuntime(newCtx))) {
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

func (m *Manager) loadHostPlugin(ctx context.Context, host Host, metadata MetadataWithDirectory) error {
	loadStartTimestamp := util.GetSystemTimestamp()
	plugin, loadErr := host.LoadPlugin(ctx, metadata.Metadata, metadata.Directory)
	if loadErr != nil {
		logger.Error(ctx, fmt.Errorf("[%s HOST] failed to load plugin: %w", host.GetRuntime(ctx), loadErr).Error())
		return loadErr
	}
	loadFinishTimestamp := util.GetSystemTimestamp()

	pluginSetting, settingErr := setting.GetSettingManager().LoadPluginSetting(ctx, metadata.Metadata.Id, metadata.Metadata.SettingDefinitions)
	if settingErr != nil {
		return settingErr
	}

	instance := &Instance{
		Metadata:              metadata.Metadata,
		PluginDirectory:       metadata.Directory,
		Plugin:                plugin,
		Host:                  host,
		Setting:               pluginSetting,
		LoadStartTimestamp:    loadStartTimestamp,
		LoadFinishedTimestamp: loadFinishTimestamp,
	}
	instance.API = NewAPI(instance)
	m.instances = append(m.instances, instance)

	if pluginSetting.Disabled {
		logger.Info(ctx, fmt.Errorf("[%s HOST] plugin is disabled by user, skip init: %s", host.GetRuntime(ctx), metadata.Metadata.Name).Error())
		return nil
	}

	util.Go(ctx, fmt.Sprintf("[%s] init plugin", metadata.Metadata.Name), func() {
		m.initPlugin(ctx, instance)
	})

	return nil
}

func (m *Manager) LoadPlugin(ctx context.Context, pluginDirectory string) error {
	metadata, parseErr := m.parseMetadata(ctx, pluginDirectory)
	if parseErr != nil {
		return parseErr
	}

	pluginHost, exist := lo.Find(AllHosts, func(item Host) bool {
		return strings.ToLower(string(item.GetRuntime(ctx))) == strings.ToLower(metadata.Runtime)
	})
	if !exist {
		return fmt.Errorf("unsupported runtime: %s", metadata.Runtime)
	}

	loadErr := m.loadHostPlugin(ctx, pluginHost, MetadataWithDirectory{metadata, pluginDirectory})
	if loadErr != nil {
		return loadErr
	}

	return nil
}

func (m *Manager) UnloadPlugin(ctx context.Context, pluginInstance *Instance) {
	pluginInstance.Host.UnloadPlugin(ctx, pluginInstance.Metadata)

	var newInstances []*Instance
	for _, instance := range m.instances {
		if instance.Metadata.Id != pluginInstance.Metadata.Id {
			newInstances = append(newInstances, instance)
		}
	}
	m.instances = newInstances
}

func (m *Manager) loadSystemPlugins(ctx context.Context) {
	logger.Info(ctx, fmt.Sprintf("start loading system plugins, found %d system plugins", len(AllSystemPlugin)))

	for _, plugin := range AllSystemPlugin {
		metadata := plugin.GetMetadata()
		pluginSetting, settingErr := setting.GetSettingManager().LoadPluginSetting(ctx, metadata.Id, metadata.SettingDefinitions)
		if settingErr != nil {
			logger.Error(ctx, fmt.Errorf("failed to load system plugin[%s] setting, use default plugin setting: %w", metadata.Name, settingErr).Error())
			pluginSetting = &setting.PluginSetting{
				Settings: util.NewHashMap[string, string](),
			}
		}

		instance := &Instance{
			Metadata:              plugin.GetMetadata(),
			Plugin:                plugin,
			Host:                  nil,
			Setting:               pluginSetting,
			IsSystemPlugin:        true,
			LoadStartTimestamp:    util.GetSystemTimestamp(),
			LoadFinishedTimestamp: util.GetSystemTimestamp(),
		}
		instance.API = NewAPI(instance)
		m.instances = append(m.instances, instance)

		util.Go(ctx, fmt.Sprintf("[%s] init system plugin", plugin.GetMetadata().Name), func() {
			m.initPlugin(util.NewTraceContext(), instance)
		})
	}
}

func (m *Manager) initPlugin(ctx context.Context, instance *Instance) {
	logger.Info(ctx, fmt.Sprintf("[%s] init plugin", instance.Metadata.Name))
	instance.InitStartTimestamp = util.GetSystemTimestamp()
	instance.Plugin.Init(ctx, InitParams{
		API:             instance.API,
		PluginDirectory: instance.PluginDirectory,
	})
	instance.InitFinishedTimestamp = util.GetSystemTimestamp()
	logger.Info(ctx, fmt.Sprintf("[%s] init plugin finished, cost %d ms", instance.Metadata.Name, instance.InitFinishedTimestamp-instance.InitStartTimestamp))
}

func (m *Manager) parseMetadata(ctx context.Context, pluginDirectory string) (Metadata, error) {
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
	if !IsSupportedOSAny(metadata.SupportedOS) {
		return Metadata{}, fmt.Errorf("unsupported os in plugin.json file (%s), os=%s", pluginDirectory, metadata.SupportedOS)
	}

	return metadata, nil
}

func (m *Manager) GetPluginInstances() []*Instance {
	return m.instances
}

func (m *Manager) canOperateQuery(ctx context.Context, pluginInstance *Instance, query Query) bool {
	var validGlobalQuery = lo.Contains(pluginInstance.GetTriggerKeywords(), "*") && query.TriggerKeyword == ""
	var validNonGlobalQuery = lo.Contains(pluginInstance.GetTriggerKeywords(), query.TriggerKeyword)
	if !validGlobalQuery && !validNonGlobalQuery {
		return false
	}
	if query.Type == QueryTypeSelection && !pluginInstance.Metadata.IsSupportFeature(MetadataFeatureQuerySelection) {
		return false
	}
	if pluginInstance.Setting.Disabled {
		return false
	}

	return true
}

func (m *Manager) queryForPlugin(ctx context.Context, pluginInstance *Instance, query Query) []QueryResult {
	logger.Info(ctx, fmt.Sprintf("[%s] start query: %s", pluginInstance.Metadata.Name, query.RawQuery))
	start := util.GetSystemTimestamp()
	results := pluginInstance.Plugin.Query(ctx, query)
	logger.Debug(ctx, fmt.Sprintf("[%s] finish query, result count: %d, cost: %dms", pluginInstance.Metadata.Name, len(results), util.GetSystemTimestamp()-start))

	for i := range results {
		results[i] = m.PolishResult(ctx, pluginInstance, query, results[i])
	}
	return results
}

func (m *Manager) PolishResult(ctx context.Context, pluginInstance *Instance, query Query, result QueryResult) QueryResult {
	// set default id
	if result.Id == "" {
		result.Id = uuid.NewString()
	}
	for actionIndex := range result.Actions {
		if result.Actions[actionIndex].Id == "" {
			result.Actions[actionIndex].Id = uuid.NewString()
		}
		if result.Actions[actionIndex].Icon.ImageType == "" {
			result.Actions[actionIndex].Icon = NewWoxImageBase64(`data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAB4AAAAeCAYAAAA7MK6iAAAACXBIWXMAAAsTAAALEwEAmpwYAAAAVklEQVR4nO3VQQqAMAxE0X88S29tb2K9hyJ0EQQhbirCf5BVFgOBISBNVIEd6ECZGdyBY8x227WwezPtq+A1E1xG+BW6pO8kJdjjyB7rn6r/OLDHEk9OW8N7ef+eTPQAAAAASUVORK5CYII=`)
		}
	}

	// convert icon
	result.Icon = ConvertIcon(ctx, result.Icon, pluginInstance.PluginDirectory)

	// if query is input and trigger keyword is global, disable preview
	if query.IsGlobalQuery() {
		result.Preview = WoxPreview{}
	}
	// if query is selection, replace preview with selection
	if query.Type == QueryTypeSelection {
		if query.Selection.Type == util.SelectionTypeText {
			result.Preview = WoxPreview{
				PreviewType: WoxPreviewTypeText,
				PreviewData: query.Selection.Text,
			}
		}
		if query.Selection.Type == util.SelectionTypeFile {
			result.Preview = WoxPreview{
				PreviewType: WoxPreviewTypeMarkdown,
				PreviewData: strings.Join(query.Selection.FilePaths, "\n"),
			}
		}
	}

	// translate title
	result.Title = m.translatePlugin(ctx, pluginInstance, result.Title)
	// translate subtitle
	result.SubTitle = m.translatePlugin(ctx, pluginInstance, result.SubTitle)
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
	}

	// set first action as default if no default action is set
	defaultActionCount := lo.CountBy(result.Actions, func(item QueryResultAction) bool {
		return item.IsDefault
	})
	if defaultActionCount == 0 && len(result.Actions) > 0 {
		result.Actions[0].IsDefault = true
	}

	var resultCache = &QueryResultCache{
		ResultId:       result.Id,
		ResultTitle:    result.Title,
		ResultSubTitle: result.SubTitle,
		ContextData:    result.ContextData,
		PluginInstance: pluginInstance,
		Query:          query,
		Actions:        util.NewHashMap[string, func(actionContext ActionContext)](),
	}

	// store actions for ui invoke later
	for actionId := range result.Actions {
		var action = result.Actions[actionId]
		if action.Action != nil {
			resultCache.Actions.Store(action.Id, action.Action)
		}
	}

	// store preview for ui invoke later
	// because preview may contain some heavy data (E.g. image or large text), we will store preview in cache and only send preview to ui when user select the result
	if result.Preview.PreviewType != "" && result.Preview.PreviewType != WoxPreviewTypeRemote {
		resultCache.Preview = result.Preview
		result.Preview = WoxPreview{
			PreviewType: WoxPreviewTypeRemote,
			PreviewData: fmt.Sprintf("/preview?id=%s", result.Id),
		}
	}

	if result.RefreshInterval > 0 && result.OnRefresh != nil {
		newInterval := int(math.Floor(float64(result.RefreshInterval)/100) * 100)
		if result.RefreshInterval != newInterval {
			logger.Info(ctx, fmt.Sprintf("[%s] result(%s) refresh interval %d is not divisible by 100, use %d instead", pluginInstance.Metadata.Name, result.Id, result.RefreshInterval, newInterval))
			result.RefreshInterval = newInterval
		}
		resultCache.Refresh = result.OnRefresh
	}

	ignoreAutoScore := pluginInstance.Metadata.IsSupportFeature(MetadataFeatureIgnoreAutoScore)
	if !ignoreAutoScore {
		score := m.calculateResultScore(ctx, pluginInstance.Metadata.Id, result.Title, result.SubTitle)
		if score > 0 {
			logger.Info(ctx, fmt.Sprintf("[%s] result(%s) add score: %d", pluginInstance.Metadata.Name, result.Title, score))
			result.Score += score
		}
	}

	m.resultCache.Store(result.Id, resultCache)

	return result
}

func (m *Manager) calculateResultScore(ctx context.Context, pluginId, title, subTitle string) int64 {
	resultHash := setting.NewResultHash(pluginId, title, subTitle)
	woxAppData := setting.GetSettingManager().GetWoxAppData(ctx)
	actionResults, ok := woxAppData.ActionedResults.Load(resultHash)
	if !ok {
		return 0
	}

	// actioned score are based on actioned counts, the more actioned, the more score
	// also, action timestamp will be considered, the more recent actioned, the more score weight. If action is in recent 7 days, it will be considered as recent actioned and add score weight
	// we will use fibonacci sequence to calculate score, the more recent actioned, the more score: 5, 8, 13, 21, 34, 55, 89
	// that means, actions in day one, we will add weight 89, day two, we will add weight 55, day three, we will add weight 34, and so on
	// E.g. if actioned 3 times in day one, 2 times in day two, 1 time in day three, the score will be: 89*3 + 55*2 + 34*1 = 450

	var score int64 = 0
	for _, actionResult := range actionResults {
		var weight int64 = 2

		actionedTime := util.ParseTimeStamp(actionResult.Timestamp)
		hours := util.GetSystemTime().Sub(actionedTime).Hours()
		if hours < 24*7 {
			fibonacciIndex := int(math.Ceil(hours / 24))
			if fibonacciIndex > 7 {
				fibonacciIndex = 7
			}
			if fibonacciIndex < 1 {
				fibonacciIndex = 1
			}
			fibonacci := []int64{5, 8, 13, 21, 34, 55, 89}
			score += fibonacci[7-fibonacciIndex]
		}

		score += weight
	}

	return score
}

func (m *Manager) PolishRefreshableResult(ctx context.Context, pluginInstance *Instance, result RefreshableResult) RefreshableResult {
	// convert icon
	result.Icon = ConvertIcon(ctx, result.Icon, pluginInstance.PluginDirectory)
	// translate title
	result.Title = m.translatePlugin(ctx, pluginInstance, result.Title)
	// translate subtitle
	result.SubTitle = m.translatePlugin(ctx, pluginInstance, result.SubTitle)
	// translate preview properties
	var previewProperties = make(map[string]string)
	for key, value := range result.Preview.PreviewProperties {
		translatedKey := m.translatePlugin(ctx, pluginInstance, key)
		previewProperties[translatedKey] = value
	}
	result.Preview.PreviewProperties = previewProperties
	return result
}

func (m *Manager) Query(ctx context.Context, query Query) (results chan []QueryResultUI, done chan bool) {
	results = make(chan []QueryResultUI, 10)
	done = make(chan bool)

	// clear old result cache
	m.resultCache.Clear()

	counter := &atomic.Int32{}
	counter.Store(int32(len(m.instances)))

	for _, instance := range m.instances {
		pluginInstance := instance
		if pluginInstance.Metadata.IsSupportFeature(MetadataFeatureDebounce) {
			debounceParams, err := pluginInstance.Metadata.GetFeatureParamsForDebounce()
			if err == nil {
				logger.Debug(ctx, fmt.Sprintf("[%s] debounce query, will execute in %d ms", pluginInstance.Metadata.Name, debounceParams.intervalMs))
				if v, ok := m.debounceQueryTimer.Load(pluginInstance.Metadata.Id); ok {
					if v.timer.Stop() {
						v.onStop()
					}
				}

				timer := time.AfterFunc(time.Duration(debounceParams.intervalMs)*time.Millisecond, func() {
					m.queryParallel(ctx, pluginInstance, query, results, done, counter)
				})
				onStop := func() {
					logger.Debug(ctx, fmt.Sprintf("[%s] previous debounced query cancelled", pluginInstance.Metadata.Name))
					counter.Add(-1)
					if counter.Load() == 0 {
						done <- true
					}
				}
				m.debounceQueryTimer.Store(pluginInstance.Metadata.Id, &debounceTimer{
					timer:  timer,
					onStop: onStop,
				})
				continue
			} else {
				logger.Error(ctx, fmt.Sprintf("[%s] %s, query directlly", pluginInstance.Metadata.Name, err))
			}
		}

		m.queryParallel(ctx, pluginInstance, query, results, done, counter)
	}

	return
}

func (m *Manager) queryParallel(ctx context.Context, pluginInstance *Instance, query Query, results chan []QueryResultUI, done chan bool, counter *atomic.Int32) {
	util.Go(ctx, fmt.Sprintf("[%s] parallel query", pluginInstance.Metadata.Name), func() {
		if !m.canOperateQuery(ctx, pluginInstance, query) {
			counter.Add(-1)
			if counter.Load() == 0 {
				done <- true
			}
			return
		}

		queryResults := m.queryForPlugin(ctx, pluginInstance, query)
		results <- lo.Map(queryResults, func(item QueryResult, index int) QueryResultUI {
			return item.ToUI()
		})
		counter.Add(-1)
		if counter.Load() == 0 {
			done <- true
		}
	}, func() {
		counter.Add(-1)
		if counter.Load() == 0 {
			done <- true
		}
	})
}

func (m *Manager) translatePlugin(ctx context.Context, pluginInstance *Instance, key string) string {
	if !strings.HasPrefix(key, "i18n:") {
		return key
	}

	if pluginInstance.IsSystemPlugin {
		return i18n.GetI18nManager().TranslateWox(ctx, key)
	} else {
		return i18n.GetI18nManager().TranslatePlugin(ctx, key, pluginInstance.PluginDirectory)
	}
}

func (m *Manager) GetUI() share.UI {
	return m.ui
}

func (m *Manager) NewQuery(ctx context.Context, changedQuery share.ChangedQuery) (Query, error) {
	if changedQuery.QueryType == QueryTypeInput {
		newQuery := changedQuery.QueryText
		woxSetting := setting.GetSettingManager().GetWoxSetting(ctx)
		if len(woxSetting.QueryShortcuts) > 0 {
			originQuery := changedQuery.QueryText
			expandedQuery := m.expandQueryShortcut(ctx, changedQuery.QueryText, woxSetting.QueryShortcuts)
			if originQuery != expandedQuery {
				logger.Info(ctx, fmt.Sprintf("expand query shortcut: %s -> %s", originQuery, changedQuery))
				newQuery = expandedQuery
			}
		}
		return newQueryInputWithPlugins(newQuery, GetPluginManager().GetPluginInstances()), nil
	}

	if changedQuery.QueryType == QueryTypeSelection {
		return Query{
			Type:      QueryTypeSelection,
			Selection: changedQuery.QuerySelection,
		}, nil
	}

	return Query{}, errors.New("invalid query type")
}

func (m *Manager) expandQueryShortcut(ctx context.Context, query string, queryShorts []setting.QueryShortcut) (newQuery string) {
	newQuery = query

	//sort query shorts by shortcut length, we will expand the longest shortcut first
	slices.SortFunc(queryShorts, func(i, j setting.QueryShortcut) int {
		return len(j.Shortcut) - len(i.Shortcut)
	})

	for _, shortcut := range queryShorts {
		if strings.HasPrefix(query, shortcut.Shortcut) {
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

func (m *Manager) ExecuteAction(ctx context.Context, resultId string, actionId string) {
	resultCache, found := m.resultCache.Load(resultId)
	if !found {
		logger.Error(ctx, fmt.Sprintf("result cache not found for result id: %s", resultId))
		return
	}
	action, exist := resultCache.Actions.Load(actionId)
	if !exist {
		logger.Error(ctx, fmt.Sprintf("action not found for result id: %s, action id: %s", resultId, actionId))
		return
	}

	action(ActionContext{
		ContextData: resultCache.ContextData,
	})

	setting.GetSettingManager().AddActionedResult(ctx, resultCache.PluginInstance.Metadata.Id, resultCache.ResultTitle, resultCache.ResultSubTitle)
}

func (m *Manager) ExecuteRefresh(ctx context.Context, resultId string, refreshableResult RefreshableResult) (RefreshableResult, error) {
	resultCache, found := m.resultCache.Load(resultId)
	if !found {
		logger.Error(ctx, fmt.Sprintf("result cache not found for result id: %s", resultId))
		return refreshableResult, errors.New("result cache not found")
	}

	newResult := resultCache.Refresh(refreshableResult)
	newResult = m.PolishRefreshableResult(ctx, resultCache.PluginInstance, newResult)

	return newResult, nil
}

func (m *Manager) GetResultPreview(ctx context.Context, resultId string) (WoxPreview, error) {
	resultCache, found := m.resultCache.Load(resultId)
	if !found {
		logger.Error(ctx, fmt.Sprintf("result cache not found for result id: %s", resultId))
		return WoxPreview{}, errors.New("result cache not found")
	}

	return resultCache.Preview, nil
}

func (m *Manager) ReplaceQueryVariable(ctx context.Context, query string) string {
	if strings.Contains(query, QueryVariableSelectedText) {
		selection, selectedErr := util.GetSelected()
		if selectedErr != nil {
			logger.Error(ctx, fmt.Sprintf("failed to get selected text: %s", selectedErr.Error()))
		} else {
			if selection.Type == util.SelectionTypeText {
				query = strings.ReplaceAll(query, QueryVariableSelectedText, selection.Text)
			} else {
				logger.Error(ctx, fmt.Sprintf("selected data is not text, type: %s", selection.Type))
			}
		}
	}

	return query
}
