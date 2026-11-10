package formrules

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

type cases struct {
	Fields map[string]Field `json:"fields"`
	Cases  []struct {
		Field  string            `json:"field"`
		Values []string          `json:"values"`
		Other  map[string]string `json:"other"`
		Error  string            `json:"error"`
	} `json:"cases"`
}

// The browser's validate.ts runs the same cases.
func TestCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c cases
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	for name, f := range c.Fields {
		for _, r := range f.Rules {
			if r.Name == "pattern" {
				fs, err := build(f.Name, f.Kind, f.Secret, reflect.StructTag(`validate:"pattern=`+r.Arg+`"`))
				if err != nil {
					t.Fatal(err)
				}
				f.re = fs.re
			}
		}
		c.Fields[name] = f
	}
	for _, tc := range c.Cases {
		f := c.Fields[tc.Field]
		got := Check(f, tc.Values, func(n string) string { return tc.Other[n] }, func(n string) string { return c.Fields[n].Label })
		if got != tc.Error {
			t.Errorf("%s %q: got %q, want %q", tc.Field, tc.Values, got, tc.Error)
		}
	}
}

func TestFromReflect(t *testing.T) {
	type in struct {
		Name      string    `form:"name" validate:"required,min=3" label:"Project name"`
		Email     string    `json:"email" validate:"required,email"`
		Seats     *int      `form:"seats" validate:"min=1"`
		Starts    time.Time `form:"starts"`
		Tags      []string  `form:"tags"`
		Password  string    `form:"password" validate:"required"`
		Token     string    `form:"token,secret"`
		Pattern   string    `form:"slug" validate:"min=1,pattern=[a-z]{1,3},[0-9]+"`
		Skip      string    `form:"-"`
		FirstName string
	}
	fs, err := FromReflect(reflect.TypeFor[in]())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Field{}
	for _, f := range fs {
		got[f.Name] = f
	}
	if len(fs) != 9 {
		t.Fatalf("got %d fields: %+v", len(fs), fs)
	}
	if f := got["name"]; f.Label != "Project name" || !f.Required || f.Kind != String || f.Rules[0] != (Rule{"min", "3"}) {
		t.Errorf("name: %+v", f)
	}
	if got["email"].Label != "Email" || got["seats"].Kind != Int || got["starts"].Kind != Date || got["tags"].Kind != Strings {
		t.Errorf("kinds: %+v", got)
	}
	if !got["password"].Secret || !got["token"].Secret || got["email"].Secret {
		t.Error("secrets")
	}
	if got["slug"].Rules[1].Arg != "[a-z]{1,3},[0-9]+" {
		t.Errorf("pattern takes the rest of the tag: %+v", got["slug"].Rules)
	}
	if got["FirstName"].Label != "First name" {
		t.Errorf("humanize: %q", got["FirstName"].Label)
	}

	type bad struct {
		X map[string]int `form:"x"`
	}
	if _, err := FromReflect(reflect.TypeFor[bad]()); err == nil {
		t.Error("maps can't come from forms")
	}
	type badTag struct {
		X string `validate:"required,minimum=3"`
	}
	if _, err := FromReflect(reflect.TypeFor[badTag]()); err == nil {
		t.Error("unknown rules are errors")
	}
}

func TestHumanize(t *testing.T) {
	for in, want := range map[string]string{"email": "Email", "firstName": "First name", "first_name": "First name", "URLPath": "Url path", "zip-code": "Zip code"} {
		if got := Humanize(in); got != want {
			t.Errorf("Humanize(%q) = %q, want %q", in, got, want)
		}
	}
}
