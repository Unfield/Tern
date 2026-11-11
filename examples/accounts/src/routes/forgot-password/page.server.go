package forgotpassword

import (
	"github.com/Unfield/Tern/kit"

	"accounts/internal/auth"
)

type Request struct {
	Email string `form:"email" validate:"required,email"`
}

type Sent struct {
	Sent bool `json:"sent"`
}

var Actions = kit.Actions{
	// Known and unknown addresses get the same answer, so the form doesn't
	// tell who has an account.
	"default": kit.FormAction(func(ctx *kit.Ctx, in Request) (Sent, error) {
		if err := auth.Acc.RequestPasswordReset(ctx, in.Email); err != nil {
			return Sent{}, err
		}
		return Sent{Sent: true}, nil
	}),
}
