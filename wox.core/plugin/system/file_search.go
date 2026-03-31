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
	"wox/util/filesearch"
	"wox/util/nativecontextmenu"
	"wox/util/permission"
	"wox/util/shell"
	"wox/util/trash"

	"github.com/samber/lo"
)

var fileIcon = common.PluginFileIcon

const fileRootsSettingKey = "roots"
const fileSearchToolbarMsgID = "file-search-status"

type fileRootSetting struct {
	Path string `json:"Path"`
}

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &FileSearchPlugin{})
}

type FileSearchPlugin struct {
	api    plugin.API
	engine *filesearch.Engine
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
		Features: []plugin.MetadataFeature{
			{
				Name: plugin.MetadataFeatureDebounce,
				Params: map[string]any{
					"IntervalMs": 200,
				},
			},
		},
	}
}

func (c *FileSearchPlugin) Init(ctx context.Context, initParams plugin.InitParams) {
	c.api = initParams.API

	engine, initErr := filesearch.NewEngine(ctx)
	if initErr != nil {
		c.api.Log(ctx, plugin.LogLevelError, initErr.Error())
		return
	}
	c.engine = engine
	c.api.Log(ctx, plugin.LogLevelInfo, "File search engine initialized")

	c.syncUserRoots(ctx)

	c.api.OnEnterPluginQuery(ctx, func(callbackCtx context.Context) {
		c.syncToolbarMsg(callbackCtx, true)
	})

	c.api.OnSettingChanged(ctx, func(callbackCtx context.Context, key string, value string) {
		if key != fileRootsSettingKey {
			return
		}
		c.syncUserRoots(callbackCtx)
	})

	c.api.OnUnload(ctx, func(ctx context.Context) {
		if c.engine != nil {
			_ = c.engine.Close()
		}
	})
}

func (c *FileSearchPlugin) Query(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	c.syncToolbarMsg(ctx, query.Search == "")

	// if query is empty, return empty result
	if query.Search == "" {
		return []plugin.QueryResult{}
	}

	if c.engine == nil {
		return []plugin.QueryResult{}
	}

	results, err := c.engine.SearchOnce(ctx, filesearch.SearchQuery{Raw: query.Search}, 100)
	if err != nil {
		c.api.Log(ctx, plugin.LogLevelError, err.Error())
		c.api.Notify(ctx, err.Error())
		return []plugin.QueryResult{}
	}

	queryResults := lo.Map(results, func(item filesearch.SearchResult, _ int) plugin.QueryResult {
		icon := fileIcon
		if info, err := os.Stat(item.Path); err == nil {
			if info.IsDir() {
				icon = common.FolderIcon
			} else {
				icon = common.NewWoxImageFileIcon(item.Path)
			}
		}

		return plugin.QueryResult{
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
		}
	})

	return queryResults
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
	status, found := c.buildToolbarMsg(ctx, includeReady)
	if !found {
		c.api.ClearToolbarMsg(ctx, fileSearchToolbarMsgID)
		return
	}

	c.api.ShowToolbarMsg(ctx, status)
}

func (c *FileSearchPlugin) buildToolbarMsg(ctx context.Context, includeReady bool) (plugin.ToolbarMsg, bool) {
	if c.engine == nil {
		return plugin.ToolbarMsg{}, false
	}

	status, err := c.engine.GetStatus(ctx)
	if err != nil {
		c.api.Log(ctx, plugin.LogLevelWarning, "Failed to load file search status: "+err.Error())
		return plugin.ToolbarMsg{}, false
	}

	if !includeReady && !status.IsIndexing && status.ErrorRootCount == 0 {
		return plugin.ToolbarMsg{}, false
	}

	c.api.Log(ctx, plugin.LogLevelDebug, fmt.Sprintf(
		"File search status: roots=%d scanning=%d errors=%d progress=%d/%d initial=%v",
		status.RootCount,
		status.ScanningRootCount,
		status.ErrorRootCount,
		status.ProgressCurrent,
		status.ProgressTotal,
		status.IsInitialIndexing,
	))

	title := c.api.GetTranslation(ctx, "plugin_file_status_error")
	icon := common.PermissionIcon
	progress := (*int)(nil)
	indeterminate := false
	hasPermissionError := util.IsMacOS() && isFileAccessPermissionError(status.LastError)
	if status.IsIndexing {
		title = c.api.GetTranslation(ctx, "plugin_file_status_indexing")
		icon = fileIcon
		if status.ProgressTotal > 0 {
			progressValue := int((status.ProgressCurrent * 100) / status.ProgressTotal)
			if progressValue < 0 {
				progressValue = 0
			}
			if progressValue > 100 {
				progressValue = 100
			}
			progress = &progressValue
		} else {
			indeterminate = true
		}
	} else if hasPermissionError {
		title = c.api.GetTranslation(ctx, "plugin_file_status_permission")
	} else if status.ErrorRootCount == 0 {
		title = c.api.GetTranslation(ctx, "plugin_file_status_ready")
		icon = fileIcon
	}

	return plugin.ToolbarMsg{
		Id:            fileSearchToolbarMsgID,
		Scope:         plugin.ToolbarMsgScopePlugin,
		Title:         title,
		Icon:          icon,
		Progress:      progress,
		Indeterminate: indeterminate,
		Actions:       c.toolbarMsgActions(ctx, hasPermissionError),
	}, true
}

func (c *FileSearchPlugin) toolbarMsgActions(ctx context.Context, hasPermissionError bool) []plugin.ToolbarMsgAction {
	if !hasPermissionError || !util.IsMacOS() {
		return nil
	}

	return []plugin.ToolbarMsgAction{
		{
			Name: "i18n:plugin_file_status_open_privacy_settings",
			Icon: common.PermissionIcon,
			Action: func(ctx context.Context, actionContext plugin.ToolbarMsgActionContext) {
				permission.OpenPrivacySecuritySettings(ctx)
			},
			PreventHideAfterAction: true,
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
