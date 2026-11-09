// Package kit is the server runtime for Tern applications.
//
// A route directory's Go package defines what the route does on the server,
// by exporting:
//
//	func Load(ctx *kit.Ctx) (T, error)        // data for +page.svelte
//	func LayoutLoad(ctx *kit.Ctx) (T, error)  // data for +layout.svelte
//	var Actions = kit.Actions{...}            // form actions for the page
//	func GET(ctx *kit.Ctx) error              // endpoint handlers: GET, POST, …
//
// T must encode to a JSON object: a struct or a map. Its JSON is passed to
// the Svelte component as data, with TypeScript types generated from T.
//
// Errors control the response. Return [Error] or [NotFound] for an expected
// error page, [Redirect] to redirect, and [Fail] from an action to report a
// validation error. Any other error is logged and shown as "Internal Error".
//
// `tern gen` writes tern.gen.go, which wires every route into a [Manifest]
// and defines ternApp. A typical main.go:
//
//	func main() {
//		app, err := ternApp(kit.WithHook(auth))
//		if err != nil {
//			log.Fatal(err)
//		}
//		if err := app.ListenAndServe(); err != nil {
//			log.Fatal(err)
//		}
//	}
package kit
