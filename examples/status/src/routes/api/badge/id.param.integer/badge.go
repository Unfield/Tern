// Package badge serves SVG status badges for READMEs:
// /api/badge/1 shows up or down, /api/badge/1?style=uptime the 30-day uptime.
package badge

import (
	"fmt"
	"html"
	"time"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
	"status/internal/store"
)

func GET(ctx *kit.Ctx) error {
	m, err := app.Store.Monitor(ctx, app.ID(ctx))
	if err != nil || !m.Public {
		return kit.NotFound()
	}
	label, value, color := m.Name, string(m.Status), "#9ca3af"
	switch m.Status {
	case store.Up:
		color = "#16a34a"
	case store.Down:
		color = "#dc2626"
	case store.Pending:
		color = "#d97706"
	}
	if ctx.Request.URL.Query().Get("style") == "uptime" {
		u, err := app.Store.Uptime(ctx, m.ID, time.Now().AddDate(0, 0, -30))
		if err != nil {
			return err
		}
		value = "no data"
		if u.Ratio >= 0 {
			value = fmt.Sprintf("%.2f%%", u.Ratio*100)
		}
		switch {
		case u.Ratio >= 0.999:
			color = "#16a34a"
		case u.Ratio >= 0.99:
			color = "#65a30d"
		case u.Ratio >= 0:
			color = "#d97706"
		}
	}
	w := ctx.Writer()
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache, max-age=60")
	_, err = fmt.Fprint(w, svg(label, value, color))
	return err
}

// svg draws a flat badge; widths are estimated at 7 pixels a character.
func svg(label, value, color string) string {
	lw, vw := 10+7*len([]rune(label)), 10+7*len([]rune(value))
	label, value = html.EscapeString(label), html.EscapeString(value)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%[1]d" height="20" role="img" aria-label="%[3]s: %[4]s">`+
		`<rect width="%[2]d" height="20" fill="#555" rx="3"/><rect x="%[2]d" width="%[5]d" height="20" fill="%[6]s" rx="3"/>`+
		`<rect x="%[2]d" width="4" height="20" fill="%[6]s"/>`+
		`<g fill="#fff" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11">`+
		`<text x="%[7]d" y="14" text-anchor="middle">%[3]s</text><text x="%[8]d" y="14" text-anchor="middle">%[4]s</text></g></svg>`,
		lw+vw, lw, label, value, vw, color, lw/2, lw+vw/2)
}
