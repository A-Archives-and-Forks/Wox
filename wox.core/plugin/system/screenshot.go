package system

import (
	"context"
	"fmt"
	"os"
	"time"
	"wox/common"
	"wox/plugin"
	"wox/util"
	"wox/util/screenshot"
)

var screenshotPluginIcon = common.PluginSelectionIcon

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &ScreenshotPlugin{})
}

type ScreenshotPlugin struct {
	api plugin.API
}

func (s *ScreenshotPlugin) GetMetadata() plugin.Metadata {
	return plugin.Metadata{
		Id:              "fd0c7ddf-00f6-47cf-9483-8a9d0fb92e75",
		Name:            "i18n:plugin_screenshot_plugin_name",
		Author:          "Wox Launcher",
		Website:         "https://github.com/Wox-launcher/Wox",
		Version:         "1.0.0",
		MinWoxVersion:   "2.0.0",
		Runtime:         "Go",
		Description:     "i18n:plugin_screenshot_plugin_description",
		Icon:            screenshotPluginIcon.String(),
		TriggerKeywords: []string{"screenshot", "shot"},
		SupportedOS:     []string{"Windows", "Macos"},
		Features: []plugin.MetadataFeature{
			{
				Name: plugin.MetadataFeatureIgnoreAutoScore,
			},
		},
	}
}

func (s *ScreenshotPlugin) Init(ctx context.Context, initParams plugin.InitParams) {
	s.api = initParams.API
}

func (s *ScreenshotPlugin) Query(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	if query.Type != plugin.QueryTypeInput {
		return nil
	}

	return []plugin.QueryResult{
		{
			Title:    s.getTranslation(ctx, "plugin_screenshot_capture_title", "Capture screen"),
			SubTitle: s.getTranslation(ctx, "plugin_screenshot_capture_subtitle", "Select a region and capture it to clipboard"),
			Icon:     screenshotPluginIcon,
			Score:    1000,
			Actions: []plugin.QueryResultAction{
				{
					Name:      s.getTranslation(ctx, "plugin_screenshot_capture_action", "Start screenshot"),
					Icon:      common.ExecuteRunIcon,
					IsDefault: true,
					Action: func(actionCtx context.Context, actionContext plugin.ActionContext) {
						if s.api != nil {
							s.api.HideApp(actionCtx)
						}

						util.Go(actionCtx, "start screenshot session", func() {
							time.Sleep(150 * time.Millisecond)

							result, err := screenshot.NewManager().StartSession(actionCtx, screenshot.StartOptions{
								Mode:            screenshot.CaptureModeRegion,
								CopyToClipboard: true,
								SaveToFile:      true,
								TempDir:         os.TempDir(),
							})
							if err != nil {
								if err == screenshot.ErrSessionCancelled {
									return
								}

								if s.api != nil {
									s.api.Notify(actionCtx, fmt.Sprintf("%s: %s", s.getTranslation(actionCtx, "plugin_screenshot_capture_failed", "Screenshot failed"), err.Error()))
									s.api.Log(actionCtx, plugin.LogLevelError, fmt.Sprintf("screenshot plugin: start session failed: %s", err.Error()))
								}
								return
							}

							if s.api != nil && result != nil {
								s.api.Notify(actionCtx, fmt.Sprintf("%s: %s", s.getTranslation(actionCtx, "plugin_screenshot_capture_success", "Screenshot saved"), result.FilePath))
							}
						})
					},
				},
			},
		},
	}
}

func (s *ScreenshotPlugin) getTranslation(ctx context.Context, key string, fallback string) string {
	if s.api == nil {
		return fallback
	}

	value := s.api.GetTranslation(ctx, key)
	if value == "" || value == key {
		return fallback
	}

	return value
}
