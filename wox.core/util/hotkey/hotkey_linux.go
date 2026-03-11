//go:build linux

package hotkey

import (
	"fmt"
	"strings"
	"wox/util/keyboard"
)

func parseModifierToken(token string) (keyboard.Modifier, keyboard.Key, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "ctrl", "control":
		return keyboard.ModifierCtrl, keyboard.KeyCtrl, true
	case "shift":
		return keyboard.ModifierShift, keyboard.KeyShift, true
	case "alt":
		return keyboard.ModifierAlt, keyboard.KeyAlt, true
	case "super", "win", "window":
		return keyboard.ModifierSuper, keyboard.KeySuper, true
	default:
		return 0, keyboard.KeyUnknown, false
	}
}

func validateHotkeySpec(spec hotkeySpec) error {
	if spec.isDoubleModifier() && keyboard.IsWaylandSession() {
		return fmt.Errorf("double modifier hotkeys are not supported on Wayland")
	}
	return nil
}
