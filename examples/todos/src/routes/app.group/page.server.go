package app

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Unfield/Tern/kit"

	"todos/internal/session"
	"todos/internal/store"
)

type Page struct {
	Todos []store.Todo `json:"todos"`
	Left  int          `json:"left"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	todos := store.Default.List(session.User(ctx))
	left := 0
	for _, t := range todos {
		if !t.Done {
			left++
		}
	}
	return Page{Todos: todos, Left: left}, nil
}

type Created struct {
	Created store.Todo `json:"created"`
}

type Invalid struct {
	Title string `json:"title"`
	Error string `json:"error"`
}

var Actions = kit.Actions{
	"create": kit.Action(create),
	"toggle": kit.Action(withID(store.Default.Toggle)),
	"delete": kit.Action(withID(store.Default.Delete)),
}

func create(ctx *kit.Ctx) (Created, error) {
	form, err := ctx.Form()
	if err != nil {
		return Created{}, err
	}
	title := strings.TrimSpace(form.Get("title"))
	if title == "" || utf8.RuneCountInString(title) > 120 {
		return Created{}, kit.Fail(http.StatusBadRequest, Invalid{Title: title, Error: "A todo needs a title of up to 120 characters."})
	}
	return Created{Created: store.Default.Add(session.User(ctx), title)}, nil
}

func withID(op func(user string, id int) error) func(*kit.Ctx) (struct{}, error) {
	return func(ctx *kit.Ctx) (struct{}, error) {
		form, err := ctx.Form()
		if err != nil {
			return struct{}{}, err
		}
		id, err := strconv.Atoi(form.Get("id"))
		if err != nil {
			return struct{}{}, kit.Error(http.StatusBadRequest, "Invalid todo ID")
		}
		if err := op(session.User(ctx), id); err != nil {
			return struct{}{}, kit.NotFound()
		}
		return struct{}{}, nil
	}
}
