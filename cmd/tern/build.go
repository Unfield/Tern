package main

import (
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Unfield/Tern/internal/codegen"
)

// cmdBuild produces a production binary with the client embedded.
func cmdBuild(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("build", flag.ContinueOnError)
	fl.SetOutput(stderr)
	out := fl.String("o", "", "output binary (default build/<directory name>)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: tern build [-o output]")
		return 2
	}
	if *out == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "tern build: %v\n", err)
			return 1
		}
		*out = filepath.Join("build", filepath.Base(wd))
		if runtime.GOOS == "windows" {
			*out += ".exe"
		}
	}

	start := time.Now()
	step := func(name string, fn func() error) bool {
		fmt.Fprintf(stdout, "  %s\n", name)
		if err := fn(); err != nil {
			fmt.Fprintf(stderr, "tern build: %v\n", err)
			return false
		}
		return true
	}

	fmt.Fprintln(stdout, "Building for production")
	ok := step("generating routes and types", func() error {
		_, err := codegen.Generate(".")
		return err
	}) && step("building the client with Vite", func() error {
		return runVite(stdout, stderr, "build", "--logLevel", "warn")
	}) && step("compressing assets", func() error {
		return precompress(codegen.ClientOut)
	}) && step("compiling the server", func() error {
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", *out, ".")
		cmd.Stdout, cmd.Stderr = stdout, stderr
		return cmd.Run()
	})
	if !ok {
		return 1
	}
	fmt.Fprintf(stdout, "Built %s in %s\n", *out, time.Since(start).Round(time.Millisecond))
	return 0
}

// viteBin returns the project's local Vite executable.
func viteBin() (string, error) {
	name := "vite"
	if runtime.GOOS == "windows" {
		name = "vite.cmd"
	}
	p := filepath.Join("node_modules", ".bin", name)
	if _, err := os.Stat(p); err != nil {
		return "", errors.New("vite is not installed; run your package manager's install command (npm install, pnpm install, …) first")
	}
	return p, nil
}

func runVite(stdout, stderr io.Writer, args ...string) error {
	bin, err := viteBin()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vite %s: %w", args[0], err)
	}
	return nil
}

var compressible = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".html": true, ".svg": true, ".json": true,
	".txt": true, ".xml": true, ".wasm": true, ".map": true, ".webmanifest": true, ".ico": true,
}

// precompress writes a .gz next to every compressible asset over 1 KiB
// when compression saves space. The server serves it to clients that
// accept gzip.
func precompress(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !compressible[strings.ToLower(filepath.Ext(p))] {
			return err
		}
		info, err := d.Info()
		if err != nil || info.Size() < 1024 {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var buf strings.Builder
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return err
		}
		if _, err := zw.Write(src); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		if buf.Len() >= len(src) {
			return nil
		}
		return os.WriteFile(p+".gz", []byte(buf.String()), 0o644)
	})
}
