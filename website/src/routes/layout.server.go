// Package routes is the site's root layout.
package routes

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

// Site is every page's data.
type Site struct {
	// Version is Tern's latest release, from the changelog.
	Version string `json:"version"`
	Repo    string `json:"repo"`
}

var releaseRe = regexp.MustCompile(`(?m)^## \[(\d+\.\d+\.\d+)\]`)

func LayoutLoad(ctx *kit.Ctx) (Site, error) {
	s := Site{Repo: docs.RepoURL}
	b, err := os.ReadFile(filepath.Join(docs.Root, "CHANGELOG.md"))
	if err != nil {
		return s, err
	}
	if m := releaseRe.FindSubmatch(b); m != nil {
		s.Version = string(m[1])
	}
	return s, nil
}
