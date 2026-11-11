// Command website is tern.land, Tern's website, built with Tern. Every page
// is prerendered: `go tool tern build` writes the whole site to
// .tern/output/client, which is served as static files.
package main

import "log"

func main() {
	app, err := ternApp()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
