package kit

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/internal/formrules"
)

// Invalid is what a FormAction fails with when the form doesn't pass its
// rules: the page's form prop, so it can show each error next to its field
// and refill what was typed. Tern UI's Form reads it.
type Invalid struct {
	// Errors maps field names to messages.
	Errors map[string]string `json:"errors"`
	// Values holds what was posted, except secret fields such as passwords.
	Values map[string]any `json:"values"`
	// Message is an error about the whole form, such as "Wrong email or password."
	Message string `json:"message,omitempty"`
}

// FieldErrors maps field names to messages. Return it from a FormAction, or
// from an input's Validate method, to fail with errors on those fields; the
// key "" is a message for the whole form.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	var parts []string
	for k, v := range e {
		if k == "" {
			parts = append(parts, v)
		} else {
			parts = append(parts, k+": "+v)
		}
	}
	return strings.Join(parts, "; ")
}

// FieldError fails a FormAction with a message on one field, such as
// kit.FieldError("email", "That email is already registered.").
func FieldError(field, message string) error { return FieldErrors{field: message} }

// FormError fails a FormAction with a message about the whole form.
func FormError(message string) error { return FieldErrors{"": message} }

// FormAction adapts an action that takes its form as a struct. The form is
// decoded into In and checked against its validate tags, and then its
// Validate method if it has one, before fn runs:
//
//	type NewProject struct {
//		Name  string `form:"name" validate:"required,min=3,max=60" label:"Project name"`
//		Email string `form:"email" validate:"required,email"`
//		Seats int    `form:"seats" validate:"min=1,max=500"`
//	}
//
//	var Actions = kit.Actions{
//		"create": kit.FormAction(func(ctx *kit.Ctx, in NewProject) (Created, error) { ... }),
//	}
//
// If a check fails, the action fails with status 400 and an Invalid. tern
// gen writes the same rules to ./$forms, so the page can check them as the
// user types. The rules are:
//
//	required        the field must have a value; a checkbox must be checked
//	min=N, max=N    characters for text, the value for numbers, the date for
//	                dates, choices for []string
//	len=N           exactly N characters
//	email, url      an email address; an http or https URL
//	pattern=RE      the whole value matches RE; it must be the last rule
//	oneof=a|b|c     one of these values
//	same=field      equal to another field, such as a password confirmation
//
// Fields are named by their form tag, else their json name. The label tag
// names them in messages (default: the name, humanized), and message
// replaces all of a field's messages. form:"name,secret" keeps a value out
// of Invalid.Values; fields named like "password" are secret anyway.
// time.Time fields are dates (YYYY-MM-DD), or with form:"name,datetime",
// a datetime-local input's value.
func FormAction[In, Out any](fn func(*Ctx, In) (Out, error)) ActionFunc {
	fields, err := formrules.FromReflect(reflect.TypeFor[In]())
	if err != nil {
		panic(fmt.Sprintf("kit.FormAction: %v", err))
	}
	return func(c *Ctx) (any, error) {
		form, err := c.Form()
		if err != nil {
			return nil, err
		}
		if errs := checkForm(c, fields, form); len(errs) > 0 {
			return nil, Fail(http.StatusBadRequest, invalid(fields, form, errs))
		}
		var in In
		decodeForm(fields, form, reflect.ValueOf(&in).Elem())
		if err := validateInput(&in); err != nil {
			if fe, ok := errors.AsType[FieldErrors](err); ok {
				return nil, Fail(http.StatusBadRequest, invalid(fields, form, fe))
			}
			return nil, asBadRequest(err)
		}
		out, err := fn(c, in)
		if fe, ok := errors.AsType[FieldErrors](err); ok {
			return nil, Fail(http.StatusBadRequest, invalid(fields, form, fe))
		}
		return out, err
	}
}

func validateInput(in any) error {
	if v, ok := in.(Validator); ok {
		return v.Validate()
	}
	if v, ok := reflect.ValueOf(in).Elem().Interface().(Validator); ok {
		return v.Validate()
	}
	return nil
}

func checkForm(c *Ctx, fields []formrules.Field, form url.Values) map[string]string {
	labels := map[string]string{}
	for _, f := range fields {
		labels[f.Name] = f.Label
	}
	errs := map[string]string{}
	for _, f := range fields {
		if m := formrules.Check(f, form[f.Name], form.Get, func(n string) string { return labels[n] }, c.tb); m != "" {
			errs[f.Name] = m
		}
	}
	return errs
}

