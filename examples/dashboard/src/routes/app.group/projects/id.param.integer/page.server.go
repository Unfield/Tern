package project

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Unfield/Tern/kit"

	"dashboard/internal/app"
	"dashboard/internal/db"
)

type Detail struct {
	Project db.Project `json:"project"`
	Tasks   []db.Task  `json:"tasks"`
}

func projectID(ctx *kit.Ctx) int64 {
	id, _ := strconv.ParseInt(ctx.Param("id"), 10, 64) // the integer matcher guarantees a number
	return id
}

func Load(ctx *kit.Ctx) (Detail, error) {
	user, store, err := app.User(ctx)
	if err != nil {
		return Detail{}, err
	}
	p, err := store.Project(ctx, user, projectID(ctx))
	if err != nil {
		return Detail{}, app.NotFound(err)
	}
	tasks, err := store.Tasks(ctx, p.ID)
	return Detail{Project: p, Tasks: tasks}, err
}

type TaskAdded struct {
	Added db.Task `json:"added"`
}

type Saved struct {
	Saved bool `json:"saved"`
}

type Invalid struct {
	Values map[string]string `json:"values"`
	Errors map[string]string `json:"errors"`
}

var priorities = []string{"low", "medium", "high"}
var statuses = []string{"active", "paused", "done"}

var Actions = kit.Actions{
	"addTask": kit.Action(func(ctx *kit.Ctx) (TaskAdded, error) {
		user, store, err := app.User(ctx)
		if err != nil {
			return TaskAdded{}, err
		}
		form, err := ctx.Form()
		if err != nil {
			return TaskAdded{}, err
		}
		title := strings.TrimSpace(form.Get("title"))
		notes := strings.TrimSpace(form.Get("notes"))
		priority := form.Get("priority")
		errs := map[string]string{}
		if title == "" || utf8.RuneCountInString(title) > 120 {
			errs["title"] = "A task needs a title of up to 120 characters."
		}
		if !slices.Contains(priorities, priority) {
			errs["priority"] = "Pick a priority."
		}
		if len(errs) > 0 {
			return TaskAdded{}, kit.Fail(http.StatusBadRequest, Invalid{
				Values: map[string]string{"title": title, "notes": notes, "priority": priority},
				Errors: errs,
			})
		}
		t, err := store.AddTask(ctx, user, projectID(ctx), title, notes, priority)
		return TaskAdded{Added: t}, app.NotFound(err)
	}),
	"toggle": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		user, store, err := app.User(ctx)
		if err != nil {
			return struct{}{}, err
		}
		id, err := app.FormID(ctx, "task")
		if err != nil {
			return struct{}{}, err
		}
		form, _ := ctx.Form()
		return struct{}{}, app.NotFound(store.SetTaskDone(ctx, user, id, form.Get("done") == "true"))
	}),
	"deleteTask": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		user, store, err := app.User(ctx)
		if err != nil {
			return struct{}{}, err
		}
		id, err := app.FormID(ctx, "task")
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, app.NotFound(store.DeleteTask(ctx, user, id))
	}),
	"update": kit.Action(func(ctx *kit.Ctx) (Saved, error) {
		user, store, err := app.User(ctx)
		if err != nil {
			return Saved{}, err
		}
		form, err := ctx.Form()
		if err != nil {
			return Saved{}, err
		}
		name := strings.TrimSpace(form.Get("name"))
		desc := strings.TrimSpace(form.Get("description"))
		status := form.Get("status")
		errs := map[string]string{}
		if name == "" || utf8.RuneCountInString(name) > 60 {
			errs["name"] = "The name must be 1 to 60 characters."
		}
		if !slices.Contains(statuses, status) {
			errs["status"] = "Pick a status."
		}
		if len(errs) > 0 {
			return Saved{}, kit.Fail(http.StatusBadRequest, Invalid{
				Values: map[string]string{"name": name, "description": desc, "status": status},
				Errors: errs,
			})
		}
		return Saved{Saved: true}, app.NotFound(store.UpdateProject(ctx, user, projectID(ctx), name, desc, status))
	}),
	"delete": func(ctx *kit.Ctx) (any, error) {
		user, store, err := app.User(ctx)
		if err != nil {
			return nil, err
		}
		if err := store.DeleteProject(ctx, user, projectID(ctx)); err != nil {
			return nil, app.NotFound(err)
		}
		return nil, kit.Redirect(http.StatusSeeOther, "/projects")
	},
}
