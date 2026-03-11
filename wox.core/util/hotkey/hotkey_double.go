package hotkey

import (
	"wox/util"
	"wox/util/keyboard"
)

var (
	doubleKeyMu        = util.NewHashMap[keyboard.Key, int64]()
	doubleKeyCallbacks = util.NewHashMap[keyboard.Key, func()]()
	doubleKeyListener  keyboard.RawKeySubscription
)

func registerDoubleHotKey(modifierKey keyboard.Key, callback func()) error {
	doubleKeyCallbacks.Store(modifierKey, callback)

	if doubleKeyListener != nil {
		return nil
	}

	listener, err := keyboard.AddRawKeyListener(func(event keyboard.RawKeyEvent) bool {
		if event.Type != keyboard.EventTypeKeyUp || event.Key == keyboard.KeyUnknown {
			return false
		}

		callback, ok := doubleKeyCallbacks.Load(event.Key)
		if !ok || callback == nil {
			return false
		}

		now := util.GetSystemTimestamp()
		if lastUpAt, found := doubleKeyMu.Load(event.Key); found && now-lastUpAt < 500 {
			doubleKeyMu.Delete(event.Key)
			util.Go(util.NewTraceContext(), "double hotkey callback", func() {
				callback()
			})
			return false
		}

		doubleKeyMu.Store(event.Key, now)
		return false
	})
	if err != nil {
		doubleKeyCallbacks.Delete(modifierKey)
		return err
	}

	doubleKeyListener = listener
	return nil
}

func unregisterDoubleHotKey(modifierKey keyboard.Key) {
	doubleKeyCallbacks.Delete(modifierKey)
	doubleKeyMu.Delete(modifierKey)

	if doubleKeyCallbacks.Len() > 0 {
		return
	}

	if doubleKeyListener != nil {
		_ = doubleKeyListener.Close()
		doubleKeyListener = nil
	}
}
