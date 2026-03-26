package i18n

import "fmt"

type Lang = string

const (
	RU Lang = "ru"
	EN Lang = "en"
	KZ Lang = "kz"
)

var translations = map[Lang]map[string]string{
	RU: ruMessages,
	EN: enMessages,
	KZ: kzMessages,
}

func T(lang Lang, key string, args ...any) string {
	m, ok := translations[lang]
	if !ok {
		m = translations[EN]
	}
	s, ok := m[key]
	if !ok {
		s, ok = translations[EN][key]
		if !ok {
			return key
		}
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}
