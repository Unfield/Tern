package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Unfield/Tern/internal/native"
)

// nativeFile is the server renderer compiled to Go: part of the app's main
// package, it registers itself with the manifest.
const nativeFile = "tern_native.gen.go"

// nativeIR is where the Vite plugin records the server build's modules.
const nativeIR = ".tern/native/ir.json"

// nativeSources is the modules' code, embedded for functions' toString.
const nativeSources = ".tern/native/sources.gz"

// nativeReport lists what native rendering leaves to the JavaScript engine.
const nativeReport = ".tern/native/report.txt"

// engineFree is whether the compiler left nothing to the JavaScript
// engine, so the binary can be built without it.
var engineFree bool

// compileNative compiles the server build to Go. It doesn't fail the
// build: without the file, pages render with the JavaScript engine as
// before.
func compileNative(log io.Writer) (string, error) {
	engineFree = false
	ir, err := native.LoadIR(nativeIR)
	if err != nil {
		return "skipped: " + err.Error(), nil //nolint:nilerr // the engine renders instead
	}
	res, err := native.Compile(ir, native.Options{Package: "main", Prefix: "tern", Register: "ternManifest.Native", EmbedSources: nativeSources})
	if err != nil {
		fmt.Fprintln(log, err)
		return "skipped: the compiler failed, see " + nativeReport, writeReport(res, err)
	}
	if err := os.WriteFile(nativeSources, res.Sources, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(nativeFile, res.Source, 0o644); err != nil {
		return "", err
	}
	if err := writeReport(res, nil); err != nil {
		return "", err
	}
	if len(res.Problems) == 0 {
		engineFree = true
		return fmt.Sprintf("%d modules", res.Modules), nil
	}
	return fmt.Sprintf("%d modules; %d constructs use the JavaScript engine, see %s", res.Modules, len(res.Problems), nativeReport), nil
}

func writeReport(res *native.Result, compileErr error) error {
	if err := os.MkdirAll(filepath.Dir(nativeReport), 0o755); err != nil {
		return err
	}
	f, err := os.Create(nativeReport)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	fmt.Fprintln(f, "Native rendering (ADR 0011): what the compiler left to the JavaScript engine.")
	fmt.Fprintln(f, "A page renders with the engine when it runs one of these.")
	fmt.Fprintln(f)
	if compileErr != nil {
		fmt.Fprintln(f, "The compiler failed:", compileErr)
	}
	if res == nil {
		return nil
	}
	for _, p := range res.Problems {
		fmt.Fprintln(f, p)
	}
	for _, m := range res.Mismatches {
		fmt.Fprintln(f, "intrinsic not used:", m)
	}
	return nil
}
