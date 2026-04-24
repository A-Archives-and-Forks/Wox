package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"wox/common"
	"wox/plugin"
	"wox/util"
	"wox/util/clipboard"
	"wox/util/shell"

	"github.com/disintegration/imaging"
)

var screenshotIcon = common.PluginScreenshotIcon
var screenshotCommandNew = "new"

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &ScreenshotPlugin{})
}

type ScreenshotPlugin struct {
	api plugin.API
}

func (p *ScreenshotPlugin) GetMetadata() plugin.Metadata {
	return plugin.Metadata{
		Id:            "78fc701b-a87e-4d5f-a7f2-13cbad9f7d1d",
		Name:          "i18n:plugin_screenshot_plugin_name",
		Author:        "Wox Launcher",
		Website:       "https://github.com/Wox-launcher/Wox",
		Version:       "1.0.0",
		MinWoxVersion: "2.0.0",
		Runtime:       "Go",
		Description:   "i18n:plugin_screenshot_plugin_description",
		Icon:          screenshotIcon.String(),
		TriggerKeywords: []string{
			"screenshot",
			"截图",
		},
		Commands: []plugin.MetadataCommand{
			{
				Command:     screenshotCommandNew,
				Description: "i18n:plugin_screenshot_command_new_description",
			},
		},
		SupportedOS: []string{
			"Windows",
			"Macos",
			"Linux",
		},
	}
}

func (p *ScreenshotPlugin) Init(ctx context.Context, initParams plugin.InitParams) {
	p.api = initParams.API
}

func (p *ScreenshotPlugin) Query(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	if query.Command == screenshotCommandNew {
		return []plugin.QueryResult{p.newScreenshotResult()}
	}

	if query.Command != "" {
		return []plugin.QueryResult{}
	}

	// The default screenshot query now lists saved captures instead of starting a capture directly.
	// Starting a new capture moved to the explicit "new" command so history browsing and capture
	// creation do not compete for the same default result.
	results, err := p.queryScreenshotHistory(query)
	if err != nil {
		p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("failed to query screenshot history: %s", err.Error()))
		return []plugin.QueryResult{}
	}

	if len(results) > 0 {
		return results
	}
	if strings.TrimSpace(query.Search) != "" {
		return []plugin.QueryResult{}
	}

	return []plugin.QueryResult{
		{
			Title:    "i18n:plugin_screenshot_history_empty_title",
			SubTitle: "i18n:plugin_screenshot_history_empty_subtitle",
			Icon:     screenshotIcon,
		},
	}
}

type screenshotHistoryItem struct {
	path      string
	fileName  string
	size      int64
	timestamp int64
}

func (p *ScreenshotPlugin) newScreenshotResult() plugin.QueryResult {
	return plugin.QueryResult{
		Title:    "i18n:plugin_screenshot_capture_title",
		SubTitle: "i18n:plugin_screenshot_capture_subtitle",
		Icon:     screenshotIcon,
		Actions: []plugin.QueryResultAction{
			{
				Name:                   "i18n:plugin_screenshot_capture_action",
				IsDefault:              true,
				PreventHideAfterAction: true,
				Action:                 p.captureScreenshot,
			},
		},
	}
}

func (p *ScreenshotPlugin) queryScreenshotHistory(query plugin.Query) ([]plugin.QueryResult, error) {
	items, err := p.listScreenshotHistory()
	if err != nil {
		return nil, err
	}

	results := make([]plugin.QueryResult, 0, len(items))
	search := strings.ToLower(strings.TrimSpace(query.Search))
	for _, item := range items {
		if search != "" && !strings.Contains(strings.ToLower(item.fileName), search) && !strings.Contains(strings.ToLower(util.FormatTimestamp(item.timestamp)), search) {
			continue
		}

		results = append(results, p.screenshotHistoryResult(item))
	}

	return results, nil
}

func (p *ScreenshotPlugin) listScreenshotHistory() ([]screenshotHistoryItem, error) {
	screenshotDirectory := filepath.Join(util.GetLocation().GetWoxDataDirectory(), "screenshots")
	entries, err := os.ReadDir(screenshotDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return []screenshotHistoryItem{}, nil
		}
		return nil, fmt.Errorf("failed to read screenshot directory: %w", err)
	}

	items := make([]screenshotHistoryItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".png") {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("failed to read screenshot file info: %w", infoErr)
		}
		if info.Size() == 0 {
			continue
		}

		// Reusing the existing screenshot export directory keeps the history feature storage-free.
		// The file modification time is the simplest durable ordering signal for captures already
		// written by Flutter, and zero-byte reservation files are skipped above.
		items = append(items, screenshotHistoryItem{
			path:      filepath.Join(screenshotDirectory, entry.Name()),
			fileName:  entry.Name(),
			size:      info.Size(),
			timestamp: info.ModTime().UnixMilli(),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].timestamp > items[j].timestamp
	})

	return items, nil
}

