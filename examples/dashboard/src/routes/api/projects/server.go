// Package projects is the projects API: GET lists them, POST creates one.
package projects

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/Unfield/Tern/kit"

	"dashboard/internal/app"
	"dashboard/internal/db"
)

// ListQuery filters projects.
type ListQuery struct {
	Search string `json:"q,omitempty"`
	Status string `json:"status,omitempty"`
}

func GET(ctx *kit.Ctx, in ListQuery) ([]db.Project, error) {
	user, store, err := app.User(ctx)
	if err != nil {
		return nil, err
	}
	list, err := store.Projects(ctx, user, in.Search, in.Status)
	if list == nil {
		list = []db.Project{}
	}
	return list, err
}

// NewProject is the body of POST /api/projects.
type NewProject struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Validate runs before POST; its error is answered with 400.
func (p *NewProject) Validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > 80 {
		return errors.New("name must be 1 to 80 characters")
	}
	return nil
}

func POST(ctx *kit.Ctx, in NewProject) (db.Project, error) {
	user, store, err := app.User(ctx)
	if err != nil {
		return db.Project{}, err
	}
	return store.CreateProject(ctx, user, in.Name, in.Description)
}
