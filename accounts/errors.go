package accounts

import (
	"errors"

	"github.com/Unfield/Tern/kit"
)

// FormError turns the errors users cause into form errors, in their
// language, for an action to return: a wrong password, a taken email, an
// expired link. Other errors stay as they are.
//
//	if _, err := acc.SignIn(c, in.Email, in.Password, in.Remember); err != nil {
//		return struct{}{}, accounts.FormError(c, err)
//	}
func FormError(c *kit.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return kit.FormError(c.TB("tern.accounts.error.credentials", nil))
	case errors.Is(err, ErrWrongPassword):
		return kit.FieldError("current", c.TB("tern.accounts.error.wrongPassword", nil))
	case errors.Is(err, ErrEmailTaken):
		return kit.FieldError("email", c.TB("tern.accounts.error.emailTaken", nil))
	case errors.Is(err, ErrInvalidToken):
		return kit.FormError(c.TB("tern.accounts.error.invalidToken", nil))
	case errors.Is(err, ErrLastSignIn):
		return kit.FormError(c.TB("tern.accounts.error.lastSignIn", nil))
	case errors.Is(err, ErrInvalidCode):
		return kit.FieldError("code", c.TB("tern.accounts.error.invalidCode", nil))
	case errors.Is(err, ErrTooManyAttempts):
		return kit.FormError(c.TB("tern.accounts.error.tooMany", nil))
	case errors.Is(err, ErrPasskey):
		return kit.FormError(c.TB("tern.accounts.error.passkey", nil))
	case errors.Is(err, ErrNotAllowed):
		return kit.FormError(c.TB("tern.accounts.error.notAllowed", nil))
	case errors.Is(err, ErrLastOwner):
		return kit.FormError(c.TB("tern.accounts.error.lastOwner", nil))
	case errors.Is(err, ErrAlreadyMember):
		return kit.FieldError("email", c.TB("tern.accounts.error.alreadyMember", nil))
	case errors.Is(err, ErrWrongInvitee):
		return kit.FormError(c.TB("tern.accounts.error.wrongInvitee", nil))
	case errors.Is(err, ErrUnknownRole):
		return kit.FieldError("role", c.TB("tern.accounts.error.unknownRole", nil))
	case errors.Is(err, ErrNotMember):
		return kit.FormError(c.TB("tern.accounts.error.notMember", nil))
	}
	return err
}
