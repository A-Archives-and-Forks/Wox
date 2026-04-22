package system

import (
	"context"
	"fmt"
	"wox/common"
	"wox/plugin"
)

var screenshotIcon = common.PluginScreenshotIcon

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
	return []plugin.QueryResult{
		{
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
		},
	}
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
