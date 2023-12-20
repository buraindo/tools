package common

import (
	"bytes"
	"fmt"
	"go/token"
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

type Config struct {
	DebugLog      bool
	EnableTracing bool
	DumpSsa       bool
}

type JavaBridge struct {
	Interpreter *interp.Interpreter
	CallGraph   *callgraph.Graph
	GoCalls     int
	JavaCalls   int

	log      bool
	debug    bool
	registry map[uintptr]any
}

var AnyType = types.Type(types.NewInterfaceType(nil, nil).Complete())
var Bridge = &JavaBridge{
	registry: make(map[uintptr]any),
}

func (b *JavaBridge) Log(values ...any) {
	if b.log {
		log.Println(values...)
	}
}

func NewInterpreter(file, entrypoint string, conf Config) (*interp.Interpreter, error) {
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
	if conf.EnableTracing {
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
	if conf.EnableTracing {
		interpMode |= interp.EnableTracing
	}

	mainPackages := ssautil.MainPackages(program.AllPackages())
	if len(mainPackages) == 0 {
		return nil, fmt.Errorf("error: 0 packages")
	}
	mainPackage := mainPackages[0]
	if conf.DumpSsa {
		dump(mainPackage)
	}

	return interp.NewInterpreter(program, mainPackage, interpMode, sizes, file, entrypoint, nil), nil
}

// ---------------- region: init

func Init(file, entrypoint string, debug bool) error {
	var err error
	Bridge.Interpreter, err = NewInterpreter(file, entrypoint, Config{
		DebugLog:      false,
		EnableTracing: false,
		DumpSsa:       false,
	})
	if err != nil {
		return fmt.Errorf("init interpreter: %w", err)
	}
	if debug {
		Bridge.log = true
	}
	Bridge.debug = debug
	Bridge.JavaCalls = 0
	Bridge.GoCalls = 0

	program := Bridge.Interpreter.Program()
	// TODO: fix panic with import "reflect"
	callGraph := vta.CallGraph(ssautil.AllFunctions(program), cha.CallGraph(program))
	callGraph.DeleteSyntheticNodes()
	Bridge.CallGraph = callGraph

	return nil
}

// ---------------- region: init

// ---------------- region: shutdown

func Shutdown() error {
	return nil
}

// ---------------- region: shutdown

// ---------------- region: utils

func FromPointer[T any](in uintptr) *T {
	return Bridge.registry[in].(*T)
}

func ToPointer[T any](in *T) uintptr {
	out := uintptr(unsafe.Pointer(in))
	Bridge.registry[out] = in
	return out
}

func ResolveVar(in ssa.Value) string {
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

// ---------------- region: utils
