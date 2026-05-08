//go:build windows

package glance

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
	"wox/common"
	"wox/plugin"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemPowerStatus = kernel32.NewProc("GetSystemPowerStatus")
)

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

const (
	batteryFlagCharging   = 8
	batteryFlagNoBattery  = 128
	unknownBatteryPercent = 255
)

func (p *GlancePlugin) windowsBatteryGlance(ctx context.Context) (plugin.GlanceItem, bool) {
	_ = ctx
	var status systemPowerStatus
	ret, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		return plugin.GlanceItem{}, false
	}

	if status.BatteryFlag&batteryFlagNoBattery != 0 || status.BatteryLifePercent == unknownBatteryPercent {
		return p.windowsPowerSourceGlance(status)
	}

	text := fmt.Sprintf("%d%%", status.BatteryLifePercent)
	tooltip := p.windowsBatteryTooltip(text, status)
	return plugin.GlanceItem{Id: "battery", Text: text, Icon: common.NewWoxImageSvg(glanceBatterySvg), Tooltip: tooltip}, true
}

func (p *GlancePlugin) windowsPowerSourceGlance(status systemPowerStatus) (plugin.GlanceItem, bool) {
	// Windows can still report useful AC power state when there is no battery or
	// the battery percentage is unknown. Showing a compact power-source label is
	// more useful than hiding the selected Glance item on desktops and docks.
	switch status.ACLineStatus {
	case 0:
		return plugin.GlanceItem{Id: "battery", Text: "BAT", Icon: common.NewWoxImageSvg(glanceBatterySvg), Tooltip: "Battery percent unknown"}, true
	case 1:
		return plugin.GlanceItem{Id: "battery", Text: "AC", Icon: common.NewWoxImageSvg(glancePlugSvg), Tooltip: "Plugged in"}, true
	default:
		return plugin.GlanceItem{}, false
	}
}

func (p *GlancePlugin) windowsBatteryTooltip(text string, status systemPowerStatus) string {
	// Keep the tooltip compact by translating the raw Windows status flags into
	// the state users care about instead of exposing numeric API constants.
	parts := []string{text}
	if status.BatteryFlag&batteryFlagCharging != 0 {
		parts = append(parts, "Charging")
	} else {
		switch status.ACLineStatus {
		case 0:
			parts = append(parts, "On battery")
		case 1:
			parts = append(parts, "Plugged in")
		}
	}

	if status.BatteryLifeTime != ^uint32(0) {
		parts = append(parts, windowsBatteryRemainingText(status.BatteryLifeTime))
	}
	return joinBatteryTooltipParts(parts...)
}

func windowsBatteryRemainingText(seconds uint32) string {
	minutes := seconds / 60
	hours := minutes / 60
	minutes = minutes % 60
	values := []string{}
	if hours > 0 {
		values = append(values, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 || hours == 0 {
		values = append(values, fmt.Sprintf("%dm", minutes))
	}
	return strings.Join(values, " ") + " remaining"
}
