package interp

import (
	"fmt"
	"go/token"
	"go/types"
	"runtime"

	"golang.org/x/tools/go/ssa"
)

type Interpreter struct {
	*interpreter
	filename    string
	mainPackage *ssa.Package
}

func NewInterpreter(
	program *ssa.Program,
	mainPackage *ssa.Package,
	mode Mode,
	sizes types.Sizes,
	filename string,
	args []string,
) *Interpreter {
	i := &interpreter{
		prog:       program,
		globals:    make(map[*ssa.Global]*value),
		mode:       mode,
		sizes:      sizes,
		goroutines: 1,
	}
	initReflect(i)
	i.osArgs = append(i.osArgs, filename)
	for _, arg := range args {
		i.osArgs = append(i.osArgs, arg)
	}

	for _, pkg := range i.prog.AllPackages() {
		// Initialize global storage.
		for _, m := range pkg.Members {
			switch v := m.(type) {
			case *ssa.Global:
				cell := zero(deref(v.Type()))
				i.globals[v] = &cell
			}
		}
	}

	return &Interpreter{
		interpreter: i,
		filename:    filename,
		mainPackage: mainPackage,
	}
}

func (i *Interpreter) Interpret() int {
	fmt.Printf("Running: %s\n", i.mainPackage.Pkg.Name())
	exitCode := 2
	defer func() {
		if exitCode != 2 || i.mode&DisableRecover != 0 {
			return
		}
		switch p := recover().(type) {
		case exitPanic:
			exitCode = int(p)
			return
		case targetPanic:
			fmt.Println("panic:", toString(p.v))
		case runtime.Error:
			fmt.Println("panic:", p.Error())
		case string:
			fmt.Println("panic:", p)
		default:
			fmt.Printf("panic: unexpected type: %T: %v\n", p, p)
		}

		// TODO(adonovan): dump panicking interpreter goroutine?
		// buf := make([]byte, 0x10000)
		// runtime.Stack(buf, false)
		// fmt.Fprintln(os.Stderr, string(buf))
		// (Or dump panicking target goroutine?)
	}()

	// Run!
	call(i.interpreter, nil, token.NoPos, i.Init(), nil)
	if mainFn := i.Main(); mainFn != nil {
		call(i.interpreter, nil, token.NoPos, mainFn, nil)
		exitCode = 0
	} else {
		fmt.Println("No main function")
		exitCode = 1
	}
	return exitCode
}

func (i *Interpreter) Filename() string {
	if i == nil {
		return "not initialized"
	}
	return i.filename
}

func (i *Interpreter) Program() *ssa.Program {
	return i.prog
}

func (i *Interpreter) Init() *ssa.Function {
	return i.mainPackage.Func("init")
}

func (i *Interpreter) Main() *ssa.Function {
	return i.mainPackage.Func("main")
}
