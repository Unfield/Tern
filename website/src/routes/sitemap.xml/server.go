// Package sitemap lists the site's pages for search engines, prerendered
// to /sitemap.xml.
package sitemap

import (
	"encoding/xml"
	"net/http"

	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

const Prerender = true

// Origin is where the site is served.
const Origin = "https://tern.land"

type urlset struct {
	XMLName xml.Name `xml:"urlset"`
	NS      string   `xml:"xmlns,attr"`
	URLs    []loc    `xml:"url"`
}

type loc struct {
	Loc string `xml:"loc"`
}

func GET(ctx *kit.Ctx) error {
	set := urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, p := range []string{"/", "/examples"} {
		set.URLs = append(set.URLs, loc{Origin + p})
	}
	nav, err := docs.Nav()
	if err != nil {
		return err
	}
	for _, s := range nav {
		for _, l := range s.Links {
			set.URLs = append(set.URLs, loc{Origin + l.URL})
		}
	}
	b, err := xml.MarshalIndent(set, "", "\t")
	if err != nil {
		return err
	}
	ctx.SetHeader("Content-Type", "application/xml; charset=utf-8")
	return ctx.Text(http.StatusOK, xml.Header+string(b)+"\n")
}
