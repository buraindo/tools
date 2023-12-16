package main

import (
	"fmt"
	"go/types"
	"log"
	"os"
	"unsafe"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/interp"
	"golang.org/x/tools/go/ssa/ssautil"
)

type javaBridge struct {
	log         bool
	interpreter *interp.Interpreter
	callGraph   *callgraph.Graph
	registry    map[uintptr]any
	goCalls     int
	javaCalls   int
}

var anyType = types.Type(types.NewInterfaceType(nil, nil).Complete())
var bridge = &javaBridge{
	registry: make(map[uintptr]any),
}

func (b *javaBridge) Log(values ...any) {
	if b.log {
		log.Println(values...)
	}
}

func newInterpreter(file, entrypoint string, conf config) (*interp.Interpreter, error) {
	f, err := os.Open(file)
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
	initialPackages, err := packages.Load(cfg, file)
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

	return interp.NewInterpreter(program, mainPackage, interpMode, sizes, file, entrypoint, nil), nil
}

// ---------------- region: init

func initBridge(file, entrypoint string, debug bool) error {
	var err error
	bridge.interpreter, err = newInterpreter(file, entrypoint, config{
		debugLog:      false,
		enableTracing: false,
		dumpSsa:       false,
	})
	if err != nil {
		return fmt.Errorf("init interpreter: %w", err)
	}
	if debug {
		bridge.log = true
	}
	bridge.javaCalls = 0
	bridge.goCalls = 0

	program := bridge.interpreter.Program()
	// TODO: fix panic with import "reflect"
	callGraph := vta.CallGraph(ssautil.AllFunctions(program), cha.CallGraph(program))
	callGraph.DeleteSyntheticNodes()
	bridge.callGraph = callGraph

	return nil
}

// ---------------- region: init

// ---------------- region: utils

func fromPointer[T any](in uintptr) *T {
	return bridge.registry[in].(*T)
}

func toPointer[T any](in *T) uintptr {
	out := uintptr(unsafe.Pointer(in))
	bridge.registry[out] = in
	return out
}

func resolveVar(in ssa.Value) string {
	switch in := in.(type) {
	case *ssa.Parameter:
		f := in.Parent()
		for i, p := range f.Params {
			if p == in {
				return fmt.Sprintf("p%d", i)
			}
		}
	case *ssa.Const:
		return in.Value.String()
	}
	return in.Name()
}

// ---------------- region: utils
