package link

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

type Data struct {
	Link shelf.Link `json:"link"`
}

func Load(ctx *kit.Ctx) (Data, error) {
	user, err := shelf.Accounts.Require(ctx)
	if err != nil {
		return Data{}, err
	}
	link, err := shelf.Get(ctx, user.ID, id(ctx))
	if errors.Is(err, shelf.ErrNotFound) {
		return Data{}, kit.NotFound()
	}
	if err != nil {
		return Data{}, err
	}
	return Data{Link: link}, nil
}

// id is the link's ID from the URL. The integer matcher only lets
// numbers through, so it always parses.
func id(ctx *kit.Ctx) int64 {
	n, _ := strconv.ParseInt(ctx.Param("id"), 10, 64)
	return n
}

type Toggle struct {
	Read bool `form:"read"`
}

var Actions = kit.Actions{
	"read": kit.FormAction(func(ctx *kit.Ctx, in Toggle) (struct{}, error) {
		user, err := shelf.Accounts.Require(ctx)
		if err != nil {
			return struct{}{}, err
		}
		err = shelf.SetRead(ctx, user.ID, id(ctx), in.Read)
		if errors.Is(err, shelf.ErrNotFound) {
			return struct{}{}, kit.NotFound()
		}
		return struct{}{}, err
	}),
	"delete": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		user, err := shelf.Accounts.Require(ctx)
		if err != nil {
			return struct{}{}, err
		}
		err = shelf.Delete(ctx, user.ID, id(ctx))
		if errors.Is(err, shelf.ErrNotFound) {
			return struct{}{}, kit.NotFound()
		}
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/")
	}),
}
