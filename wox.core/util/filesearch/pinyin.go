package filesearch

import (
	"strings"
	"unicode"

	"wox/util/fuzzymatch"
)

func buildPinyinFields(input string) (string, string) {
	var full strings.Builder
	var initials strings.Builder

	for _, r := range input {
		switch {
		case unicode.Is(unicode.Han, r):
			pinyins, ok := fuzzymatch.PinyinDict[int(r)]
			if !ok || len(pinyins) == 0 {
				continue
			}

			pinyin := strings.ToLower(strings.TrimSpace(pinyins[0]))
			if pinyin == "" {
				continue
			}

			full.WriteString(pinyin)
			initials.WriteByte(pinyin[0])
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			lower := strings.ToLower(string(r))
			full.WriteString(lower)
			initials.WriteString(lower)
		}
	}

	return full.String(), initials.String()
}
