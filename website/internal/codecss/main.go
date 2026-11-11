// Command codecss writes src/lib/code.css: go run ./internal/codecss
package main

import (
	"log"
	"os"

	"github.com/Unfield/Tern/website/internal/docs"
)

func main() {
	css, err := docs.CodeCSS()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("src/lib/code.css", []byte(css), 0o644); err != nil {
		log.Fatal(err)
	}
}
