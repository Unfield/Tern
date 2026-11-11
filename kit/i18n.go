package kit

import "github.com/Unfield/Tern/internal/i18n"

// localeCookie is set by setLocale() in the browser.
const localeCookie = "locale"

// resolveLocale picks the request's locale: a "lang" route parameter, then
// the locale cookie, then Accept-Language, then the default locale.
func (a *App) resolveLocale(c *Ctx) {
	if a.i18n == nil {
		return
	}
	if v := c.params["lang"]; v != "" && a.i18n.Has(v) {
		c.locale, c.localeFromParam = v, true
		return
	}
	if v, ok := c.Cookie(localeCookie); ok && a.i18n.Has(v) {
		c.locale = v
		return
	}
	if v := a.i18n.Match(c.Request.Header.Get("Accept-Language")); v != "" {
		c.locale = v
		return
	}
	c.locale = a.i18n.Default
}

// Locale returns the request's locale, such as "de", or "" if the app has
// no src/locales. Tern picks it from a "lang" route parameter, the locale
// cookie (set by setLocale in the browser), Accept-Language, and finally
// the default locale. A load that reads it reruns when it changes.
func (c *Ctx) Locale() string {
	r := c.rootCtx()
	if c.uses != nil {
		if r.localeFromParam {
			c.uses.param("lang")
		} else {
			c.uses.dep("tern:locale")
		}
	}
	return r.locale
}

// SetLocale overrides the request's locale, for example from a user's
// saved preference in a hook. Unsupported locales are ignored.
func (c *Ctx) SetLocale(locale string) {
	r := c.rootCtx()
	if r.app.i18n != nil && r.app.i18n.Has(locale) {
		r.locale, r.localeFromParam = locale, false
	}
}

// T translates key into the request's locale, for text produced on the
// server such as validation errors. params fill placeholders like {name}
// and plurals; see the guide for the message syntax.
func (c *Ctx) T(key string, params ...map[string]any) string {
	if c.app.i18n == nil {
		return key
	}
	var p map[string]any
	if len(params) > 0 {
		p = params[0]
	}
	return c.app.i18n.T(c.Locale(), key, p)
}

// tb translates one of Tern's own texts, such as a validation message:
// the app's message for its key in src/locales, else Tern's English.
func (c *Ctx) tb(key string, params map[string]any) string {
	if c == nil {
		var none *i18n.Catalog
		return none.TB("en", key, params)
	}
	return c.app.i18n.TB(c.Locale(), key, params)
}

// htmlLang is the value of %tern.lang%.
func (a *App) htmlLang(c *Ctx) string {
	if l := c.rootCtx().locale; l != "" {
		return l
	}
	return "en"
}
