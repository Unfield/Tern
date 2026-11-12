package flags

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/kit"
)

// Admin is a resource for the admin package's screens, to see and change
// the flags. Changes apply in this process at once.
//
//	admin.New(admin.Options{Authorize: onlyAdmins}, f.Admin())
func (f *Flags) Admin(opts ...admin.Option) admin.Res {
	return admin.Resource[Flag]("flags", store{f}, append([]admin.Option{admin.Label("Feature flags", "Flag")}, opts...)...)
}

// store is admin.Store over the flags.
type store struct{ f *Flags }

func (s store) List(_ context.Context, q admin.Query) ([]Flag, int, error) {
	var out []Flag
	text := strings.ToLower(q.Search)
	for _, fl := range s.f.List() {
		if text != "" && !strings.Contains(strings.ToLower(fl.Name+" "+fl.Description), text) {
			continue
		}
		if v, ok := q.Filters["on"]; ok && (v == "true") != fl.On {
			continue
		}
		out = append(out, fl)
	}
	switch q.Sort {
	case "percent":
		slices.SortStableFunc(out, func(a, b Flag) int { return cmp.Compare(a.Percent, b.Percent) })
	case "on":
		slices.SortStableFunc(out, func(a, b Flag) int { return cmp.Compare(boolInt(a.On), boolInt(b.On)) })
	case "by":
		slices.SortStableFunc(out, func(a, b Flag) int { return strings.Compare(a.By, b.By) })
	}
	if q.Desc {
		slices.Reverse(out)
	}
	from := min(q.Offset, len(out))
	to := min(from+q.Limit, len(out))
	return out[from:to], len(out), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var errExists = kit.FieldError("name", "There's already a flag with this name.")

func (s store) Get(_ context.Context, key string) (Flag, error) {
	fl, ok := s.f.Get(key)
	if !ok {
		return Flag{}, admin.ErrNotFound
	}
	return fl, nil
}

func (s store) Create(ctx context.Context, fl Flag) (string, error) {
	if _, ok := s.f.Get(fl.Name); ok {
		return "", errExists
	}
	return fl.Name, s.f.Set(ctx, fl)
}

func (s store) Update(ctx context.Context, key string, fl Flag, _ []string) error {
	if _, ok := s.f.Get(key); !ok {
		return admin.ErrNotFound
	}
	fl.Name = key
	return s.f.Set(ctx, fl)
}

func (s store) Delete(ctx context.Context, key string) error {
	if _, ok := s.f.Get(key); !ok {
		return admin.ErrNotFound
	}
	return s.f.Remove(ctx, key)
}
