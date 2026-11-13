package admin

import (
	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/kit"
)

func Load(c *kit.Ctx) (admin.Page, error) { return admin.Load(c) }

var Actions = admin.Actions
