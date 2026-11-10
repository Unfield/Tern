// Command ternbench is the Tern half of the comparison with SvelteKit.
package main

import (
	"log"
	"os"
	"strconv"

	"github.com/Unfield/Tern/kit"
)

func main() {
	var opts []kit.Option
	// SSR_WORKERS changes the number of rendering VMs, to measure scaling.
	if n, err := strconv.Atoi(os.Getenv("SSR_WORKERS")); err == nil {
		opts = append(opts, kit.WithSSRWorkers(n))
	}
	app, err := ternApp(opts...)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(app.ListenAndServe())
}
