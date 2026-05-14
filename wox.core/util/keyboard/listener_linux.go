//go:build linux && cgo

package keyboard

import (
	"fmt"
	"os"
	"strings"
	"wox/util"
)

func RegisterGlobalHotkey(modifiers Modifier, key Key, callback func()) (HotkeyRegistration, error) {
	if IsWaylandSession() {
		// First choice: XDG GlobalShortcuts portal (GNOME 47+, KDE, etc.).
		reg, err := registerGlobalHotkeyLinuxWayland(modifiers, key, callback)
		if err == nil {
			return reg, nil
		}

		// Second choice: GNOME custom keybindings via gsettings.
		// This works on any GNOME version without portal support.
		// XGrabKey via XWayland is intentionally skipped here: it registers
		// without error but cannot intercept keys globally under GNOME's
		// Wayland compositor, making it silently non-functional.
		util.GetLogger().Warn(util.NewTraceContext(), fmt.Sprintf(
			"[hotkey] wayland portal unavailable (%v), falling back to GNOME custom keybindings", err))
		return registerGlobalHotkeyLinuxGnome(modifiers, key, callback)
	}
	return registerGlobalHotkeyLinuxX11(modifiers, key, callback)
}

func AddRawKeyListener(handler RawKeyHandler) (RawKeySubscription, error) {
	if IsWaylandSession() {
		return addRawKeyListenerLinuxWayland(handler)
	}
	return addRawKeyListenerLinuxX11(handler)
}

func IsWaylandSession() bool {
	return strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") || os.Getenv("WAYLAND_DISPLAY") != ""
}

func unsupportedWaylandRawListenerError() error {
	return fmt.Errorf("raw keyboard listeners are not supported on Wayland; double modifier hotkeys are unavailable")
}
