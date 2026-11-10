package nativespike

import "strconv"

// The benchmark's data, as Go structs: in Tern a load already has them.
type Item struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Price   float64  `json:"price"`
	Tags    []string `json:"tags"`
	Updated string   `json:"updated"`
	Stock   int      `json:"stock"`
}

type ItemsData struct {
	Items []Item `json:"items"`
}

// What a compiler would emit for Root, the layout and the page: Tern's
// Root.svelte wraps the layout and page with these markers.
const (
	rootOpen  = `<!--[--><!--[0--><!--[-1--><!--[--><!--[0--><!--[0--><!--[-->`
	rootClose = `<!--]--><!--]--><!--]--><!----><!--]--><!--]--><!--]--><!--]-->`
)

// RenderItemsPage renders /items: Root, then Shell, then Items.
func RenderItemsPage(b []byte, d ItemsData) []byte {
	b = append(b, rootOpen...)
	b = renderShell(b, "Items", func(b []byte) []byte { return renderItems(b, d.Items) })
	return append(b, rootClose...)
}

var shellLinks = [...]struct{ href, label string }{{"/", "Home"}, {"/items", "Items"}, {"/about", "About"}}

// renderShell is Shell.svelte, as Svelte's server output does it.
func renderShell(b []byte, title string, children func([]byte) []byte) []byte {
	b = append(b, `<header><a class="brand" href="/">Bench</a> <nav aria-label="Main"><!--[-->`...)
	for _, link := range shellLinks {
		b = append(b, `<a href="`...)
		b = escapeHTML(b, link.href, true)
		b = append(b, `">`...)
		b = escapeHTML(b, link.label, false)
		b = append(b, `</a>`...)
	}
	b = append(b, `<!--]--></nav></header> <main><h1>`...)
	b = escapeHTML(b, title, false)
	b = append(b, `</h1> `...)
	b = children(b)
	return append(b, `<!----></main> <footer><p>The same components, rendered by two frameworks.</p></footer>`...)
}

// renderItems is Items.svelte. Its $derived values are plain Go here: the
// types come from the Go structs, so `items.reduce((sum, i) => sum + i.price
// * i.stock, 0)` is a typed loop, in the same order, with the same floats.
func renderItems(b []byte, items []Item) []byte {
	total := 0.0
	out := 0
	for _, i := range items {
		total += i.Price * float64(i.Stock)
		if i.Stock == 0 {
			out++
		}
	}
	b = append(b, `<p>`...)
	b = strconv.AppendInt(b, int64(len(items)), 10)
	b = append(b, ` items, `...)
	b = strconv.AppendInt(b, int64(out), 10)
	b = append(b, ` out of stock, worth `...)
	b = escapeHTML(b, jsToFixed(total, 2), false)
	b = append(b, ` in all.</p> <table><thead><tr><th>#</th><th>Name</th><th>Tags</th><th>Price</th><th>Stock</th><th>Updated</th></tr></thead><tbody><!--[-->`...)
	for _, item := range items {
		b = append(b, `<tr`...)
		if item.Stock == 0 {
			b = append(b, ` class="out"`...)
		}
		b = append(b, `><td>`...)
		b = strconv.AppendInt(b, int64(item.ID), 10)
		b = append(b, `</td><td><a href="/items/`...)
		b = strconv.AppendInt(b, int64(item.ID), 10)
		b = append(b, `">`...)
		b = escapeHTML(b, item.Name, false)
		b = append(b, `</a></td><td><!--[-->`...)
		for _, tag := range item.Tags {
			b = append(b, `<span class="tag">`...)
			b = escapeHTML(b, tag, false)
			b = append(b, `</span>`...)
		}
		b = append(b, `<!--]--></td><td>`...)
		b = escapeHTML(b, jsToFixed(item.Price, 2), false)
		b = append(b, `</td><td>`...)
		if item.Stock == 0 {
			b = append(b, `Out of stock`...)
		} else {
			b = strconv.AppendInt(b, int64(item.Stock), 10)
		}
		b = append(b, `</td><td>`...)
		u := item.Updated
		if len(u) > 10 {
			u = u[:10] // slice(0, 10); safe here because the strings are ASCII
		}
		b = escapeHTML(b, u, false)
		b = append(b, `</td></tr>`...)
	}
	return append(b, `<!--]--></tbody></table>`...)
}
