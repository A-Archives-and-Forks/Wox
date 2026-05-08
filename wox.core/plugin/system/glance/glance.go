package glance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"wox/common"
	"wox/plugin"
	"wox/util"
)

const systemGlancePluginId = "e3ad9f18-fbbe-4f22-8c1b-8274c751f6e6"

const (
	glancePluginSvg  = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"><path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z" stroke="#8AB4F8" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><circle cx="12" cy="12" r="3" fill="#8AB4F8"/></svg>`
	glanceTimeSvg    = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="8.5" stroke="#8AB4F8" stroke-width="2"/><path d="M12 7v5l3 2" stroke="#8AB4F8" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	glanceDateSvg    = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"><rect x="4" y="5" width="16" height="15" rx="2.5" stroke="#8AB4F8" stroke-width="2"/><path d="M8 3v4M16 3v4M4 10h16" stroke="#8AB4F8" stroke-width="2" stroke-linecap="round"/><path d="M8 14h2M12 14h2M16 14h1M8 17h2M12 17h2" stroke="#8AB4F8" stroke-width="1.8" stroke-linecap="round"/></svg>`
	glanceBatterySvg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"><rect x="3" y="7" width="16" height="10" rx="2" stroke="#8AB4F8" stroke-width="2"/><path d="M21 10v4" stroke="#8AB4F8" stroke-width="2" stroke-linecap="round"/><rect x="6" y="10" width="8" height="4" rx="1" fill="#8AB4F8"/></svg>`
	glancePlugSvg    = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"><path d="M9 3v6M15 3v6M7 9h10v3a5 5 0 0 1-4 4.9V21h-2v-4.1A5 5 0 0 1 7 12V9Z" stroke="#8AB4F8" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`
)

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &GlancePlugin{})
}

type GlancePlugin struct {
	api plugin.API
}

func (p *GlancePlugin) GetMetadata() plugin.Metadata {
	return plugin.Metadata{
		Id:              systemGlancePluginId,
		Name:            "i18n:plugin_glance_plugin_name",
		Author:          "Wox Launcher",
		Website:         "https://github.com/Wox-launcher/Wox",
		Version:         "1.0.0",
		MinWoxVersion:   "2.0.0",
		Runtime:         "Go",
		Description:     "i18n:plugin_glance_plugin_description",
		Icon:            glanceSvgString(glancePluginSvg),
		Entry:           "",
		TriggerKeywords: []string{"glance"},
		SupportedOS:     []string{"Windows", "Macos", "Linux"},
		Glances: []plugin.MetadataGlance{
			{Id: "time", Name: "i18n:plugin_glance_time_name", Description: "i18n:plugin_glance_time_description", Icon: glanceSvgString(glanceTimeSvg), RefreshIntervalMs: 60000},
			{Id: "date", Name: "i18n:plugin_glance_date_name", Description: "i18n:plugin_glance_date_description", Icon: glanceSvgString(glanceDateSvg), RefreshIntervalMs: 60000},
			{Id: "battery", Name: "i18n:plugin_glance_battery_name", Description: "i18n:plugin_glance_battery_description", Icon: glanceSvgString(glanceBatterySvg), RefreshIntervalMs: 60000},
		},
	}
}

func glanceSvgString(svg string) string {
	// Glance icons use inline SVG rather than emoji so every platform renders
	// the same compact glyphs and avoids OS-specific emoji fallback metrics.
	image := common.NewWoxImageSvg(svg)
	return image.String()
}

func (p *GlancePlugin) Init(ctx context.Context, initParams plugin.InitParams) {
	p.api = initParams.API
}

func (p *GlancePlugin) Query(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	return []plugin.QueryResult{}
}

