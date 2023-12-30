package main

import (
	"fmt"
	"golang.design/x/hotkey/mainthread"
	"runtime"
	"strings"
	"time"
	"wox/i18n"
	"wox/plugin"
	"wox/resource"
	"wox/setting"
	"wox/share"
	"wox/ui"
	"wox/util"
	"wox/util/hotkey"
)

import _ "wox/plugin/host" // import all hosts

// import all system plugins
import _ "wox/plugin/system"
import _ "wox/plugin/system/app"
import _ "wox/plugin/system/calculator"

func main() {
	// logger depends on location, so location must be initialized first
	locationErr := util.GetLocation().Init()
	if locationErr != nil {
		panic(locationErr)
	}

	ctx := util.NewTraceContext()
	util.GetLogger().Info(ctx, "------------------------------")
	util.GetLogger().Info(ctx, "Wox starting")
	util.GetLogger().Info(ctx, fmt.Sprintf("golang version: %s", strings.ReplaceAll(runtime.Version(), "go", "")))
	util.GetLogger().Info(ctx, fmt.Sprintf("wox data location: %s", util.GetLocation().GetWoxDataDirectory()))
	util.GetLogger().Info(ctx, fmt.Sprintf("user data location: %s", util.GetLocation().GetUserDataDirectory()))

	share.ExitApp = ExitApp

	extractErr := resource.Extract(ctx)
	if extractErr != nil {
		util.GetLogger().Error(ctx, fmt.Sprintf("failed to extract embed file: %s", extractErr.Error()))
		return
	}

	settingErr := setting.GetSettingManager().Init(ctx)
	if settingErr != nil {
		util.GetLogger().Error(ctx, fmt.Sprintf("failed to initialize settings: %s", settingErr.Error()))
		return
	}
	woxSetting := setting.GetSettingManager().GetWoxSetting(ctx)

	langErr := i18n.GetI18nManager().UpdateLang(ctx, woxSetting.LangCode)
	if langErr != nil {
		util.GetLogger().Error(ctx, fmt.Sprintf("failed to initialize lang(%s): %s", woxSetting.LangCode, langErr.Error()))
		return
	}

	themeErr := ui.GetUIManager().Start(ctx)
	if themeErr != nil {
		util.GetLogger().Error(ctx, fmt.Sprintf("failed to initialize themes: %s", themeErr.Error()))
		return
	}

	serverPort := 34987
	if util.IsProd() {
		availablePort, portErr := util.GetAvailableTcpPort(ctx)
		if portErr != nil {
			util.GetLogger().Error(ctx, fmt.Sprintf("failed to initialize lang(%s): %s", woxSetting.LangCode, portErr.Error()))
			return
		}
		serverPort = availablePort
	}

	if woxSetting.ShowTray {
		startTray(ctx)
	}

	shareUI := ui.GetUIManager().GetUI(ctx)
	plugin.GetPluginManager().Start(ctx, shareUI)

	util.InitSelection()

	// hotkey must be registered in main thread
	mainthread.Init(func() {
		registerMainHotkeyErr := ui.GetUIManager().RegisterMainHotkey(ctx, woxSetting.MainHotkey.Get())
		if registerMainHotkeyErr != nil {
			util.GetLogger().Error(ctx, fmt.Sprintf("failed to register main hotkey: %s", registerMainHotkeyErr.Error()))
		}
		registerSelectionHotkeyErr := ui.GetUIManager().RegisterSelectionHotkey(ctx, woxSetting.SelectionHotkey.Get())
		if registerSelectionHotkeyErr != nil {
			util.GetLogger().Error(ctx, fmt.Sprintf("failed to register selection hotkey: %s", registerSelectionHotkeyErr.Error()))
		}
		for _, queryHotkey := range woxSetting.QueryHotkeys.Get() {
			registerQueryHotkeyErr := ui.GetUIManager().RegisterQueryHotkey(ctx, queryHotkey)
			if registerQueryHotkeyErr != nil {
				util.GetLogger().Error(ctx, fmt.Sprintf("failed to register query hotkey: %s", registerQueryHotkeyErr.Error()))
			}
		}

		t := hotkey.Hotkey{}
		t.Register(ctx, "ctrl+ctrl", func() {
			ui.GetUIManager().GetUI(ctx).Notify(ctx, "Wox", "Wox is running")
		})

		if util.IsProd() {
			util.Go(ctx, "start ui", func() {
				time.Sleep(time.Millisecond * 200) // wait websocket server start
				appErr := ui.GetUIManager().StartUIApp(ctx, serverPort)
				if appErr != nil {
					util.GetLogger().Error(ctx, fmt.Sprintf("failed to start ui app: %s", appErr.Error()))
					return
				}
			})
		}

		ui.GetUIManager().StartWebsocketAndWait(ctx, serverPort)
	})
}
