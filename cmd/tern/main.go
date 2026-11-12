// Command tern is the command-line interface for the Tern framework.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/Unfield/Tern/internal/ui"
	"github.com/Unfield/Tern/internal/version"
)

type command struct {
	name, args, summary string
}

var commands = []command{
	{"new", "<dir>", "Create a new project"},
	{"dev", "", "Start the development server"},
	{"build", "", "Build a production binary"},
	{"deploy", "[target]", "Deploy to a server over SSH, Fly.io or a container registry"},
	{"gen", "", "Generate route wiring and TypeScript types"},
	{"check", "", "Generate code and type-check the app"},
	{"routes", "[dir]", "List the app's routes"},
	{"migrate", "new <name>", "Create a database migration"},
	{"ui", "add <Component>", "Copy Tern UI components into the project"},
	{"add", "<plugin>", "Add a plugin to the app"},
	{"doctor", "[target]", "Check the project before a deploy"},
	{"mcp", "", "Serve the Model Context Protocol for AI coding tools"},
	{"lsp", "", "Serve the language server for editors"},
	{"version", "", "Print the Tern version"},
}

func printUsage(w io.Writer) {
	p := ui.New(w)
	p.Printf("\n  %s %s\n\n", p.Brand(p.Bold("tern")), p.Dim("Svelte on the client, Go on the server"))
	p.Printf("  %s\n", p.Bold("Usage"))
	p.Printf("    tern %s %s\n\n", p.Cyan("<command>"), p.Dim("[options]"))
	p.Printf("  %s\n", p.Bold("Commands"))
	for _, c := range commands {
		usage := c.name
		if c.args != "" {
			usage += " " + c.args
		}
		p.Printf("    %s%s %s\n", p.Cyan(usage), pad(usage, 19), c.summary)
	}
	p.Printf("\n  Run %s for a command's options.\n", p.Cyan("tern <command> -h"))
	p.Printf("  Docs: %s\n\n", p.Dim("https://github.com/Unfield/Tern/blob/main/docs/guide.md"))
}

func pad(s string, width int) string {
	if len(s) >= width {
		return " "
	}
	return fmt.Sprintf("%*s", width-len(s), "")
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch cmd := args[0]; cmd {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "tern %s (%s, %s/%s)\n", version.String(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return 0
	case "routes":
		return cmdRoutes(args[1:], stdout, stderr)
	case "gen":
		return cmdGen(args[1:], stdout, stderr)
	case "check":
		return cmdCheck(args[1:], stdout, stderr)
	case "dev":
		return cmdDev(args[1:], stdout, stderr)
	case "build":
		return cmdBuild(args[1:], stdout, stderr)
	case "deploy":
		return cmdDeploy(args[1:], stdout, stderr)
	case "new":
		return cmdNew(args[1:], stdout, stderr)
	case "mcp":
		return cmdMCP(args[1:], stdout, stderr)
	case "lsp":
		return cmdLSP(args[1:], os.Stdin, stdout, stderr)
	case "migrate":
		return cmdMigrate(args[1:], stdout, stderr)
	case "ui":
		return cmdUI(args[1:], stdout, stderr)
	case "add":
		return cmdAdd(args[1:], stdout, stderr)
	case "doctor":
		return cmdDoctor(args[1:], stdout, stderr)
	default:
		ui.New(stderr).Error(fmt.Errorf("unknown command %q", cmd))
		printUsage(stderr)
		return 2
	}
}