func (p *GlancePlugin) Glance(ctx context.Context, request plugin.GlanceRequest) plugin.GlanceResponse {
	items := make([]plugin.GlanceItem, 0, len(request.Ids))
	for _, id := range request.Ids {
		switch id {
		case "time":
			items = append(items, plugin.GlanceItem{Id: id, Text: time.Now().Format("15:04"), Icon: common.NewWoxImageSvg(glanceTimeSvg)})
		case "date":
			items = append(items, plugin.GlanceItem{Id: id, Text: time.Now().Format("Mon 01/02"), Icon: common.NewWoxImageSvg(glanceDateSvg)})
		case "battery":
			if item, ok := p.batteryGlance(ctx); ok {
				items = append(items, item)
			}
		}
	}
	return plugin.GlanceResponse{Items: items}
}

func (p *GlancePlugin) batteryGlance(ctx context.Context) (plugin.GlanceItem, bool) {
	// Battery is a system-specific signal. Returning no item when no battery can
	// be detected keeps desktop machines from showing stale or misleading data.
	if util.IsMacOS() {
		return p.macOSBatteryGlance(ctx)
	}
	if util.IsLinux() {
		return p.linuxBatteryGlance(ctx)
	}
	if util.IsWindows() {
		return p.windowsBatteryGlance(ctx)
	}
	return plugin.GlanceItem{}, false
}

func (p *GlancePlugin) macOSBatteryGlance(ctx context.Context) (plugin.GlanceItem, bool) {
	output, err := exec.CommandContext(ctx, "pmset", "-g", "batt").Output()
	if err != nil {
		return plugin.GlanceItem{}, false
	}
	match := regexp.MustCompile(`(\d+)%`).FindStringSubmatch(string(output))
	if len(match) < 2 {
		return plugin.GlanceItem{}, false
	}
	text := match[1] + "%"
	tooltip := p.macOSBatteryTooltip(text, string(output))
	return plugin.GlanceItem{Id: "battery", Text: text, Icon: common.NewWoxImageSvg(glanceBatterySvg), Tooltip: tooltip}, true
}

func (p *GlancePlugin) macOSBatteryTooltip(text string, output string) string {
	// pmset returns a diagnostic sentence with battery ids and presence flags.
	// Glance tooltips are small UI labels, so keep only the state users can act
	// on instead of exposing the raw command output.
	cleanOutput := strings.TrimSpace(strings.ReplaceAll(output, "\n", " "))
	parts := []string{text}
	if statusMatch := regexp.MustCompile(`%;\s*([^;]+);`).FindStringSubmatch(cleanOutput); len(statusMatch) >= 2 {
		parts = append(parts, strings.TrimSpace(statusMatch[1]))
	}
	if remainingMatch := regexp.MustCompile(`;\s*([^;]+ remaining)`).FindStringSubmatch(cleanOutput); len(remainingMatch) >= 2 {
		parts = append(parts, strings.TrimSpace(remainingMatch[1]))
	}
	return joinBatteryTooltipParts(parts...)
}

func (p *GlancePlugin) linuxBatteryGlance(ctx context.Context) (plugin.GlanceItem, bool) {
	paths, err := filepath.Glob("/sys/class/power_supply/BAT*/capacity")
	if err != nil || len(paths) == 0 {
		return plugin.GlanceItem{}, false
	}
	capacity, err := os.ReadFile(paths[0])
	if err != nil {
		return plugin.GlanceItem{}, false
	}
	text := strings.TrimSpace(string(capacity)) + "%"
	statusPath := filepath.Join(filepath.Dir(paths[0]), "status")
	status, _ := os.ReadFile(statusPath)
	// Linux exposes battery status as a clean field already. Include the percent
	// so the tooltip remains useful without becoming a second verbose data dump.
	tooltip := joinBatteryTooltipParts(text, string(status))
	return plugin.GlanceItem{Id: "battery", Text: text, Icon: common.NewWoxImageSvg(glanceBatterySvg), Tooltip: tooltip}, true
}

func joinBatteryTooltipParts(parts ...string) string {
	// Tooltip parts come from platform commands, and some fields are optional.
	// Filtering blanks here avoids dangling separators in compact Glance labels.
	nonEmptyParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			nonEmptyParts = append(nonEmptyParts, trimmed)
		}
	}
	return strings.Join(nonEmptyParts, " - ")
}
