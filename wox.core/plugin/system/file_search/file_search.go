package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"wox/common"
	"wox/plugin"
	"wox/setting/definition"
	"wox/setting/validator"
	"wox/util"
	"wox/util/fileicon"
	"wox/util/filesearch"
	"wox/util/nativecontextmenu"
	"wox/util/permission"
	"wox/util/shell"
	"wox/util/trash"
)

var fileIcon = common.PluginFileIcon

const fileRootsSettingKey = "roots"
const fileSearchToolbarMsgID = "file-search-status"

const (
	slowFileSearchQueryThresholdMs int64 = 40
	slowFileSearchStageThresholdMs int64 = 15
)

type fileRootSetting struct {
	Path string `json:"Path"`
}

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &FileSearchPlugin{})
}

type FileSearchPlugin struct {
	api                     plugin.API
	engine                  *filesearch.Engine
	unsubscribeStatusChange func()
}

type fileSearchQueryDiagnostics struct {
	toolbarElapsedMs int64
	searchElapsedMs  int64
	buildElapsedMs   int64
	statElapsedMs    int64
	statCount        int
	statMissCount    int
	directoryCount   int
	thumbnailCount   int
}

func (c *FileSearchPlugin) GetMetadata() plugin.Metadata {
	return plugin.Metadata{
		Id:            "979d6363-025a-4f51-88d3-0b04e9dc56bf",
		Name:          "i18n:plugin_file_plugin_name",
		Author:        "Wox Launcher",
		Website:       "https://github.com/Wox-launcher/Wox",
		Version:       "1.0.0",
		MinWoxVersion: "2.0.0",
		Runtime:       "Go",
		Description:   "i18n:plugin_file_plugin_description",
		Icon:          fileIcon.String(),
		Entry:         "",
		TriggerKeywords: []string{
			"f",
		},
		SupportedOS: []string{
			"Windows",
			"Macos",
			"Linux",
		},
		SettingDefinitions: definition.PluginSettingDefinitions{
			{
				Type: definition.PluginSettingDefinitionTypeTable,
				Value: &definition.PluginSettingValueTable{
					Key:          fileRootsSettingKey,
					DefaultValue: "[]",
					Title:        "i18n:plugin_file_setting_roots_title",
					Tooltip:      "i18n:plugin_file_setting_roots_tooltip",
					Columns: []definition.PluginSettingValueTableColumn{
						{
							Key:   "Path",
							Label: "i18n:plugin_file_setting_root_path",
							Type:  definition.PluginSettingValueTableColumnTypeDirPath,
							Validators: []validator.PluginSettingValidator{
								{
									Type:  validator.PluginSettingValidatorTypeNotEmpty,
									Value: &validator.PluginSettingValidatorNotEmpty{},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (c *FileSearchPlugin) Init(ctx context.Context, initParams plugin.InitParams) {
	c.api = initParams.API

	engine, initErr := filesearch.NewEngineWithOptions(ctx, filesearch.EngineOptions{
		Policy: newFileSearchIndexPolicy().toFilesearchPolicy(),
	})
	if initErr != nil {
		c.api.Log(ctx, plugin.LogLevelError, initErr.Error())
		return
	}
	c.engine = engine
	c.api.Log(ctx, plugin.LogLevelInfo, "File search engine initialized")
	c.unsubscribeStatusChange = c.engine.OnStatusChanged(func(status filesearch.StatusSnapshot) {
		c.handleStatusChanged(status)
	})

	// Sync toolbar state once when the session enters file-search query mode because
	// the previous per-keystroke refresh forced a synchronous UI round-trip on every
	// Query() call even though later status changes already arrive through events.
	// Enter-time sync keeps the initial state correct and lets inactive sessions rely
	// on manager-side ignore behavior instead of blocking every search.
	c.api.OnEnterPluginQuery(ctx, func(ctx context.Context) {
		c.syncToolbarMsg(ctx, false)
	})

	c.syncUserRoots(ctx)

	c.api.OnSettingChanged(ctx, func(callbackCtx context.Context, key string, value string) {
		if key != fileRootsSettingKey {
			return
		}
		c.syncUserRoots(callbackCtx)
	})

	c.api.OnUnload(ctx, func(ctx context.Context) {
		if c.unsubscribeStatusChange != nil {
			c.unsubscribeStatusChange()
			c.unsubscribeStatusChange = nil
		}
		if c.engine != nil {
			_ = c.engine.Close()
		}
	})
}

func (c *FileSearchPlugin) Query(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	queryStartedAt := util.GetSystemTimestamp()
	diagnostics := fileSearchQueryDiagnostics{}

	// if query is empty, return empty result
	if query.Search == "" {
		return []plugin.QueryResult{}
	}

	if c.engine == nil {
		return []plugin.QueryResult{}
	}

	searchStartedAt := util.GetSystemTimestamp()
	results, err := c.engine.SearchOnce(ctx, filesearch.SearchQuery{Raw: query.Search}, 100)
	diagnostics.searchElapsedMs = util.GetSystemTimestamp() - searchStartedAt
	if err != nil {
		c.logQueryDiagnostics(ctx, query.Search, diagnostics, 0, util.GetSystemTimestamp()-queryStartedAt)
		c.api.Log(ctx, plugin.LogLevelError, err.Error())
		c.api.Notify(ctx, err.Error())
		return []plugin.QueryResult{}
	}

	// Split result-materialization timing out from engine search timing because the
	// previous logs stopped at SearchOnce and could not show when os.Stat/icon setup
	// made the plugin itself look slow even though the index lookup was fast.
	buildStartedAt := util.GetSystemTimestamp()
	// Cache file-type icons per extension inside one query because the previous
	// per-result file-icon conversion retried embedded-icon extraction for every
	// source file path, which turned an 8ms indexed search into a much slower
	// end-to-end query even though most files only need their shared type icon.
	fileTypeIcons := map[string]common.WoxImage{}
	queryResults := make([]plugin.QueryResult, 0, len(results))
	for _, item := range results {
		icon := resolveFileSearchResultIcon(ctx, item, fileTypeIcons, &diagnostics)

		queryResults = append(queryResults, plugin.QueryResult{
			Title:    item.Name,
			SubTitle: item.Path,
			Icon:     icon,
			Actions: []plugin.QueryResultAction{
				{
					Name: "i18n:plugin_file_open",
					Icon: common.PreviewIcon,
					Action: func(ctx context.Context, actionContext plugin.ActionContext) {
						shell.Open(item.Path)
					},
				},
				{
					Name: "i18n:plugin_file_open_containing_folder",
					Icon: common.OpenContainingFolderIcon,
					Action: func(ctx context.Context, actionContext plugin.ActionContext) {
						shell.OpenFileInFolder(item.Path)
					},
					Hotkey: "ctrl+enter",
				},
				{
					Name: "i18n:plugin_clipboard_delete",
					Icon: common.TrashIcon,
					Action: func(ctx context.Context, actionContext plugin.ActionContext) {
						err := trash.MoveToTrash(item.Path)
						if err != nil {
							c.api.Log(ctx, plugin.LogLevelError, err.Error())
							c.api.Notify(ctx, err.Error())
							return
						}
					},
				},
				{
					Name: "i18n:plugin_file_show_context_menu",
					Icon: common.PluginMenusIcon,
					Action: func(ctx context.Context, actionContext plugin.ActionContext) {
						c.api.Log(ctx, plugin.LogLevelInfo, "Showing context menu for: "+item.Path)
						err := nativecontextmenu.ShowContextMenu(item.Path)
						if err != nil {
							c.api.Log(ctx, plugin.LogLevelError, err.Error())
							c.api.Notify(ctx, err.Error())
						}
					},
					Hotkey:                 "ctrl+m",
					PreventHideAfterAction: true,
				},
			},
		})
	}
	diagnostics.buildElapsedMs = util.GetSystemTimestamp() - buildStartedAt
	c.logQueryDiagnostics(ctx, query.Search, diagnostics, len(queryResults), util.GetSystemTimestamp()-queryStartedAt)

	return queryResults
}

func resolveFileSearchResultIcon(ctx context.Context, result filesearch.SearchResult, fileTypeIcons map[string]common.WoxImage, diagnostics *fileSearchQueryDiagnostics) common.WoxImage {
	if result.IsDir {
		diagnostics.directoryCount++
		return common.FolderIcon
	}

	if shouldUseFileSearchImageThumbnail(result.Path) {
		diagnostics.thumbnailCount++
		// Trust indexed metadata for regular files because the old per-result os.Stat
		// spent several milliseconds confirming directory state that the scanner had
		// already stored. Keep a thumbnail existence check only for image paths so UI
		// does not try to render a deleted file after the index falls briefly behind.
		statStartedAt := util.GetSystemTimestamp()
		_, statErr := os.Stat(result.Path)
		diagnostics.statElapsedMs += util.GetSystemTimestamp() - statStartedAt
		diagnostics.statCount++
		if statErr == nil {
			return common.NewWoxImageAbsolutePath(result.Path)
		}
		diagnostics.statMissCount++
	}

	// Resolve regular files to a cached type icon here because letting manager-side
	// icon conversion inspect every file path forces repeated embedded-icon probes.
	// That fallback work was the main reason logs showed 30ms+ end-to-end latency
	// even when file search itself had already finished within the single-digit budget.
	extension := strings.ToLower(strings.TrimSpace(filepath.Ext(result.Path)))
	if cachedIcon, ok := fileTypeIcons[extension]; ok {
		return cachedIcon
	}

	iconPath, err := fileicon.GetFileTypeIcon(ctx, extension)
	if err == nil && strings.TrimSpace(iconPath) != "" {
		icon := common.NewWoxImageAbsolutePath(iconPath)
		fileTypeIcons[extension] = icon
		return icon
	}

	return common.NewWoxImageFileIcon(result.Path)
}

func shouldUseFileSearchImageThumbnail(filePath string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(filePath))) {
	case ".avif", ".bmp", ".gif", ".heic", ".heif", ".ico", ".jpeg", ".jpg", ".png", ".svg", ".tif", ".tiff", ".webp":
		return true
	default:
		return false
	}
}

func (c *FileSearchPlugin) syncUserRoots(ctx context.Context) {
	if c.engine == nil {
		return
	}

	effectiveRoots := c.getEffectiveRootPaths(ctx)
	c.api.Log(ctx, plugin.LogLevelInfo, fmt.Sprintf("Syncing file search roots: %d roots", len(effectiveRoots)))
	if err := c.engine.SyncUserRoots(ctx, effectiveRoots); err != nil {
		c.api.Log(ctx, plugin.LogLevelError, "Failed to sync file search roots: "+err.Error())
	}
}

func (c *FileSearchPlugin) getEffectiveRootPaths(ctx context.Context) []string {
	paths := append(c.defaultRootPaths(), c.getConfiguredRootPaths(ctx)...)

	uniquePaths := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, path := range paths {
		cleaned := filepath.Clean(strings.TrimSpace(path))
		if cleaned == "." || cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		uniquePaths = append(uniquePaths, cleaned)
	}

	return uniquePaths
}

func (c *FileSearchPlugin) defaultRootPaths() []string {
	// Integration tests provide explicit roots and should not inherit the
	// developer machine's personal folders, which can keep the scanner busy
	// and make file search assertions race with unrelated indexing work.
	if util.IsTestMode() {
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	candidates := []string{
		filepath.Join(homeDir, "Desktop"),
		filepath.Join(homeDir, "Documents"),
		filepath.Join(homeDir, "Downloads"),
		filepath.Join(homeDir, "Pictures"),
	}

	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			paths = append(paths, candidate)
		}
	}

	return paths
}

func (c *FileSearchPlugin) getConfiguredRootPaths(ctx context.Context) []string {
	raw := strings.TrimSpace(c.api.GetSetting(ctx, fileRootsSettingKey))
	if raw == "" {
		return nil
	}

	var roots []fileRootSetting
	if err := json.Unmarshal([]byte(raw), &roots); err != nil {
		c.api.Log(ctx, plugin.LogLevelWarning, "Failed to parse file search roots setting: "+err.Error())
		return nil
	}

	paths := make([]string, 0, len(roots))
	for _, root := range roots {
		if path := strings.TrimSpace(root.Path); path != "" {
			paths = append(paths, path)
		}
	}

	return paths
}

func (c *FileSearchPlugin) syncToolbarMsg(ctx context.Context, includeReady bool) {
	if c.engine == nil {
		c.api.ClearToolbarMsg(ctx, fileSearchToolbarMsgID)
		return
	}

	status, err := c.engine.GetStatus(ctx)
	if err != nil {
		c.api.Log(ctx, plugin.LogLevelWarning, "Failed to load file search status: "+err.Error())
		return
	}

	c.syncToolbarMsgWithStatus(ctx, status, includeReady)
}

func (c *FileSearchPlugin) syncToolbarMsgWithStatus(ctx context.Context, status filesearch.StatusSnapshot, includeReady bool) {
	toolbarMsg, found := c.buildToolbarMsgFromStatus(ctx, status, includeReady)
	if !found {
		c.api.ClearToolbarMsg(ctx, fileSearchToolbarMsgID)
		return
	}

	c.api.ShowToolbarMsg(ctx, toolbarMsg)
}

func (c *FileSearchPlugin) buildToolbarMsgFromStatus(ctx context.Context, status filesearch.StatusSnapshot, includeReady bool) (plugin.ToolbarMsg, bool) {
	if !includeReady && !status.IsIndexing && status.ErrorRootCount == 0 {
		return plugin.ToolbarMsg{}, false
	}

	c.api.Log(ctx, plugin.LogLevelDebug, fmt.Sprintf(
		"File search status: roots=%d preparing=%d scanning=%d syncing=%d writing=%d finalizing=%d errors=%d active=%s progress=%d/%d root=%d/%d dirs=%d/%d items=%d/%d pending=%d/%d discovered=%d initial=%v",
		status.RootCount,
		status.PreparingRootCount,
		status.ScanningRootCount,
		status.SyncingRootCount,
		status.WritingRootCount,
		status.FinalizingRootCount,
		status.ErrorRootCount,
		status.ActiveRootStatus,
		status.ActiveProgressCurrent,
		status.ActiveProgressTotal,
		status.ActiveRootIndex,
		status.ActiveRootTotal,
		status.ActiveDirectoryIndex,
		status.ActiveDirectoryTotal,
		status.ActiveItemCurrent,
		status.ActiveItemTotal,
		status.PendingDirtyRootCount,
		status.PendingDirtyPathCount,
		status.ActiveDiscoveredCount,
		status.IsInitialIndexing,
	))

	title := c.api.GetTranslation(ctx, "plugin_file_status_error")
	icon := common.PermissionIcon
	progress := (*int)(nil)
	indeterminate := false
	hasPermissionError := util.IsMacOS() && isFileAccessPermissionError(status.LastError)
	if status.ActiveRootStatus == filesearch.RootStatusPreparing {
		title = c.buildPreparingToolbarTitle(ctx, status)
		icon = fileIcon
		indeterminate = true
	} else if status.ActiveRootStatus == filesearch.RootStatusSyncing {
		title = c.buildSyncingToolbarTitle(ctx, status)
		icon = fileIcon
		indeterminate = true
	} else if status.ActiveRootStatus == filesearch.RootStatusWriting {
		title = c.api.GetTranslation(ctx, "plugin_file_status_writing")
		icon = fileIcon
		if progressValue, ok := resolveToolbarProgressPercent(status.ActiveProgressCurrent, status.ActiveProgressTotal); ok {
			progress = &progressValue
			title = fmt.Sprintf("%s %d%%", title, progressValue)
		} else {
			indeterminate = true
		}
	} else if status.ActiveRootStatus == filesearch.RootStatusFinalizing {
		title = c.api.GetTranslation(ctx, "plugin_file_status_finalizing")
		icon = fileIcon
		indeterminate = true
	} else if status.ActiveRootStatus == filesearch.RootStatusScanning {
		title = c.buildScanningToolbarTitle(ctx, status)
		icon = fileIcon
		if progressValue, ok := resolveToolbarProgressPercent(status.ActiveItemCurrent, status.ActiveItemTotal); ok {
			progress = &progressValue
		} else {
			indeterminate = true
		}
	} else if status.IsIndexing {
		title = c.api.GetTranslation(ctx, "plugin_file_status_indexing")
		icon = fileIcon
		indeterminate = true
	} else if hasPermissionError {
		title = c.api.GetTranslation(ctx, "plugin_file_status_permission")
	} else if status.ErrorRootCount == 0 {
		return plugin.ToolbarMsg{}, false
	}

	return plugin.ToolbarMsg{
		Id:            fileSearchToolbarMsgID,
		Title:         title,
		Icon:          icon,
		Progress:      progress,
		Indeterminate: indeterminate,
		Actions:       c.toolbarMsgActions(ctx, hasPermissionError),
	}, true
}

func (c *FileSearchPlugin) handleStatusChanged(status filesearch.StatusSnapshot) {
	c.syncToolbarMsgWithStatus(util.NewTraceContext(), status, false)
}

func (c *FileSearchPlugin) logQueryDiagnostics(ctx context.Context, rawQuery string, diagnostics fileSearchQueryDiagnostics, resultCount int, totalElapsedMs int64) {
	msg := fmt.Sprintf(
		"file_search query diagnostics: query=%q total=%dms toolbar=%dms search=%dms build=%dms stat=%dms stat_calls=%d stat_miss=%d results=%d dirs=%d thumbnails=%d",
		rawQuery,
		totalElapsedMs,
		diagnostics.toolbarElapsedMs,
		diagnostics.searchElapsedMs,
		diagnostics.buildElapsedMs,
		diagnostics.statElapsedMs,
		diagnostics.statCount,
		diagnostics.statMissCount,
		resultCount,
		diagnostics.directoryCount,
		diagnostics.thumbnailCount,
	)

	if totalElapsedMs >= slowFileSearchQueryThresholdMs ||
		diagnostics.searchElapsedMs >= slowFileSearchStageThresholdMs ||
		diagnostics.buildElapsedMs >= slowFileSearchStageThresholdMs ||
		diagnostics.statElapsedMs >= slowFileSearchStageThresholdMs {
		c.api.Log(ctx, plugin.LogLevelInfo, "slow "+msg)
		return
	}

	c.api.Log(ctx, plugin.LogLevelDebug, msg)
}

func (c *FileSearchPlugin) buildPreparingToolbarTitle(ctx context.Context, status filesearch.StatusSnapshot) string {
	if status.ActiveDiscoveredCount <= 0 {
		return c.api.GetTranslation(ctx, "plugin_file_status_preparing")
	}

	return fmt.Sprintf(
		c.api.GetTranslation(ctx, "plugin_file_status_preparing_progress"),
		status.ActiveDiscoveredCount,
	)
}

func (c *FileSearchPlugin) buildScanningToolbarTitle(ctx context.Context, status filesearch.StatusSnapshot) string {
	if status.ActiveDirectoryTotal <= 0 || status.ActiveItemTotal <= 0 {
		return c.api.GetTranslation(ctx, "plugin_file_status_indexing")
	}

	return fmt.Sprintf(
		c.api.GetTranslation(ctx, "plugin_file_status_scanning_progress"),
		status.ActiveDirectoryIndex,
		status.ActiveDirectoryTotal,
		status.ActiveItemCurrent,
		status.ActiveItemTotal,
	)
}

func (c *FileSearchPlugin) buildSyncingToolbarTitle(ctx context.Context, status filesearch.StatusSnapshot) string {
	if status.PendingDirtyRootCount <= 0 && status.PendingDirtyPathCount <= 0 {
		return c.api.GetTranslation(ctx, "plugin_file_status_syncing")
	}

	return fmt.Sprintf(
		c.api.GetTranslation(ctx, "plugin_file_status_syncing_progress"),
		status.PendingDirtyRootCount,
		status.PendingDirtyPathCount,
	)
}

func resolveToolbarProgressPercent(current int64, total int64) (int, bool) {
	if total <= 0 {
		return 0, false
	}

	progressValue := int((current * 100) / total)
	if progressValue < 0 {
		progressValue = 0
	}
	if progressValue > 100 {
		progressValue = 100
	}

	return progressValue, true
}

func (c *FileSearchPlugin) toolbarMsgActions(ctx context.Context, hasPermissionError bool) []plugin.ToolbarMsgAction {
	if !hasPermissionError || !util.IsMacOS() {
		return nil
	}

	return []plugin.ToolbarMsgAction{
		{
			Name:   "i18n:plugin_file_status_open_privacy_settings",
			Icon:   common.PermissionIcon,
			Hotkey: "ctrl+enter",
			Action: func(ctx context.Context, actionContext plugin.ToolbarMsgActionContext) {
				permission.OpenPrivacySecuritySettings(ctx)
			},
		},
	}
}

func isFileAccessPermissionError(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return false
	}

	return strings.Contains(message, "operation not permitted") || strings.Contains(message, "permission denied")
}
