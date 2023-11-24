package main

import (
	"bytes"
	"fmt"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/interp"
	"golang.org/x/tools/go/ssa/ssautil"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

var (
	_, b, _, _ = runtime.Caller(0)
	basePath   = filepath.Dir(b)
)

func dump(mainPackage *ssa.Package) {
	out := bytes.Buffer{}
	ssa.WritePackage(&out, mainPackage)
	for _, object := range mainPackage.Members {
		if object.Token() == token.FUNC {
			ssa.WriteFunction(&out, mainPackage.Func(object.Name()))
		}
	}
	fmt.Print(out.String())
}

func interpret(fileName string, debug bool) error {
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
	initialPackages, err := packages.Load(cfg, fileName)
	if err != nil {
		log.Fatal(err)
	}

	if packages.PrintErrors(initialPackages) > 0 {
		log.Fatalf("packages contain errors")
	}

	program, ssaPackages := ssautil.AllPackages(initialPackages, ssa.InstantiateGenerics|ssa.SanityCheckFunctions)
	program.Build()

	sizes := &types.StdSizes{
		MaxAlign: 8,
		WordSize: 8,
	}
	var interpMode interp.Mode
	if debug {
		interpMode |= interp.EnableTracing
	}
	for _, mainPackage := range ssautil.MainPackages(ssaPackages) {
		fmt.Printf("Running: %s\n", mainPackage.Pkg.Name())
		if !debug {
			dump(mainPackage)
		}
		code := interp.InterpretFunc(mainPackage, "main", interpMode, sizes, mainPackage.Pkg.Path(), nil)
		os.Exit(code)
	}
	return fmt.Errorf("no main package")
}

func main() {
	fatal(interpret(path("../testdata/buraindo/max2.go"), false))
}

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func path(p string) string {
	return filepath.Join(basePath, p)
}
