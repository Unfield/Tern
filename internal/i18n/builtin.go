package i18n

import (
	_ "embed"
	"encoding/json"
)

//go:embed builtin.json
var builtinJSON []byte

// Builtin holds Tern's own texts in English: Tern UI's and the
// validation messages, under keys starting with "tern.". An app
// translates them in src/locales like its own messages.
var Builtin = func() map[string]string {
	m := map[string]string{}
	if err := json.Unmarshal(builtinJSON, &m); err != nil {
		panic(err)
	}
	return m
}()

// TB formats one of Tern's own texts: the app's message for key in
// locale or its default locale if it has one, else Tern's English. c may
// be nil, for an app without translations.
func (c *Catalog) TB(locale, key string, params map[string]any) string {
	if c != nil {
		if msg, ok := c.messages[locale][key]; ok {
			return Format(locale, msg, params)
		}
		if msg, ok := c.messages[c.Default][key]; ok {
			return Format(locale, msg, params)
		}
	}
	if locale == "" {
		locale = "en"
	}
	msg, ok := Builtin[key]
	if !ok {
		return key
	}
	return Format(locale, msg, params)
}
