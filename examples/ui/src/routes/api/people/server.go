// Package people is a small search endpoint for the Combobox demo.
package people

import (
	"strings"

	"github.com/Unfield/Tern/kit"
)

type Person struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Team  string `json:"team"`
}

var everyone = []Person{
	{"ada", "Ada Lovelace", "ada@example.com", "Engineering"},
	{"grace", "Grace Hopper", "grace@example.com", "Engineering"},
	{"alan", "Alan Turing", "alan@example.com", "Research"},
	{"katherine", "Katherine Johnson", "katherine@example.com", "Research"},
	{"linus", "Linus Torvalds", "linus@example.com", "Engineering"},
	{"margaret", "Margaret Hamilton", "margaret@example.com", "Engineering"},
	{"barbara", "Barbara Liskov", "barbara@example.com", "Research"},
	{"ken", "Ken Thompson", "ken@example.com", "Operations"},
	{"radia", "Radia Perlman", "radia@example.com", "Operations"},
}

type Query struct {
	Q string `json:"q,omitempty"`
}

// GET finds people whose name or email contains q.
func GET(ctx *kit.Ctx, in Query) ([]Person, error) {
	q := strings.ToLower(strings.TrimSpace(in.Q))
	out := []Person{}
	for _, p := range everyone {
		if q == "" || strings.Contains(strings.ToLower(p.Name), q) || strings.Contains(p.Email, q) {
			out = append(out, p)
		}
	}
	return out, nil
}
