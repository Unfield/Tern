package kit

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type signup struct {
	Name     string    `form:"name" validate:"required,min=2" label:"Your name"`
	Email    string    `form:"email" validate:"required,email"`
	Password string    `form:"password" validate:"required,min=8"`
	Confirm  string    `form:"confirm" validate:"same=password" label:"Confirmation"`
	Seats    *int      `form:"seats" validate:"min=1"`
	Starts   time.Time `form:"starts"`
	Tags     []string  `form:"tags" validate:"max=2"`
	Terms    bool      `form:"terms" validate:"required"`
}

func (s *signup) Validate() error {
	if strings.HasSuffix(s.Email, "@blocked.example") {
		return FieldError("email", "That domain can't sign up.")
	}
	return nil
}

func formManifest(got *signup) *Manifest {
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/signup", Page: &PageDef{
		Component: "src/routes/signup/+page.svelte",
		Actions: Actions{
			"default": FormAction(func(c *Ctx, in signup) (map[string]string, error) {
				if in.Email == "taken@example.com" {
					return nil, FieldError("email", "That email is already registered.")
				}
				if in.Name == "Mallory" {
					return nil, FormError("Sign-ups are closed.")
				}
				*got = in
				return map[string]string{"welcome": in.Name}, nil
			}),
		},
	}})
	return m
}

func TestFormAction(t *testing.T) {
	var got signup
	a, _ := newTestApp(t, formManifest(&got))
	good := url.Values{"name": {"Ada"}, "email": {"ada@example.com"}, "password": {"correct horse"}, "confirm": {"correct horse"}, "seats": {"3"}, "starts": {"2026-11-10"}, "tags": {"go", "", "svelte"}, "terms": {"on"}}

	ok := jsonPayload(t, postForm(t, a, "/signup", good, "X-Tern-Action", "true"))
	if ok.Type != "success" || ok.Data.(map[string]any)["welcome"] != "Ada" {
		t.Fatalf("success = %+v", ok)
	}
	if got.Seats == nil || *got.Seats != 3 || !got.Terms || got.Starts.Format(time.DateOnly) != "2026-11-10" || strings.Join(got.Tags, ",") != "go,svelte" {
		t.Errorf("decoded %+v", got)
	}

	bad := url.Values{"name": {"A"}, "email": {"nope"}, "password": {"short"}, "confirm": {"other"}, "seats": {"x"}, "tags": {"a", "b", "c"}}
	rec := postForm(t, a, "/signup", bad, "X-Tern-Action", "true")
	fail := jsonPayload(t, rec)
	if fail.Type != "failure" || fail.Status != 400 {
		t.Fatalf("failure = %+v", fail)
	}
	data := fail.Data.(map[string]any)
	errs := data["errors"].(map[string]any)
	want := map[string]string{
		"name":     "Your name must be at least 2 characters.",
		"email":    "Email must be an email address.",
		"password": "Password must be at least 8 characters.",
		"confirm":  "Confirmation must match password.",
		"seats":    "Seats must be a whole number.",
		"tags":     "Choose at most 2 for tags.",
		"terms":    "Terms must be checked.",
	}
	for k, v := range want {
		if errs[k] != v {
			t.Errorf("errors[%s] = %v, want %q", k, errs[k], v)
		}
	}
	values := data["values"].(map[string]any)
	if values["name"] != "A" || values["email"] != "nope" {
		t.Errorf("values = %v", values)
	}
	if _, leaked := values["password"]; leaked {
		t.Error("passwords must not be sent back")
	}
	if _, leaked := values["confirm"]; leaked {
		t.Error("a field that must match a password is secret too")
	}

	// Errors from Validate and from the action land on fields too.
	for email, msg := range map[string]string{"x@blocked.example": "That domain can't sign up.", "taken@example.com": "That email is already registered."} {
		v := cloneValues(good)
		v.Set("email", email)
		f := jsonPayload(t, postForm(t, a, "/signup", v, "X-Tern-Action", "true"))
		if f.Status != 400 || f.Data.(map[string]any)["errors"].(map[string]any)["email"] != msg {
			t.Errorf("%s: %+v", email, f)
		}
	}
	v := cloneValues(good)
	v.Set("name", "Mallory")
	f := jsonPayload(t, postForm(t, a, "/signup", v, "X-Tern-Action", "true"))
	if f.Data.(map[string]any)["message"] != "Sign-ups are closed." {
		t.Errorf("form error: %+v", f)
	}

	// Without JavaScript, the page renders with the errors as its form prop.
	page := pagePayload(t, postForm(t, a, "/signup", bad))
	if page.Form.(map[string]any)["errors"].(map[string]any)["email"] != "Email must be an email address." {
		t.Errorf("no-JS failure: %+v", page.Form)
	}
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, s := range v {
		out[k] = append([]string(nil), s...)
	}
	return out
}

func TestFormActionBadTagsPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a bad tag should panic when the action is defined")
		}
	}()
	type bad struct {
		X string `validate:"requird"`
	}
	FormAction(func(*Ctx, bad) (int, error) { return 0, nil })
}

type typedIn struct {
	Name  string `json:"name" validate:"required,max=5"`
	Count int    `json:"count" validate:"min=1,max=10"`
}

func TestCheckTyped(t *testing.T) {
	if err := checkTyped(typedIn{Name: "ok", Count: 3}); err != nil {
		t.Errorf("valid input: %v", err)
	}
	err := checkTyped(typedIn{Name: "", Count: 11})
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != http.StatusBadRequest || he.Message != "Name is required. Count must be at most 10." {
		t.Errorf("got %v", err)
	}
	if err := checkTyped(typedIn{Name: "ok"}); err != nil {
		t.Errorf("a missing number isn't checked against its rules: %v", err)
	}
	if checkTyped(struct{ M map[string]int }{}) != nil {
		t.Error("inputs without form types have no rules")
	}
}
