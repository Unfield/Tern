// Package app is the signed-in part of the dashboard. The app.group
// directory shares one layout without adding to the URLs.
package app

import (
	"github.com/Unfield/Tern/kit"

	"dashboard/internal/app"
)

type Shell struct {
	DisplayName string        `json:"displayName"`
	Email       string        `json:"email"`
	Projects    []ProjectLink `json:"projects"`
}

type ProjectLink struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func LayoutLoad(ctx *kit.Ctx) (Shell, error) {
	user, store, err := app.User(ctx)
	if err != nil {
		return Shell{}, err
	}
	profile, err := store.Profile(ctx, user)
	if err != nil {
		return Shell{}, err
	}
	projects, err := store.Projects(ctx, user, "", "active")
	if err != nil {
		return Shell{}, err
	}
	links := make([]ProjectLink, 0, len(projects))
	for _, p := range projects {
		links = append(links, ProjectLink{ID: p.ID, Name: p.Name})
	}
	return Shell{DisplayName: profile.DisplayName, Email: profile.Email, Projects: links}, nil
}
