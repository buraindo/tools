package main

import (
	"fmt"
	"go/types"
	"log"
	"os"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/interp"
	"golang.org/x/tools/go/ssa/ssautil"
)

func newInterpreter(fileName string, debug bool) (*interp.Interpreter, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	if err = f.Close(); err != nil {
		return nil, fmt.Errorf("close file: %w", err)
	}

	mode := packages.NeedName |
		packages.NeedFiles |
		packages.NeedCompiledGoFiles |
		packages.NeedImports |
		packages.NeedDeps |
		packages.NeedExportFile |
		packages.NeedTypes |
		packages.NeedTypesSizes |
		packages.NeedTypesInfo |
		packages.NeedSyntax |
		packages.NeedModule |
		packages.NeedEmbedFiles |
		packages.NeedEmbedPatterns
	cfg := &packages.Config{Mode: mode}
	if debug {
		cfg.Logf = log.Printf
	}
	initialPackages, err := packages.Load(cfg, fileName)
	if err != nil {
		return nil, err
	}
	if len(initialPackages) == 0 {
		return nil, fmt.Errorf("no packages were loaded")
	}

	if packages.PrintErrors(initialPackages) > 0 {
		return nil, fmt.Errorf("packages contain errors")
	}

	program, _ := ssautil.AllPackages(initialPackages, ssa.InstantiateGenerics|ssa.SanityCheckFunctions)
	program.Build()

	sizes := &types.StdSizes{
		MaxAlign: 8,
		WordSize: 8,
	}
	var interpMode interp.Mode
	if debug {
		interpMode |= interp.EnableTracing
	}

	return interp.NewInterpreter(program, interpMode, sizes, fileName, nil), nil
}

func interpret(fileName string, debug bool) {
	i, err := newInterpreter(fileName, debug)
	if err != nil {
		log.Fatal(err)
	}
	code := i.Interpret(debug)
	os.Exit(code)
}

func main() {
	interpret("/home/buraindo/programs/max2.go", false)
}
