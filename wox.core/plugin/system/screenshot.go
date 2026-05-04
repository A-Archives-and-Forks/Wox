package system

import (
	"context"
	"fmt"
	"image"
	_ "image/png" // Register PNG header decoding for file-backed pinned screenshots.
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"wox/common"
	"wox/plugin"
	"wox/util"
	"wox/util/clipboard"
	"wox/util/overlay"
	"wox/util/shell"

	"github.com/disintegration/imaging"
)

var screenshotIcon = common.PluginScreenshotIcon
var screenshotCommandNew = "new"
var screenshotHistoryPreviewWidth = 400
var screenshotHistoryIconWidth = 40
var screenshotPinnedOverlayPrefix = "wox_screenshot_pin_"

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &ScreenshotPlugin{})
}

type ScreenshotPlugin struct {
	api        plugin.API
	thumbnailM sync.Mutex
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

	// Screenshot history thumbnails are warmed during plugin startup so the first user query does
	// not pay the old cost of decoding every original screenshot through the generic icon pipeline.
	util.Go(ctx, "warm screenshot history thumbnails", func() {
		p.warmScreenshotHistoryThumbnails(ctx)
	})
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

func (p *ScreenshotPlugin) warmScreenshotHistoryThumbnails(ctx context.Context) {
	items, err := p.listScreenshotHistory()
	if err != nil {
		p.api.Log(ctx, plugin.LogLevelWarning, fmt.Sprintf("failed to warm screenshot history thumbnails: %s", err.Error()))
		return
	}

	for _, item := range items {
		if err := p.ensureScreenshotHistoryThumbnails(ctx, item); err != nil {
			p.api.Log(ctx, plugin.LogLevelWarning, fmt.Sprintf("failed to warm screenshot history thumbnail: path=%s err=%s", item.path, err.Error()))
		}
	}
}

func (p *ScreenshotPlugin) ensureScreenshotHistoryThumbnailsForPath(ctx context.Context, screenshotPath string) error {
	item, err := p.screenshotHistoryItemFromPath(screenshotPath)
	if err != nil {
		return err
	}

	return p.ensureScreenshotHistoryThumbnails(ctx, item)
}

func (p *ScreenshotPlugin) screenshotHistoryItemFromPath(screenshotPath string) (screenshotHistoryItem, error) {
	info, err := os.Stat(screenshotPath)
	if err != nil {
		return screenshotHistoryItem{}, fmt.Errorf("failed to read screenshot file info: %w", err)
	}
	if info.IsDir() {
		return screenshotHistoryItem{}, fmt.Errorf("screenshot path is a directory")
	}
	if !strings.EqualFold(filepath.Ext(screenshotPath), ".png") {
		return screenshotHistoryItem{}, fmt.Errorf("screenshot path is not a png")
	}
	if info.Size() == 0 {
		return screenshotHistoryItem{}, fmt.Errorf("screenshot file is empty")
	}

	return screenshotHistoryItem{
		path:      screenshotPath,
		fileName:  filepath.Base(screenshotPath),
		size:      info.Size(),
		timestamp: info.ModTime().UnixMilli(),
	}, nil
}

func (p *ScreenshotPlugin) ensureScreenshotHistoryThumbnails(ctx context.Context, item screenshotHistoryItem) error {
	previewPath, iconPath := p.screenshotHistoryThumbnailPaths(item)
	if util.IsFileExists(previewPath) && util.IsFileExists(iconPath) {
		p.warmScreenshotHistoryManagerIconCache(ctx, iconPath)
		return nil
	}

	p.thumbnailM.Lock()
	defer p.thumbnailM.Unlock()

	if util.IsFileExists(previewPath) && util.IsFileExists(iconPath) {
		p.warmScreenshotHistoryManagerIconCache(ctx, iconPath)
		return nil
	}
	if err := util.GetLocation().EnsureDirectoryExist(util.GetLocation().GetImageCacheDirectory()); err != nil {
		return fmt.Errorf("failed to ensure image cache directory: %w", err)
	}

	sourceImage, err := imaging.Open(item.path)
	if err != nil {
		return fmt.Errorf("failed to decode screenshot image: %w", err)
	}

	// Screenshot history now owns its thumbnails instead of relying on Manager.ConvertIcon.
	// The old path decoded full-size screenshots during query polishing; generating bounded
	// cache files here moves that work to init/capture time and keeps query latency stable.
	previewImage := imaging.Resize(sourceImage, screenshotHistoryPreviewWidth, 0, imaging.Lanczos)
	if err := imaging.Save(previewImage, previewPath); err != nil {
		return fmt.Errorf("failed to save screenshot preview thumbnail: %w", err)
	}

	iconImage := imaging.Resize(sourceImage, screenshotHistoryIconWidth, 0, imaging.Lanczos)
	if err := imaging.Save(iconImage, iconPath); err != nil {
		return fmt.Errorf("failed to save screenshot icon thumbnail: %w", err)
	}

	p.warmScreenshotHistoryManagerIconCache(ctx, iconPath)
	return nil
}

func (p *ScreenshotPlugin) warmScreenshotHistoryManagerIconCache(ctx context.Context, iconPath string) {
	// Manager.PolishResult always normalizes result icons with ConvertIcon. Running the same
	// conversion on the already-small screenshot icon during warm-up prevents query-time cache
	// generation while keeping the normal UI icon contract unchanged.
	common.ConvertIcon(ctx, common.NewWoxImageAbsolutePath(iconPath), "")
}

func (p *ScreenshotPlugin) getScreenshotHistoryThumbnails(item screenshotHistoryItem) (previewImage common.WoxImage, iconImage common.WoxImage, ok bool) {
	previewPath, iconPath := p.screenshotHistoryThumbnailPaths(item)
	if !util.IsFileExists(previewPath) || !util.IsFileExists(iconPath) {
		return common.WoxImage{}, common.WoxImage{}, false
	}

	return common.NewWoxImageAbsolutePath(previewPath), common.NewWoxImageAbsolutePath(iconPath), true
}

func (p *ScreenshotPlugin) screenshotHistoryThumbnailPaths(item screenshotHistoryItem) (previewPath string, iconPath string) {
	cacheKey := util.Md5([]byte(fmt.Sprintf("%s:%d:%d", item.path, item.size, item.timestamp)))
	cacheDirectory := util.GetLocation().GetImageCacheDirectory()
	return filepath.Join(cacheDirectory, fmt.Sprintf("screenshot_%s_preview.png", cacheKey)),
		filepath.Join(cacheDirectory, fmt.Sprintf("screenshot_%s_icon.png", cacheKey))
}

func (p *ScreenshotPlugin) screenshotHistoryResult(item screenshotHistoryItem) plugin.QueryResult {
	group, groupScore := p.screenshotHistoryGroup(item.timestamp)
	previewImage, iconImage, thumbnailsReady := p.getScreenshotHistoryThumbnails(item)
	if !thumbnailsReady {
		// Query must never generate thumbnails: doing image decode/write work here made the first
		// screenshot search slow. A default icon keeps listing responsive while init/new-capture
		// warm-up finishes; preview can still open the original file on explicit selection.
		previewImage = common.NewWoxImageAbsolutePath(item.path)
		iconImage = screenshotIcon
	}

	return plugin.QueryResult{
		Title:      item.fileName,
		SubTitle:   util.FormatTimestamp(item.timestamp),
		Icon:       iconImage,
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

func (p *ScreenshotPlugin) readScreenshotImageSize(screenshotPath string) (int, int, error) {
	// DecodeConfig reads only the PNG header. It is used only when Flutter did not return a logical
	// selection size, so the file-backed pin path still avoids full image decoding on the common path.
	file, err := os.Open(screenshotPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open pinned screenshot image: %w", err)
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read pinned screenshot image size: %w", err)
	}
	return config.Width, config.Height, nil
}

func (p *ScreenshotPlugin) pinScreenshotToScreen(ctx context.Context, screenshotPath string, selectionRect *common.ScreenshotRect) error {
	// File-backed overlays depend on the PNG remaining readable after Flutter writes it. Validate the
	// path cheaply here so native decode failures do not become silent missing pinned windows.
	info, err := os.Stat(screenshotPath)
	if err != nil {
		return fmt.Errorf("failed to read pinned screenshot file info: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("pinned screenshot path is a directory")
	}
	if info.Size() == 0 {
		return fmt.Errorf("pinned screenshot file is empty")
	}

	width := 0.0
	height := 0.0
	offsetX := 0.0
	offsetY := 0.0
	if selectionRect != nil {
		// The PNG may be device-pixel sized on high-DPI screens, while the overlay API positions and
		// sizes windows in logical desktop coordinates. Use Flutter's selection rect for the pinned
		// window so the image appears at the same desktop size the user selected.
		if selectionRect.Width >= 1 {
			width = selectionRect.Width
		}
		if selectionRect.Height >= 1 {
			height = selectionRect.Height
		}
		offsetX = selectionRect.X
		offsetY = selectionRect.Y
	}
	// File-backed overlays usually get logical size from Flutter, but this header-only fallback keeps
	// older or incomplete capture results usable without returning to the full image decode path.
	if width < 1 || height < 1 {
		pixelWidth, pixelHeight, err := p.readScreenshotImageSize(screenshotPath)
		if err != nil {
			return err
		}
		if width < 1 {
			width = float64(pixelWidth)
		}
		if height < 1 {
			height = float64(pixelHeight)
		}
	}

	name := screenshotPinnedOverlayPrefix + util.Md5([]byte(fmt.Sprintf("%s:%d", screenshotPath, time.Now().UnixNano())))
	overlay.Show(overlay.OverlayOptions{
		Name:          name,
		Title:         "Wox pinned screenshot",
		Icon:          overlay.NewFileIcon(screenshotPath),
		Transparent:   true,
		Movable:       true,
		CloseOnEscape: true,
		Anchor:        overlay.AnchorTopLeft,
		OffsetX:       offsetX,
		OffsetY:       offsetY,
		Width:         width,
		Height:        height,
		IconWidth:     width,
		IconHeight:    height,
	})
	return nil
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
		if err := p.ensureScreenshotHistoryThumbnailsForPath(ctx, result.ScreenshotPath); err != nil {
			// A thumbnail failure should not turn a successful capture into a failed screenshot. The
			// history result will temporarily fall back to the default icon and the next init warm-up
			// can repair the cache.
			p.api.Log(ctx, plugin.LogLevelWarning, fmt.Sprintf("failed to generate screenshot history thumbnails: path=%s err=%s", result.ScreenshotPath, err.Error()))
		}

		if result.PinToScreen {
			// Flutter owns final image composition, but the pinned desktop window belongs in Go because
			// util/overlay is already the native surface abstraction used by core. Branching on the
			// explicit result flag avoids overloading normal clipboard confirmation with pin behavior.
			if err := p.pinScreenshotToScreen(ctx, result.ScreenshotPath, result.LogicalSelectionRect); err != nil {
				p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("failed to pin screenshot: path=%s err=%s", result.ScreenshotPath, err.Error()))
				p.api.Notify(ctx, "plugin_screenshot_pin_failed")
			}
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
