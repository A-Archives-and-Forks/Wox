package hotkey

import (
	"context"
	"fmt"
	"wox/util"
	"wox/util/keyboard"
)

// platformHotkeyAvailableCheck is a platform hook that can short-circuit the
// standard register-test-unregister availability check. If non-nil, it is called
// first. When the returned `handled` flag is true the returned `available` value
// is used directly and the standard check is skipped. Platforms that have a
// fundamentally different hotkey model (e.g. Wayland's portal-based registration)
// should set this in their init() to avoid incorrect or harmful probe behaviour.
var platformHotkeyAvailableCheck func(ctx context.Context, hotkeyStr string) (available bool, handled bool)

type Hotkey struct {
	// combineKey is the original hotkey string used for registration, e.g. "Ctrl+Shift+A".
	combineKey   string
	registration keyboard.HotkeyRegistration

	// isDoubleKey indicates whether the hotkey is a double modifier key (e.g. "Ctrl+Ctrl").
	isDoubleKey       bool
	doubleModifierKey keyboard.Key
}

func (h *Hotkey) Register(ctx context.Context, combineKey string, callback func()) error {
	spec, parseErr := h.parseCombineKey(combineKey)
	if parseErr != nil {
		return parseErr
	}
	if validateErr := validateHotkeySpec(spec); validateErr != nil {
		return validateErr
	}

	h.Unregister(ctx)
	h.combineKey = combineKey

	if spec.isDoubleModifier() {
		util.GetLogger().Info(ctx, fmt.Sprintf("register double hotkey: %s", combineKey))
		h.isDoubleKey = true
		h.doubleModifierKey = spec.doubleModifierKey
		return registerDoubleHotKey(spec.doubleModifierKey, callback)
	}

	registration, err := keyboard.RegisterGlobalHotkey(spec.modifiers, spec.key, callback)
	if err != nil {
		return err
	}

	util.GetLogger().Info(ctx, fmt.Sprintf("register normal hotkey: %s", combineKey))
	h.isDoubleKey = false
	h.registration = registration
	return nil
}

func (h *Hotkey) Unregister(ctx context.Context) {
	if h.isDoubleKey {
		util.GetLogger().Info(ctx, fmt.Sprintf("unregister double hotkey: %s", h.combineKey))
		unregisterDoubleHotKey(h.doubleModifierKey)
		h.isDoubleKey = false
		h.doubleModifierKey = keyboard.KeyUnknown
		return
	}

	if h.registration == nil {
		return
	}

	util.GetLogger().Info(ctx, fmt.Sprintf("unregister normal hotkey: %s", h.combineKey))
	if err := h.registration.Unregister(); err != nil {
		util.GetLogger().Error(ctx, fmt.Sprintf("failed to unregister hotkey: %s", err.Error()))
	}
	h.registration = nil
}

func IsHotkeyAvailable(ctx context.Context, hotkeyStr string) (isAvailable bool) {
	// Allow platforms to override the availability check with their own logic.
	// On Wayland the XDG GlobalShortcuts portal does not have a "is this key
	// taken" concept, so we cannot probe availability the same way we do on X11
	// or macOS/Windows.
	if platformHotkeyAvailableCheck != nil {
		if available, handled := platformHotkeyAvailableCheck(ctx, hotkeyStr); handled {
			return available
		}
	}

	hk := Hotkey{}
	registerErr := hk.Register(ctx, hotkeyStr, func() {})
	if registerErr == nil {
		isAvailable = true
		hk.Unregister(ctx)
	}
	return
}