func invalid(fields []formrules.Field, form url.Values, errs map[string]string) Invalid {
	out := Invalid{Errors: map[string]string{}, Values: map[string]any{}}
	for k, v := range errs {
		if k == "" {
			out.Message = v
		} else {
			out.Errors[k] = v
		}
	}
	secret := map[string]bool{}
	for _, f := range fields {
		if f.Secret {
			secret[f.Name] = true
		}
		if f.Kind == formrules.Strings {
			if v, ok := form[f.Name]; ok {
				out.Values[f.Name] = v
			}
			secret[f.Name] = true // handled
		}
	}
	for k, v := range form {
		if secret[k] || strings.Contains(strings.ToLower(k), "password") || len(v) == 0 {
			continue
		}
		out.Values[k] = v[0]
	}
	return out
}

// decodeForm sets the fields of v from a form that passed its checks.
func decodeForm(fields []formrules.Field, form url.Values, v reflect.Value) {
	for _, f := range fields {
		values := form[f.Name]
		first := ""
		if len(values) > 0 {
			first = strings.TrimSpace(values[0])
		}
		dst := v.FieldByIndex(f.Index())
		if f.Kind != formrules.Strings && f.Kind != formrules.Bool && first == "" {
			continue // the zero value, or nil for pointers
		}
		if dst.Kind() == reflect.Pointer {
			p := reflect.New(dst.Type().Elem())
			dst.Set(p)
			dst = p.Elem()
		}
		switch f.Kind {
		case formrules.String:
			dst.SetString(values[0])
		case formrules.Bool:
			dst.SetBool(len(values) > 0 && formrules.Truthy(values[0]))
		case formrules.Int:
			if dst.CanInt() {
				n, _ := strconv.ParseInt(first, 10, 64)
				dst.SetInt(n)
			} else {
				n, _ := strconv.ParseUint(strings.TrimPrefix(first, "+"), 10, 64)
				dst.SetUint(n)
			}
		case formrules.Float:
			n, _ := strconv.ParseFloat(first, 64)
			dst.SetFloat(n)
		case formrules.Date, formrules.DateTime:
			t, _ := formrules.ParseTime(f.Kind, first)
			dst.Set(reflect.ValueOf(t))
		case formrules.Strings:
			var list []string
			for _, s := range values {
				if strings.TrimSpace(s) != "" {
					list = append(list, s)
				}
			}
			s := reflect.MakeSlice(dst.Type(), len(list), len(list))
			for i, x := range list {
				s.Index(i).SetString(x)
			}
			dst.Set(s)
		}
	}
}

// checkTyped applies validate tags to a typed endpoint's decoded input, by
// turning its fields back into the text a form would post.
func checkTyped(c *Ctx, in any) error {
	t := reflect.TypeOf(in)
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	fields, err := cachedFields(t)
	if err != nil || len(fields) == 0 {
		return nil //nolint:nilerr // types that can't come from a form have no rules
	}
	v := reflect.ValueOf(in)
	form := url.Values{}
	for _, f := range fields {
		form[f.Name] = asText(f.Kind, v.FieldByIndex(f.Index()))
	}
	errs := checkForm(c, fields, form)
	if len(errs) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(errs))
	for _, f := range fields {
		if m, ok := errs[f.Name]; ok {
			msgs = append(msgs, m)
		}
	}
	return Error(http.StatusBadRequest, strings.Join(msgs, " "))
}

func asText(kind formrules.Kind, v reflect.Value) []string {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	} else if (kind == formrules.Int || kind == formrules.Float) && v.IsZero() {
		// A missing query parameter or JSON field leaves 0; use a pointer
		// to tell an explicit 0 apart.
		return nil
	}
	switch kind {
	case formrules.Strings:
		out := make([]string, v.Len())
		for i := range out {
			out[i] = v.Index(i).String()
		}
		return out
	case formrules.Bool:
		if v.Bool() {
			return []string{"on"}
		}
		return nil
	case formrules.Date, formrules.DateTime:
		t := v.Interface().(time.Time)
		if t.IsZero() {
			return nil
		}
		if kind == formrules.Date {
			return []string{t.Format(time.DateOnly)}
		}
		return []string{t.Format(time.RFC3339)}
	case formrules.Int:
		if v.CanInt() {
			return []string{strconv.FormatInt(v.Int(), 10)}
		}
		return []string{strconv.FormatUint(v.Uint(), 10)}
	case formrules.Float:
		return []string{strconv.FormatFloat(v.Float(), 'f', -1, 64)}
	}
	return []string{v.String()}
}

var fieldCache sync.Map // reflect.Type → []formrules.Field

func cachedFields(t reflect.Type) ([]formrules.Field, error) {
	if v, ok := fieldCache.Load(t); ok {
		return v.([]formrules.Field), nil
	}
	fs, err := formrules.FromReflect(t)
	if err != nil {
		fs = nil
	}
	fieldCache.Store(t, fs)
	return fs, err
}
