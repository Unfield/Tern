package todo

import (
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"

	"todos/internal/session"
	"todos/internal/store"
)

type Detail struct {
	Todo store.Todo `json:"todo"`
	// Similar is slow to compute, so the page renders without waiting for it.
	Similar kit.Deferred[[]store.Todo] `json:"similar"`
}

func Load(ctx *kit.Ctx) (Detail, error) {
	id, _ := strconv.Atoi(ctx.Param("id")) // the integer matcher guarantees a number
	user := session.User(ctx)
	t, err := store.Default.Get(user, id)
	if err != nil {
		return Detail{}, kit.NotFound()
	}
	return Detail{
		Todo: t,
		Similar: kit.Defer(ctx, func() ([]store.Todo, error) {
			select {
			case <-time.After(300 * time.Millisecond): // pretend this is an expensive query
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			first, _, _ := strings.Cut(strings.ToLower(t.Title), " ")
			similar := []store.Todo{}
			for _, other := range store.Default.List(user) {
				if other.ID != t.ID && strings.Contains(strings.ToLower(other.Title), first) {
					similar = append(similar, other)
				}
			}
			return similar, nil
		}),
	}, nil
}
