package main

import (
	"bytes"
	"fmt"
	"go/token"
	"go/types"
	"log"
	"os"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/interp"
	"golang.org/x/tools/go/ssa/ssautil"
)

type config struct {
	debugLog      bool
	enableTracing bool
	dumpSsa       bool
}

func newInterpreter(fileName string, conf config) (*interp.Interpreter, error) {
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
	if conf.enableTracing {
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
	if conf.enableTracing {
		interpMode |= interp.EnableTracing
	}

	mainPackages := ssautil.MainPackages(program.AllPackages())
	if len(mainPackages) == 0 {
		return nil, fmt.Errorf("error: 0 packages")
	}
	mainPackage := mainPackages[0]
	if conf.dumpSsa {
		dump(mainPackage)
	}

	return interp.NewInterpreter(program, mainPackage, interpMode, sizes, fileName, nil), nil
}

//func main() {
//	i, err := newInterpreter("/home/buraindo/programs/max2.go", config{
//		debugLog:      false,
//		enableTracing: false,
//		dumpSsa:       true,
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	code := i.Interpret()
//	os.Exit(code)
//}

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
