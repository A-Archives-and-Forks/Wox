package hotkey

import (
	"fmt"
	"strings"
	"wox/util/keyboard"

	"github.com/samber/lo"
)

type hotkeySpec struct {
	modifiers         keyboard.Modifier
	key               keyboard.Key
	doubleModifierKey keyboard.Key
}

func (s hotkeySpec) isDoubleModifier() bool {
	return s.doubleModifierKey != keyboard.KeyUnknown
}

func (h *Hotkey) parseCombineKey(combineKey string) (hotkeySpec, error) {
	tokens := lo.Map(strings.Split(combineKey, "+"), func(item string, index int) string {
		return strings.TrimSpace(item)
	})

	var spec hotkeySpec
	var modifierKeys []keyboard.Key

	for _, token := range tokens {
		modifier, modifierKey, ok := parseModifierToken(token)
		if ok {
			spec.modifiers |= modifier
			modifierKeys = append(modifierKeys, modifierKey)
			continue
		}

		key, err := keyboard.ParseKey(token)
		if err != nil {
			return hotkeySpec{}, err
		}
		if spec.key != keyboard.KeyUnknown {
			return hotkeySpec{}, fmt.Errorf("multiple keys in hotkey: %s", combineKey)
		}
		spec.key = key
	}

	if spec.key == keyboard.KeyUnknown {
		if len(modifierKeys) == 2 && modifierKeys[0] == modifierKeys[1] {
			spec.doubleModifierKey = modifierKeys[0]
			return spec, nil
		}
		return hotkeySpec{}, fmt.Errorf("missing key in hotkey: %s", combineKey)
	}

	return spec, nil
}
