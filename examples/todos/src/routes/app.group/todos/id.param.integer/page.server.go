package todo

import (
	"strconv"

	"github.com/Unfield/Tern/kit"

	"todos/internal/session"
	"todos/internal/store"
)

type Detail struct {
	Todo store.Todo `json:"todo"`
}

func Load(ctx *kit.Ctx) (Detail, error) {
	id, _ := strconv.Atoi(ctx.Param("id")) // the integer matcher guarantees a number
	t, err := store.Default.Get(session.User(ctx), id)
	if err != nil {
		return Detail{}, kit.NotFound()
	}
	return Detail{Todo: t}, nil
}
