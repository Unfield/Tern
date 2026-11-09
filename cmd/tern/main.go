// Command tern is the command-line interface for the Tern framework.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Unfield/Tern/internal/version"
)

const usage = `Tern: Svelte on the client, Go on the server.

Usage:
  tern <command> [arguments]

Run tern <command> -h for a command's options.

Commands:
  new       Create a new Tern project
  dev       Start the development server
  gen       Generate the route manifest and TypeScript types
  build     Build a production binary
  routes    List the routes in src/routes (or the given directory)
  version   Print the Tern version
  help      Show this help
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch cmd := args[0]; cmd {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "tern %s\n", version.String())
		return 0
	case "routes":
		return cmdRoutes(args[1:], stdout, stderr)
	case "gen":
		return cmdGen(args[1:], stdout, stderr)
	case "dev":
		return cmdDev(args[1:], stdout, stderr)
	case "build":
		return cmdBuild(args[1:], stdout, stderr)
	case "new":
		return cmdNew(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "tern: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}
