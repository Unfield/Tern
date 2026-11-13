// Command shelf runs Shelf as a program, as examples/shelf does.
package main

import (
	"log"
	"os"

	shelfapp "shelf"
)

func main() {
	path := os.Getenv("DATABASE")
	if path == "" {
		path = "shelf.db"
	}
	if err := shelfapp.Run(path); err != nil {
		log.Fatal(err)
	}
}
