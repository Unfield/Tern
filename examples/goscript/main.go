// Command goscript shows Tern's experimental <script lang="go">.
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
