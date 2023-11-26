package interp

import (
	"bytes"
	"fmt"
	"go/token"
	"go/types"
	"runtime"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type Interpreter struct {
	*interpreter
	filename string
}

func NewInterpreter(
	program *ssa.Program,
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

	return &Interpreter{interpreter: i, filename: filename}
}

func (i *Interpreter) Interpret(debug bool) int {
	packages := ssautil.MainPackages(i.prog.AllPackages())
	if len(packages) == 0 {
		fmt.Println("error: 0 packages")
		return 1
	}
	mainPackage := packages[0]
	fmt.Printf("Running: %s\n", mainPackage.Pkg.Name())
	if !debug {
		dump(mainPackage)
	}

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
	call(i.interpreter, nil, token.NoPos, mainPackage.Func("init"), nil)
	if mainFn := mainPackage.Func("main"); mainFn != nil {
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
