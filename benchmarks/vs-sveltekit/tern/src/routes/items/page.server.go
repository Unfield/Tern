package items

import (
	"fmt"
	"time"

	"github.com/Unfield/Tern/kit"
)

type Item struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Price   float64  `json:"price"`
	Tags    []string `json:"tags"`
	Updated string   `json:"updated"`
	Stock   int      `json:"stock"`
}

type Data struct {
	Items []Item `json:"items"`
}

// The same items as the SvelteKit app's load, from the same formula.
func Load(*kit.Ctx) (Data, error) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	items := make([]Item, 100)
	for i := range items {
		n := i + 1
		items[i] = Item{
			ID:      n,
			Name:    fmt.Sprintf("Item %d", n),
			Price:   float64((n*37)%1000)/10 + 0.99,
			Tags:    []string{fmt.Sprintf("tag%d", n%5), fmt.Sprintf("tag%d", n%7+5), fmt.Sprintf("tag%d", n%3+12)},
			Updated: start.AddDate(0, 0, n).Format(time.RFC3339),
			Stock:   (n * 13) % 9,
		}
	}
	return Data{Items: items}, nil
}
