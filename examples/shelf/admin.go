package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

// The admin screens, at /admin, for the accounts in SHELF_ADMINS.

// adminLink is a link as the admin screens show it: everyone's, with its owner.
type adminLink struct {
	ID      int64     `admin:"key"`
	Title   string    `validate:"required,max=200" admin:"list,search"`
	URL     string    `validate:"required,url,max=2000" label:"Address" admin:"list,search"`
	Read    bool      `admin:"list,filter"`
	UserID  int64     `label:"Owner" admin:"list,ref=users"`
	AddedAt time.Time `admin:"list,readonly"`
}

// adminUser is an account. Accounts change their own email and password,
// so here they are only shown.
type adminUser struct {
	ID            int64  `admin:"key"`
	Email         string `admin:"list,search"`
	Name          string `admin:"list,search"`
	EmailVerified *int64 `admin:"list,unixms"`
	CreatedAt     int64  `label:"Joined" admin:"list,unixms"`
}

func adminPanel() (*admin.Admin, error) {
	links, err := admin.NewTable[adminLink](shelf.DB, "links", "")
	if err != nil {
		return nil, err
	}
	users, err := admin.NewTable[adminUser](shelf.DB, "tern_users", "")
	if err != nil {
		return nil, err
	}
	return admin.New(admin.Options{
		Title: "Shelf admin",
		Authorize: func(c *kit.Ctx) error {
			u, err := shelf.Accounts.Require(c)
			if err != nil {
				return err
			}
			if !shelf.IsAdmin(u) {
				return kit.Error(http.StatusForbidden, "")
			}
			return nil
		},
	},
		// The search index follows what admins change.
		admin.Resource("links", links,
			admin.AfterSave(func(ctx context.Context, key string) error {
				id, _ := strconv.ParseInt(key, 10, 64)
				return shelf.IndexLink(ctx, id)
			}),
			admin.AfterDelete(func(ctx context.Context, key string) error { return shelf.Index.Delete(ctx, key) }),
		),
		admin.Resource("users", users, admin.ReadOnly()),
	), nil
}