func (p *ScreenshotPlugin) screenshotHistoryResult(item screenshotHistoryItem) plugin.QueryResult {
	group, groupScore := p.screenshotHistoryGroup(item.timestamp)
	previewImage := common.NewWoxImageAbsolutePath(item.path)

	return plugin.QueryResult{
		Title:      item.fileName,
		SubTitle:   util.FormatTimestamp(item.timestamp),
		Icon:       previewImage,
		Group:      group,
		GroupScore: groupScore,
		Preview: plugin.WoxPreview{
			PreviewType: plugin.WoxPreviewTypeImage,
			PreviewData: previewImage.String(),
			PreviewProperties: map[string]string{
				"i18n:plugin_screenshot_history_date": util.FormatTimestamp(item.timestamp),
				"i18n:plugin_screenshot_history_size": p.formatFileSize(item.size),
			},
		},
		Score: item.timestamp,
		Actions: []plugin.QueryResultAction{
			{
				Name:      "i18n:plugin_screenshot_history_copy",
				Icon:      common.CopyIcon,
				IsDefault: true,
				Action: func(ctx context.Context, actionContext plugin.ActionContext) {
					p.copyScreenshotHistoryItem(ctx, item.path)
				},
			},
			{
				Name: "i18n:plugin_screenshot_history_open",
				Icon: common.OpenIcon,
				Action: func(ctx context.Context, actionContext plugin.ActionContext) {
					if err := shell.Open(item.path); err != nil {
						p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("failed to open screenshot history item: path=%s err=%s", item.path, err.Error()))
					}
				},
			},
			{
				Name: "i18n:plugin_screenshot_history_open_folder",
				Icon: common.OpenContainingFolderIcon,
				Action: func(ctx context.Context, actionContext plugin.ActionContext) {
					if err := shell.OpenFileInFolder(item.path); err != nil {
						p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("failed to open screenshot history item folder: path=%s err=%s", item.path, err.Error()))
					}
				},
			},
		},
	}
}

func (p *ScreenshotPlugin) screenshotHistoryGroup(timestamp int64) (string, int64) {
	now := time.Now()
	itemTime := time.UnixMilli(timestamp)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)

	// Screenshot history now owns the default query surface, so grouping by local calendar day keeps
	// older captures browsable without mixing "start new screenshot" into the history list.
	if !itemTime.Before(today) {
		return "i18n:plugin_screenshot_group_today", 90
	}
	if !itemTime.Before(yesterday) {
		return "i18n:plugin_screenshot_group_yesterday", 80
	}

	return "i18n:plugin_screenshot_group_history", 10
}

func (p *ScreenshotPlugin) copyScreenshotHistoryItem(ctx context.Context, screenshotPath string) {
	img, err := imaging.Open(screenshotPath)
	if err != nil {
		p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("failed to decode screenshot history item: path=%s err=%s", screenshotPath, err.Error()))
		return
	}

	if err := clipboard.Write(&clipboard.ImageData{Image: img}); err != nil {
		p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("failed to copy screenshot history item: path=%s err=%s", screenshotPath, err.Error()))
	}
}

func (p *ScreenshotPlugin) formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func (p *ScreenshotPlugin) captureScreenshot(ctx context.Context, actionContext plugin.ActionContext) {
	request := common.DefaultCaptureScreenshotRequest()
	result, err := plugin.GetPluginManager().GetUI().CaptureScreenshot(ctx, request)
	if err != nil {
		// The screenshot session spans Go, Flutter, and the native bridge, so transport failures need a local
		// notification here instead of silently falling through to keep the action predictable for the user.
		p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("capture screenshot request failed: %s", err.Error()))
		p.api.Notify(ctx, "plugin_screenshot_capture_failed")
		return
	}

	switch result.Status {
	case common.CaptureScreenshotStatusCompleted:
		// Screenshot export and clipboard write now complete inside Flutter plus the platform runner.
		// Go treats a completed export as success and only surfaces clipboard warnings separately.
		if result.ScreenshotPath == "" {
			p.api.Log(ctx, plugin.LogLevelError, "screenshot completed without an export path")
			p.api.Notify(ctx, "plugin_screenshot_capture_failed")
			return
		}

		p.api.Notify(ctx, "plugin_screenshot_capture_success")
		if result.ClipboardWarningMessage != "" {
			p.api.Log(ctx, plugin.LogLevelWarning, fmt.Sprintf("screenshot clipboard warning: %s", result.ClipboardWarningMessage))
			p.api.Notify(ctx, "plugin_screenshot_capture_clipboard_warning")
		}
	case common.CaptureScreenshotStatusFailed:
		errText := result.ErrorMessage
		if errText == "" {
			errText = "screenshot session failed"
		}
		p.api.Log(ctx, plugin.LogLevelError, errText)
		p.api.Notify(ctx, "plugin_screenshot_capture_failed")
	case common.CaptureScreenshotStatusCancelled:
		return
	default:
		p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("unexpected screenshot status: %s", result.Status))
		p.api.Notify(ctx, "plugin_screenshot_capture_failed")
	}
}
